# 6. Security Purge — fail-safe, auditable hard-purge with tombstone wipe

## Status

Accepted — implemented in Phase 1 (P2 use cases).

Evidence of implementation:
the purge domain aggregate
[`internal/domain/purge/purge.go`](../../internal/domain/purge/purge.go),
the purge orchestration service
[`internal/application/security/service.go`](../../internal/application/security/service.go),
the content tombstone wipe
[`internal/adapters/outbound/persistence/memory_pg.go`](../../internal/adapters/outbound/persistence/memory_pg.go)
(`WipeContent`, lines 489–512),
the FTS removal
[`internal/adapters/outbound/search/postgres_fts.go`](../../internal/adapters/outbound/search/postgres_fts.go)
(`Remove`, lines 35–39),
the scope-aware repository contract
[`internal/ports/outbound/purge_repository.go`](../../internal/ports/outbound/purge_repository.go),
and the `purge_records` table in
[`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql)
(lines 281–307).

## Context

When a secret leaks, a compliance request lands, or sensitive content must be
removed, the engine cannot rely on soft-delete: the content must become
undiscoverable while leaving a defensible audit trail. The forces that shaped the
purge decision (per
[design §2.5, §7](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
and the implementing code) were:

- **Three concrete triggers.** Purge reasons are constrained to `secret_leak`,
  `compliance`, and `sensitive_content` (`internal/domain/shared/enums.go` lines
  157–173; migration 001 line 285 enforces the same CHECK), so every purge is
  attributable to a recognized cause.
- **Hard removal, not soft archive.** "Purge flow: target content wiped to empty
  string (tombstone), FTS cleared, embedding nulled, relations removed, derived
  records marked invalidated. PurgeRecord is the authoritative audit trail"
  (design §2.5). Archive is a separate, recoverable lifecycle state; purge is
  terminal.
- **Auditability is mandatory.** A purge cannot be requested without
  `requested_by` and a non-empty `audit_note`
  (`internal/domain/purge/purge.go` lines 54–68), and the record tracks reason,
  scope, status, executed time, and exactly which artifacts were affected
  (`PurgedArtifacts`, `purge.go` lines 9–33).
- **Atomicity across multiple stores-within-Postgres.** Wiping content, clearing
  the FTS vector, and deleting relations must all succeed or all roll back —
  partial purges are a compliance hazard.
- **Strict scope isolation.** A purge must never touch another project's data,
  even when given a known ID; the repository contract requires auth-derived scope
  on every read and status update
  (`internal/ports/outbound/purge_repository.go` lines 11–31).

## Decision

Model purge as a **dedicated aggregate with an explicit state machine**, execute
it **atomically inside a transaction** that wipes content to a tombstone, clears
FTS, and removes relations, and **fail safe** — on any error the purge is marked
`failed` and the audit record is preserved.

Concretely:

- **Explicit state machine.** `PurgeStatus` ∈ `{pending, executing, executed,
  failed}` (`enums.go` lines 175–192). `Request` creates a `pending` record;
  `Execute` transitions `pending → executing` via `MarkExecuting` (rejecting any
  non-pending record with `ErrNotActive`), then to `executed` or `failed`
  (`internal/domain/purge/purge.go` lines 93–119;
  `internal/application/security/service.go` lines 111–162).
- **Scope-checked request, no existence leak.** `Request` fetches the target via
  the *scoped* `memRepo.FindByID`; a target in another project returns
  `ErrNotFound`, so a caller cannot even confirm a record exists outside its scope
  (`service.go` lines 61–75). The purge record stores the *target's* scope, set at
  request time (`service.go` lines 77–88).
- **Atomic execution in one transaction.** `Execute` runs wipe + FTS-remove +
  relation-delete inside `txMgr.WithTx`; the purge's own stored scope
  (`purgeScope`) — not the executor's scope — bounds all writes, preserving the
  audit chain even if the executing key has a different valid scope
  (`service.go` lines 123–156).
- **Tombstone wipe at the row.** `WipeContent` sets `content = ''`, `summary =
  NULL`, `tags = '{}'`, `search_vector = NULL`, `importance_score = 0`,
  `importance_factors = NULL`, `status = 'purged'`, scoped by `project_id` and
  `tenant_id`; zero rows affected returns `ErrNotFound`
  (`internal/adapters/outbound/persistence/memory_pg.go` lines 489–512). The
  domain mirror is `MemoryRecord.MarkPurged`, which clears content/summary/tags
  (`internal/domain/memory/memory.go` lines 157–163).
- **FTS de-indexing.** `searchIdx.Remove` nulls `search_vector` so purged content
  is no longer discoverable via search (`postgres_fts.go` lines 33–39).
- **Recorded artifacts.** On success the service records `FTSInvalidated = true`
  and the count of relations removed into `PurgedArtifacts`, persisted via
  `UpdateStatus` (`service.go` lines 145–155). The `purge_records.artifacts_purged`
  JSONB column holds it (migration 001 line 291).
- **Fail-safe on error.** If the transaction fails, the record is marked `failed`
  with the error detail and the failure status is persisted; the original
  `txErr` is returned to the caller (`service.go` lines 158–162;
  `purge.MarkFailed`, `purge.go` lines 116–119).
- **Events for downstream invalidation.** `purge.requested` and `purge.executed`
  domain events are published (`service.go` lines 94–101, 164–171; event types in
  `enums.go` lines 206–207).

> TODO(verify): [`docs/security-purge.md`](../security-purge.md) is currently an
> empty placeholder (0 lines). This ADR is reconstructed entirely from the purge
> code, the domain aggregate, design §2.5/§7, and migration 001 — there is no
> prose security-purge spec to cite. Author `docs/security-purge.md` (threat
> model, compliance mapping, operator runbook) and then revisit this ADR's
> references.

> TODO(verify): The domain `PurgedArtifacts` carries `EmbeddingInvalidated`,
> `CacheInvalidated`, and `DerivedInvalidated`, and design §2.5 lists "embedding
> nulled … derived records marked invalidated" as part of the flow. The Phase 1
> `Execute` path only sets `FTSInvalidated` and `RelationsRemoved`
> (`service.go` lines 145–149); embedding/cache/derived invalidation are not
> wired (consistent with the no-op embeddings adapter and Phase 1 worker
> exclusions). The CODE is authoritative — these fields are reserved, not yet
> populated. Confirm the intended Phase 2 wiring.

> TODO(verify): The `purge_records.target_type` CHECK allows `memory`, `decision`,
> `heuristic`, and `relation` (migration 001 line 284), but `Request` is
> hard-coded to look up the target in the *memory* repo and stamps
> `TargetType = "memory"` (`service.go` lines 62, 70–79). Phase 1 purge therefore
> only supports memory targets despite the broader schema. Confirm whether
> non-memory purge targets are planned for Phase 2.

## Consequences

### Enables

- **Defensible compliance trail.** Every purge is keyed to a recognized reason,
  carries a mandatory audit note and requester, and records what was affected and
  when — the `PurgeRecord` is the authoritative audit artifact (design §2.5,
  `purge.go` invariants).
- **All-or-nothing removal.** Content wipe, FTS clear, and relation delete commit
  atomically; a mid-flight failure rolls back the data changes and leaves a
  `failed` audit record rather than a half-purged row (`service.go`
  transaction).
- **Cross-project safety by construction.** Scoped `FindByID`/`UpdateStatus` and
  the purge-record's own stored scope prevent a caller from purging — or even
  detecting — another project's data (`purge_repository.go` contract,
  `service.go` scope handling).
- **Undiscoverable after purge.** Tombstoning the content *and* nulling the
  `search_vector` removes the record from both direct reads (status `purged`) and
  FTS results (`WipeContent` + `Remove`).

### Defers / costs

- **Memory-only targets in Phase 1.** Despite a broader `target_type` CHECK, only
  memory records can be purged today (see TODO above).
- **Partial artifact invalidation.** Embedding, cache, and derived-record
  invalidation are reserved fields, not executed in Phase 1 (see TODO above) —
  acceptable because embeddings are a no-op stub
  ([ADR-0002](0002-retrieval-hybrid.md)) and derived workers are Phase 2
  (design §10).
- **No standalone purge spec yet.** Operational guidance (threat model,
  compliance mapping, runbook) is not written; `docs/security-purge.md` is empty
  (see TODO above).
- **Event delivery is in-process.** Purge events use the Phase 1 in-process
  publisher; there is no transactional outbox (design §10), so a crash after
  commit but before publish can drop the `purge.executed` notification.

## References

- [Design §2.5, §7](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`internal/domain/purge/purge.go`](../../internal/domain/purge/purge.go)
- [`internal/application/security/service.go`](../../internal/application/security/service.go)
- [`internal/adapters/outbound/persistence/memory_pg.go`](../../internal/adapters/outbound/persistence/memory_pg.go) (`WipeContent`, lines 489–512)
- [`internal/adapters/outbound/search/postgres_fts.go`](../../internal/adapters/outbound/search/postgres_fts.go) (`Remove`, lines 35–39)
- [`internal/ports/outbound/purge_repository.go`](../../internal/ports/outbound/purge_repository.go)
- [`internal/domain/shared/enums.go`](../../internal/domain/shared/enums.go) (purge reasons/statuses, lines 157–192)
- [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql) (`purge_records`, lines 281–307)
- [`docs/security-purge.md`](../security-purge.md) — empty placeholder; see TODO(verify)
- Related: [ADR-0001 Memory types](0001-memory-types.md), [ADR-0003 Storage choice](0003-storage-choice.md), [ADR-0002 Retrieval (hybrid)](0002-retrieval-hybrid.md)
