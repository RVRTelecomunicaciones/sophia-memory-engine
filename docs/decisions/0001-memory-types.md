# 1. Memory Type Taxonomy — six aggregates, episodic/semantic split inside memory

## Status

Accepted — implemented in Phase 1.

Evidence of implementation:
the six aggregate packages under
[`internal/domain/`](../../internal/domain/) —
[`memory`](../../internal/domain/memory/memory.go),
[`decision`](../../internal/domain/decision/decision.go),
[`heuristic`](../../internal/domain/heuristic/heuristic.go),
[`relation`](../../internal/domain/relation/relation.go),
[`purge`](../../internal/domain/purge/purge.go),
and [`projectprofile`](../../internal/domain/projectprofile/profile.go) —
plus the enum value objects in
[`internal/domain/shared/enums.go`](../../internal/domain/shared/enums.go)
and the matching tables in
[`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql).

## Context

The memory engine serves `sophia-orchestator`'s heterogeneous agents (coding,
review, QA, planning, docs — design §1.1) and must hold different *kinds* of
knowledge that have genuinely different lifecycles, invariants, and retrieval
behavior. A single flat "memory row" would erase those differences. The forces
that shaped the taxonomy (per
[design §1.2, §2.1–§2.6](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md))
were:

- **Distinct lifecycles per knowledge kind.** Decisions move through
  `active → superseded → contradicted → archived` and *never* delete (design
  §2.2); heuristics have an enabled-version-per-key rule and temporal expiry
  (design §2.3); memories are `active → archived → purged` (design §2.1,
  `internal/domain/shared/enums.go` lines 20–36). These cannot share one status
  machine.
- **Episodic vs. semantic memory behave differently.** "Episodic: what
  happened, when, in what context. MUST have `ValidFrom`. Importance decays with
  time. Semantic: derived knowledge, pattern, convention. `ValidFrom` optional,
  importance more stable" (design §2.1). This is a within-aggregate distinction,
  not a separate aggregate.
- **Differentiated aggregates over a fat shared model.** "Shared-kernel domain
  with differentiated aggregates … shared kernel must NOT contain
  aggregate-specific business logic. Unified read model for retrieval lives in
  the application layer, not domain" (design §1.2).
- **Evidence-bearing decisions.** A decision is only meaningful with at least one
  evidence reference — an explicit invariant, not a convention
  (`internal/domain/decision/decision.go` lines 74–80, design §2.2).
- **Relations are edges, not nodes.** Relations connect any two aggregates with a
  typed, directed edge and carry no business-rule payload of their own
  (design §2.4).
- **Purge and project-profile are first-class, not side tables.** Purge is the
  authoritative compliance audit trail (design §2.5) and the project profile is
  *derived* knowledge that is regenerated, never hand-edited (design §2.6).

## Decision

Model **six domain aggregates** in their own packages under
`internal/domain/`, with a **shared kernel of value objects** in
`internal/domain/shared`, and split the `memory` aggregate into **two memory
types** (`episodic`, `semantic`) rather than two aggregates.

Concretely:

- **Six aggregates, one package each.** `memory`, `decision`, `heuristic`,
  `relation`, `purge`, `projectprofile` — each with its own entity, constructor,
  invariants, and lifecycle methods (the package list above). The shared kernel
  holds only common value objects (`RecordID`, `Scope`, `Provenance`,
  `TemporalMetadata`, `Confidence`, `ImportanceScore`, `EvidenceRef`, `TimeRange`
  — design §1.2 / §3).
- **`MemoryType` = `episodic | semantic`** as a value object, not separate
  tables. The split lives in
  [`internal/domain/shared/enums.go`](../../internal/domain/shared/enums.go)
  lines 3–18, and the episodic-requires-`ValidFrom` invariant is enforced in the
  constructor (`internal/domain/memory/memory.go` lines 128–135) and again at the
  database level (`CONSTRAINT chk_episodic_valid_from`, design §6 / migration 001
  line 486).
- **Decision taxonomy: keyed, versioned, evidence-backed.** `DecisionKey` groups
  versions over time; `Status` is `active | superseded | contradicted | archived`
  with no transition *to* active and no deletion (`decision.go` lines 107–143,
  enums lines 38–55, design §2.2). At least one `EvidenceRef` is required
  (`decision.go` lines 74–80).
- **Heuristic taxonomy: keyed, versioned, toggleable, expirable.** One enabled
  version per `HeuristicKey + Scope`; `IsActive` combines `Enabled` with temporal
  non-expiry (`heuristic.go` lines 126–137, design §2.3).
- **Relation taxonomy: eight typed edges.** `RelationType` ∈ `{relates_to,
  depends_on, supersedes, contradicts, references, derived_from, resolves,
  extends}` (enums lines 132–155); self-references are rejected at construction
  (`relation.go` lines 46–52). Metadata is auxiliary only — "if a metadata field
  is used programmatically for business decisions, it must be promoted to an
  explicit entity field" (design §2.4).
- **Purge taxonomy: three reasons, four states.** `PurgeReason` ∈ `{secret_leak,
  compliance, sensitive_content}` and `PurgeStatus` ∈ `{pending, executing,
  executed, failed}` (enums lines 157–192). See [ADR-0006](0006-security-purge.md).
- **Project profile is derived-only.** A regenerated snapshot of decisions,
  heuristics, patterns, and architecture signals with its own freshness state
  (`profile.go` lines 33–86, design §2.6).

> TODO(verify): The design and code establish *what* aggregates exist and *that*
> the memory split is `episodic`/`semantic`, with per-aggregate forces. I did not
> find a dedicated prose section weighing rejected alternatives — e.g. "one
> polymorphic `records` table vs. six aggregates" or "episodic/semantic as two
> aggregates vs. one typed aggregate". The rationale above is reconstructed from
> design §1.2 and §2.1–§2.6, not from a dedicated trade-off section. Confirm with
> the design authors before treating this as the canonical alternatives record.

## Consequences

### Enables

- **Lifecycle invariants live where they belong.** Each aggregate owns its own
  state machine and constructor validation, so illegal transitions (e.g.
  reviving an archived decision, enabling two heuristic versions) are impossible
  by construction rather than by convention (decision/heuristic methods cited
  above).
- **Episodic/semantic differentiation without table sprawl.** One `memories`
  table with a `type` discriminator carries both kinds; the
  `chk_episodic_valid_from` constraint and the constructor guard keep the
  episodic invariant honest at both layers.
- **Cross-aggregate linking via one edge type.** Because relations are a separate
  aggregate of typed edges, any aggregate can be connected to any other without
  embedding foreign keys into each entity (design §2.4; see also
  [ADR-0003](0003-storage-choice.md) on application-layer referential integrity).
- **Derived knowledge is clearly separated from authored knowledge.** Project
  profiles are explicitly regenerable and never hand-edited, so consumers can
  trust them as a cache, not a source of truth (design §2.6).

### Defers / costs

- **No FK-enforced cross-aggregate integrity.** Because relations point at
  memories, decisions, *or* heuristics, `source_id`/`target_id` carry no foreign
  keys; integrity is an application-layer responsibility — a documented trade-off
  (design §2.4 / §6, migration 001; see [ADR-0003](0003-storage-choice.md)).
- **Episodic/semantic is a soft, type-level distinction.** The split is a single
  enum column plus one constraint, not separate storage or retrieval paths, so
  any future divergence (e.g. separate decay models) must be layered on top
  rather than being structurally separate.
- **`tenant_id` is structural only in Phase 1.** It exists across the shared
  `Scope` value object and every table but is "not operationally active in phase
  1" (design §1.1) — multi-tenant enforcement is deferred to Phase 2 (design §10).
- **`projectprofile` generation is manual in Phase 1.** It is invocable but not
  worker-driven; `StaleAfter` is recorded but not yet a trigger (design §2.6, P3
  in design §10).

## References

- [Design §1.1–§1.2, §2.1–§2.6, §3, §6, §10](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`internal/domain/shared/enums.go`](../../internal/domain/shared/enums.go)
- [`internal/domain/memory/memory.go`](../../internal/domain/memory/memory.go)
- [`internal/domain/decision/decision.go`](../../internal/domain/decision/decision.go)
- [`internal/domain/heuristic/heuristic.go`](../../internal/domain/heuristic/heuristic.go)
- [`internal/domain/relation/relation.go`](../../internal/domain/relation/relation.go)
- [`internal/domain/projectprofile/profile.go`](../../internal/domain/projectprofile/profile.go)
- [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql)
- Related: [ADR-0003 Storage choice](0003-storage-choice.md), [ADR-0002 Retrieval (hybrid)](0002-retrieval-hybrid.md), [ADR-0006 Security purge](0006-security-purge.md)
