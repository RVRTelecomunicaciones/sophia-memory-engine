# ADR-0007: Consolidation Worker Pipeline (M2)

## Status

Accepted

## Context

When the sophia-orchestrator archives a change (`phase.archived` event), the memory-engine
must consolidate the learning record: update skill metrics in the orchestrator, evaluate
whether skills should be promoted or demoted, emit governance activation proposals for
qualified skills, and write a deterministic YAML digest to long-term memory.

This processing must be:

- **Idempotent** — re-receiving the same change_id is a no-op.
- **Fault-tolerant** — a single skill failure must not abort the entire batch; the digest
  must still be written.
- **LLM-free** — the consolidation path may not import any LLM client (D-M2-12).
- **Bounded** — the HTTP webhook response returns 202 immediately; all processing happens
  in a goroutine with `context.Background()` so the caller is never blocked.

## Decision

### 1. Webhook receiver lives in the main HTTP server (cmd/memory-engine)

The `POST /api/v1/worker/phase-archived` endpoint is registered in
`internal/adapters/inbound/http/server.go` alongside all other API routes.

`cmd/workers` stays minimal (lifecycle management only) pending M3+ scheduler work.

Rationale: the memory-engine HTTP server already owns auth middleware, tracing, and the
ingest service. Registering the webhook here avoids duplicating that stack in a second
binary and keeps the operational footprint small.

### 2. HandlerV2 pipeline (internal/application/consolidation)

The M2 pipeline executes these steps in order:

1. **Idempotency guard** — check `digest/{change_id}` existence via `MemoryClient.HasTopic`.
   Return immediately if already processed.
2. **GetUsage** — fetch all `SkillUsageRow` records for this change from the orchestrator.
3. **computeDeltas** — aggregate per-skill outcome counts; compute
   `avg_retry_reduction = (1.5 - apply_attempts) / 1.5` (D-M2-11 proxy).
4. **PatchMetrics loop** — PATCH each skill's metrics in the orchestrator; per-skill errors
   are logged and skipped (pipeline continues). Panics are recovered via defer/recover.
5. **GetSkill** — read the post-patch skill snapshot.
6. **Promoter evaluation** — `candidate → validated` when risk-level thresholds are met
   (ADR §6.1 table: low-risk requires success ≥ 1 + tests ≥ 1 + failure = 0; medium/high/
   critical additionally require avg_retry_reduction ≥ 0.20).
7. **Demoter evaluation** — `active → blocked` when failure_ratio > 15 %; `active →
   deprecated` when avg_retry_reduction drops below 5 % (requires at least one data point).
8. **Proposer emission** — `validated` skills with usage_count ≥ 5 receive a governance
   activation proposal written to `governance/skill-proposal/{skill_id}` in memory-engine.
   Re-emission merges evidence_changes without duplication.
9. **BuildDigest + Ingest** — a deterministic YAML digest is written to
   `digest/{change_id}` in memory-engine (topic_key upsert, type=semantic).

### 3. Port/adapter structure

- **Port** (`internal/ports/outbound/skills_client.go`): `SkillsClient` interface with
  `PatchMetrics`, `PatchStatus`, `GetSkill`, `GetUsage`. No LLM types anywhere in this
  tree.
- **Adapter** (`internal/adapters/outbound/orchhttp/`): HTTP implementation with
  exponential-backoff retry (100 ms → 500 ms → 2.5 s, factor 5.0, 3 attempts). 4xx errors
  are not retried. The adapter requires a non-empty API key at construction time.
- **MemoryClient** (`internal/application/consolidation/memory_client.go`): narrow
  three-method interface (`HasTopic`, `ReadContent`, `Ingest`) bridging the consolidation
  domain to the ingest service.
- **MemoryServiceClient** (`internal/application/consolidation/memory_client_adapter.go`):
  production adapter implementing `MemoryClient` by delegating to `inbound.MemoryService`.

### 4. Activation via environment variable

`SOPHIA_MEMORY_WORKER_ENABLED=true` enables the pipeline at startup.
`SOPHIA_ORCH_BASE_URL` and `SOPHIA_ORCH_API_KEY` must also be set.
When the variable is absent or false, the `/api/v1/worker/phase-archived` route is not
registered (nil pipeline guard in `NewRouter`).

### 5. LLM-free enforcement (D-M2-12)

A unit test `TestNoLLMImportsInConsolidation` uses `go list -deps` to assert that no
import path containing `openai`, `anthropic`, `llm`, `gpt`, or `claude` appears in the
transitive dependency closure of the consolidation package.

## Consequences

- Memory-engine gains a new write path activated by env flag; no schema changes required.
- The orchestrator's skills API is the authority for skill state; memory-engine reads it
  after each metrics patch and does not maintain a local copy.
- Skill promotion/demotion decisions are made per-change-archive, not continuously; latency
  is bounded by the orchestrator round-trip.
- The proposer requires `ReadContent` on `MemoryClient` to merge evidence_changes on
  re-emission; this is the only read-back path in the consolidation domain.
- `cmd/workers` is intentionally left minimal for M3+ scheduled jobs.
