# 4. Temporal validity and the graph as an in-SQL adjacency list

## Status

Accepted — implemented in Phase 1.

Evidence of implementation:
the `relations` table and the temporal columns across aggregates in
[`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql),
the recursive-CTE traversal in
[`internal/adapters/outbound/persistence/relation_pg.go`](../../internal/adapters/outbound/persistence/relation_pg.go),
and the temporal value object
[`internal/domain/shared/temporal.go`](../../internal/domain/shared/temporal.go).

## Context

The memory engine must answer two questions that go beyond a flat keyed lookup:
"what was true *at a given time*" and "what is *connected to* this record".
Both are part of the Phase 1 retrieval definition — "FTS + trigram + scope +
temporal + graph expansion. No vector similarity"
([design §1.3](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)).
The forces that shaped the temporal-and-graph decision were:

- **Records have a validity window, not just a creation time.** Every temporal
  aggregate carries `valid_from` / `valid_until` plus `last_accessed` and a
  cached `freshness` level. The value object
  `TemporalMetadata{ValidFrom, ValidUntil, LastAccessed, Freshness}` is the
  canonical shape (design §3, `internal/domain/shared/temporal.go` lines 36–42),
  persisted as `timestamptz` columns (design §6 line 266).
- **Supersession is versioning, not deletion.** Knowledge evolves by adding a
  new version that takes over from the previous one, which is retained. A
  decision moves `active → superseded` "(new version with same DecisionKey takes
  over)" (design §2 line 109), and "must never disappear silently"
  ([`docs/domain-invariants.md`](../domain-invariants.md) DecisionRecord
  invariants). Heuristics follow the same pattern — only one version may be
  `enabled=true` per key+scope, and creating a new enabled version disables the
  prior one in the same transaction (design §2 line 142).
- **Validity must be queryable as a point-in-time filter.** The active-version
  rule is expressed directly as a temporal predicate: `… AND (valid_until IS NULL
  OR valid_until > now()) ORDER BY version DESC LIMIT 1` (design §2 line 140).
- **Relations cross aggregates and must be traversable, bounded.** Relations
  link memories, decisions, *or* heuristics. Traversal must be "bounded by depth
  or budget" (`docs/domain-invariants.md` Relation invariants), and the design
  fixes the shape: "Adjacency list in PostgreSQL with recursive CTEs … Traversal
  bounded by `max_depth` (hard cap = 3) … Cycle prevention via path tracking in
  CTE … Scope and temporal filters applied during traversal" (design §1.4).
- **A graph database would add a second store for one query shape.** Phase 1
  deliberately ships a single datastore (see [ADR-0003](0003-storage-choice.md)),
  and a dedicated graph DB adapter is reserved for "Phase 2 … if volume demands"
  (design §1.4).

## Decision

Model **temporal validity** as per-record `valid_from` / `valid_until` windows
with explicit version+supersession state, and model **the graph** as an
adjacency-list `relations` table traversed by recursive CTEs inside PostgreSQL —
not as a dedicated graph database.

Concretely:

- **Temporal columns on every temporal aggregate.** `memories`, `decisions`,
  `heuristics`, and `relations` each carry `valid_from TIMESTAMPTZ` /
  `valid_until TIMESTAMPTZ` (migration 001 lines 44–45, 115–116, 183–184,
  252–253). `memories` and `decisions`/`heuristics` also carry `last_accessed`
  and a `freshness` enum (`fresh|aging|stale|expired`).
- **Freshness is computed from the validity window, not stored authoritatively.**
  `TemporalMetadata.ComputeFreshness(cfg, clock)` returns `expired` when
  `now > ValidUntil`, otherwise derives `fresh|aging|stale` from age since
  `LastAccessed` against configurable thresholds (default aging 7 d, stale 30 d)
  (`internal/domain/shared/temporal.go` lines 59–86). `IsExpired(clock)` is the
  point-in-time check (lines 89–94). All time comes through the `Clock` port so
  tests use `FixedClock` (lines 16–34; design §7 "All temporal tests use
  FixedClock — never time.Now() directly").
- **Supersession preserves history.** `Decision.Supersede(by)` flips status
  `active → superseded` and records `SupersededBy`, refusing if not active
  (`internal/domain/decision/decision.go` lines 107–118). At the schema level the
  invariant is enforced by `CONSTRAINT chk_superseded_has_ref CHECK (status !=
  'superseded' OR superseded_by IS NOT NULL)` and a `superseded_by` self-reference
  (migration 001 lines 122, 126). Versioning uniqueness is enforced by
  `idx_decisions_version_scope` over `(decision_key, project_id, …, version)`
  (migration 001 lines 130–131); heuristics mirror this with a partial unique
  index `idx_heuristics_active … WHERE enabled = true` (migration 001 lines
  200–202).
- **Graph as an adjacency list.** `relations(source_id, target_id,
  relation_type, …)` with a fixed `relation_type` CHECK set (`relates_to`,
  `depends_on`, `supersedes`, `contradicts`, `references`, `derived_from`,
  `resolves`, `extends`), `CONSTRAINT chk_no_self_ref CHECK (source_id !=
  target_id)`, and `UNIQUE(source_id, target_id, relation_type)`
  (migration 001 lines 237–258). Indexed on both endpoints
  (`idx_relations_source_id`, `idx_relations_target_id`) and on `relation_type`
  (lines 265–269).
- **Traversal via recursive CTE, hard-capped at depth 3.**
  `RelationPgRepository.Traverse` builds a `WITH RECURSIVE graph AS (…)` query
  (`internal/adapters/outbound/persistence/relation_pg.go` lines 438–470). Depth
  is clamped to `maxTraverseDepth = 3` and total nodes to `maxTraverseResults =
  100` (lines 21–24, 266–273), the recursion stops at `WHERE g.depth < $depth`
  (line 461), and cycles are prevented with a `path` array — `AND NOT
  r.target_id = ANY(g.path)` (lines 386–394). Scope (`project_id` required) and
  an optional point-in-time temporal filter — `(r.valid_from IS NULL OR
  r.valid_from <= $validAt) AND (r.valid_until IS NULL OR r.valid_until >
  $validAt)` — are applied inside the traversal (lines 284–320).

> TODO(verify): The design and code establish *what* was chosen — per-record
> validity windows, version+supersede state, and an in-SQL adjacency list capped
> at depth 3 — and the forces behind each. I did not find an explicit prose
> comparison framed as "temporal columns vs. a bitemporal/system-versioned table
> approach" or "adjacency-list-in-SQL vs. Neo4j/AGE/a dedicated graph store"
> evaluated as rejected alternatives. The rationale below is reconstructed from
> the design's stated forces (§1.3–§1.4) and the single-store decision in
> ADR-0003, not from a dedicated trade-off section. Confirm with the design
> authors before treating this as the canonical alternatives record.

## Consequences

### Enables

- **Point-in-time queries without history loss.** Because supersession sets state
  rather than deleting rows, and validity is a `valid_from`/`valid_until` window,
  the "active version at time T" question is a pure SQL predicate (design §2 line
  140) and full history remains queryable — satisfying the "must never disappear
  silently" invariant (`docs/domain-invariants.md`).
- **Graph traversal on the same store.** Relation expansion runs as one recursive
  CTE against Postgres with scope and temporal filters inline — no second
  database, no cross-store sync (`relation_pg.go` lines 438–470). This is the
  graph half of the single-query-plane benefit recorded in
  [ADR-0003](0003-storage-choice.md).
- **Deterministic temporal tests.** Freshness/expiry flow through the `Clock`
  port, so `FixedClock` makes temporal behavior reproducible
  (`temporal.go` lines 16–34).
- **Bounded, safe traversal by construction.** The depth cap (3), result cap
  (100), and `path`-array cycle guard are enforced in the adapter, so a malformed
  or cyclic graph cannot produce unbounded queries (`relation_pg.go` lines 21–24,
  386–394, 461).

### Defers / costs

- **No graph database in Phase 1.** Deep or high-fanout traversal is limited to
  `max_depth = 3`; a dedicated graph DB adapter is explicitly reserved for "Phase
  2 … if volume demands" (design §1.4).
- **Purge exclusion is not done in SQL.** Because relations are cross-aggregate
  (targets live in `memories`, `decisions`, or `heuristics`), the traversal
  cannot filter purged targets in-query; that check is an application-layer
  responsibility after traversal (`relation_pg.go` lines 258–262), consistent
  with the application-layer referential-integrity trade-off in
  [ADR-0003](0003-storage-choice.md).
- **No database-enforced FK between a relation and its endpoints.** Same
  cross-aggregate reason — `source_id`/`target_id` carry no foreign keys
  (migration 001 lines 260–262).
- **Freshness can drift until recomputed.** `freshness` is a cached column;
  truth comes from `ComputeFreshness` at read time. The cached value may lag
  reality between writes (`temporal.go` lines 59–86).

> TODO(verify): Phase 1 recency ranking uses `created_at` "for simplicity",
> with the design noting Phase 2 "may evolve to `COALESCE(valid_from,
> created_at)`" (design §714). Whether retrieval ranking should already prefer
> `valid_from` is an open question to confirm with the retrieval owners.

## References

- [Design §1.3–§1.4 (retrieval + graph model), §2 (decision/heuristic/relation entities), §3 (TemporalMetadata), §6 (schema), §7 (FixedClock)](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql) — relations table (lines 237–258), temporal columns (lines 44–45, 115–116, 183–184, 252–253), supersede constraints/indexes (lines 122, 126, 130–131, 200–202)
- [`internal/adapters/outbound/persistence/relation_pg.go`](../../internal/adapters/outbound/persistence/relation_pg.go) — recursive-CTE traversal, depth cap, cycle prevention
- [`internal/domain/shared/temporal.go`](../../internal/domain/shared/temporal.go) — `TemporalMetadata`, `ComputeFreshness`, `IsExpired`, `Clock`
- [`internal/domain/decision/decision.go`](../../internal/domain/decision/decision.go) — `Supersede` transition
- [`docs/domain-invariants.md`](../domain-invariants.md) — Decision and Relation invariants
- Related: [ADR-0003 Storage choice](0003-storage-choice.md), [ADR-0005 Project DNA](0005-project-dna.md)
