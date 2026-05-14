# Retrieval Tuning — ADR-0005 P2.3

This document captures the retrieval hot-path audit, the index choice, the
ranking-weight tuning, and the empirical evidence that backs both.

## Scope

`sophia-orchestator`'s apply phase calls `FindActiveByTopicKey(scope, topic_key)`
once per phase load. That call must use an index and must complete in
microseconds — the orchestator's apply-phase load path is dominated by it.

This document covers:

1. The hot query and its plan.
2. The decision NOT to add a new index, with EXPLAIN evidence.
3. The ranking-weight tuning for the `sdd_*` workload.
4. Benchmark numbers (pure-SQL speedup ratio).
5. Where the knobs live.

## The hot query

Defined in
[`internal/adapters/outbound/persistence/memory_pg.go`](../internal/adapters/outbound/persistence/memory_pg.go)
as `MemoryPgRepository.FindActiveByTopicKey`:

```sql
SELECT
    id, type, content, summary, tags, topic_key, fts_language,
    tenant_id, project_id, repo_id, agent_id, session_id, environment,
    source, source_uri, ingest_method, parent_id,
    valid_from, valid_until, last_accessed, freshness,
    importance_score, importance_computed_at, importance_factors,
    status, archived_by, archive_reason,
    created_at, updated_at
FROM memories
WHERE topic_key  = $1
  AND status     = 'active'
  AND project_id = $2
  AND (tenant_id IS NULL OR tenant_id = $3)
LIMIT 1
```

## Index decision: NO new migration needed

The existing `idx_memories_topic_key` (migration 001) already serves the hot
path optimally:

```
CREATE INDEX idx_memories_topic_key ON memories (topic_key) WHERE topic_key IS NOT NULL;
```

EXPLAIN evidence (run on PostgreSQL 16-alpine, 10,000 active rows + 2,000
archived rows, after `ANALYZE memories`):

```
Limit  (cost=0.29..8.31 rows=1 width=27) (actual time=0.023..0.024 rows=1 loops=1)
  Buffers: shared hit=3
  ->  Index Scan using idx_memories_topic_key on memories
        (cost=0.29..8.31 rows=1 width=27) (actual time=0.022..0.023 rows=1 loops=1)
        Index Cond: ((topic_key)::text = 'sdd/p2.3/topic-00042'::text)
        Filter: ((tenant_id IS NULL)
                  AND ((status)::text = 'active'::text)
                  AND ((project_id)::text = 'project-bench-042'::text))
        Buffers: shared hit=3
Planning Time: 0.393 ms
Execution Time: 0.035 ms
```

### Why this index — not the P1.3 partial unique index — wins

`idx_memories_topic_key_active_unique` (migration 004) is keyed
`(project_id, COALESCE(tenant_id,''), topic_key)`. The hot query has
`tenant_id IS NULL OR tenant_id = $3` — that predicate does NOT match
`COALESCE(tenant_id,'')`, so the planner cannot use the unique index's middle
column. It could in principle use `project_id` as the leading column, but
project_id alone has low selectivity in the simulated dataset (100 distinct
values across 10k rows = ~100 candidates per project), whereas `topic_key` is
near-unique (≈ 1 candidate per key). The planner correctly chooses the
high-selectivity partial index.

### Verdict

**No new migration is added.** The existing `idx_memories_topic_key` is the
canonical hot-path index. The P1.3 partial unique index continues to serve
its data-integrity role (preventing duplicate active `(project, tenant,
topic_key)` tuples) without competing for the hot-path query plan.

This is guarded by an integration regression test:
[`TestExplain_FindActiveByTopicKey_UsesIndex`](../internal/adapters/outbound/persistence/memory_pg_bench_test.go)
asserts that the plan contains `Index Scan`/`Bitmap Index Scan` (NOT `Seq Scan`)
on every migration-applied schema.

## Ranking tuning — `sdd_*` workload

The orchestator's audit/context-building queries are dominated by `sdd_*`
record types (proposals, specs, designs, tasks, etc.). Three signals were
added to the ranking pipeline
([`internal/application/retrieval/ranking.go`](../internal/application/retrieval/ranking.go)):

| Factor                    | Default | Trigger                                                        | Effect                                                |
|---------------------------|:-------:|----------------------------------------------------------------|-------------------------------------------------------|
| `TopicKeyBoost`           |  1.5×   | snippet contains the query string AND request targets `sdd_*`  | final score multiplied                                |
| `SDDTypeIncrement`        | +0.10   | record_type begins with `sdd_` AND was requested in `Types`    | added to TypeBoost signal BEFORE weighted linear sum  |
| `TruncatedSnippetPenalty` | 0.85×   | ts_headline snippet contains the `...` truncation marker       | final score multiplied                                |

### Rationale

- **Topic-key boost**: when the caller is doing an exact `topic_key` lookup
  (the orchestator's audit pattern), the FTS/TRGM signals are weak proxies.
  Promoting topic-key matches by 50 % pushes them above generic content
  matches without overwhelming truly excellent FTS hits.
- **SDD-type increment**: when the caller has explicitly filtered to
  `sdd_*` types, the type bump disambiguates within an already-filtered
  result set. It is gated on BOTH the prefix AND the filter — sdd_*
  records that surface in non-SDD queries do not get the bump.
- **Truncated-snippet penalty**: ts_headline output containing `...` signals
  fragmentation. Long-form full-content matches are preferred when the
  underlying content fits in the snippet window. A 15 % demotion is enough
  to flip ordering when other signals are close, but small enough not to
  bury a truly high-FTS-score fragmented hit.

### Public API shape — unchanged

The HTTP response (`searchResultDTO.Ranking`) keeps the same JSON shape. The
SDD-type increment is observable through the existing `type_boost` field
(which now stores `0.5 + 0.10 = 0.60` when the bump fires). The topic-key
boost and truncated-snippet penalty are observable through `final_score`.
No DTO fields were added.

## Benchmarks

Two complementary benchmarks live in
[`memory_pg_bench_test.go`](../internal/adapters/outbound/persistence/memory_pg_bench_test.go),
both behind the `integration` build tag:

### Go-side `Benchmark_FindActiveByTopicKey` (informational)

Full Go → pgx → Postgres → pgx → Go round-trip. Dominated by network and
pgx wire-protocol overhead on localhost.

```
Benchmark_FindActiveByTopicKey/indexed-8   ~  9,000  iters  ~232,000 ns/op
Benchmark_FindActiveByTopicKey/no_index-8  ~ 10,000  iters  ~225,000 ns/op
```

These are statistically indistinguishable, which **is the point**: at this
scale the index choice is dwarfed by RTT. The Go-side bench tells us the
end-to-end cost the caller experiences (~225 µs per call). The "≥ 3×
speedup" claim is unprovable here — both numbers are noise around the
network floor.

### Pure-SQL `TestSQL_HotPathSpeedup_IndexVsSeqScan` (authoritative)

This test runs both halves inside a server-side SQL function (5,000
lookups in one round-trip), eliminating network noise. The SQL function
is plain SQL (not PL/pgSQL), so the planner re-plans on every call and
honors the current session's `enable_indexscan` / `enable_bitmapscan`
flags.

```
indexed:    9.071 ms total,      1,814 ns/op
no_index: 6253.232 ms total, 1,250,646 ns/op
speedup:   689.37×
```

**Indexed plan = Index Scan using idx_memories_topic_key, 3 buffer hits, 0.035 ms.**
**No-index plan = Seq Scan on memories, 2 buffer hits per call, but multiplied across all rows of the table.**

Easily clears the ADR-0005 P2.3 acceptance threshold of ≥ 3×.

## How to retune

| Knob                          | Location                                                                           | Default |
|-------------------------------|------------------------------------------------------------------------------------|:-------:|
| `RankingWeights.TopicKeyBoost`           | `internal/infrastructure/config/config.go` → `DefaultConfig`            | `1.5`   |
| `RankingWeights.SDDTypeIncrement`        | same                                                                    | `0.10`  |
| `RankingWeights.TruncatedSnippetPenalty` | same                                                                    | `0.85`  |
| `snippetTruncationMarker`                | `internal/application/retrieval/ranking.go`                             | `"..."` |
| `sddTypePrefix`                          | same                                                                    | `"sdd_"`|

The weights live on `config.RankingWeights`, which is constructed once in
`DefaultConfig()` and threaded through `NewSearchService(..., cfg, clock)`
(R12 compliant — no globals). Override via your own `AppConfig` builder if
you need a different tuning per environment.

## Maintenance

If you change the hot-path query in `memory_pg.go`:

1. Run
   `go test -tags=integration -run TestExplain_FindActiveByTopicKey_UsesIndex
   ./internal/adapters/outbound/persistence/...`
   — it MUST still pass (Index Scan or Bitmap Heap Scan, never Seq Scan).
2. Run
   `go test -tags=integration -run TestSQL_HotPathSpeedup_IndexVsSeqScan
   ./internal/adapters/outbound/persistence/...`
   — speedup MUST stay ≥ 3×. If it drops, the plan regressed or the index
   no longer covers the query.

If you bump a ranking weight in `DefaultConfig`:

1. The unit tests in `ranking_test.go` use `baseWeights` (a local copy of
   the defaults). Update `baseWeights` alongside the config so the
   ratio-based assertions remain meaningful.
2. The DTO shape is fixed by `OpenAPI` and `inbound.RankingExplanation`. If
   you add a new factor, it must be observable through an existing field
   (e.g., apply it via a multiplier on `final_score` or fold it into one
   of the existing weighted signals) — do NOT add a new JSON key.
