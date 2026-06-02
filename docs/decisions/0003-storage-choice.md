# 3. Storage Choice — PostgreSQL as the single store

## Status

Accepted — implemented in Phase 1.

Evidence of implementation:
[`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql),
the Postgres persistence adapters under
[`internal/adapters/outbound/persistence/`](../../internal/adapters/outbound/persistence/),
and the FTS search adapter
[`internal/adapters/outbound/search/postgres_fts.go`](../../internal/adapters/outbound/search/postgres_fts.go).

## Context

The memory engine must persist six aggregates — memories, decisions, heuristics,
relations, purge records, and project profiles — and serve them back to
`sophia-orchestator` with retrieval that is full-text, fuzzy, scope-aware, and
temporally aware. The forces that shaped the storage decision (per
[design §1.2–§1.5](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
and the schema in design §6) were:

- **Multiple read patterns from one store.** Phase 1 retrieval is defined as
  "FTS + trigram + scope + temporal + graph expansion. No vector similarity"
  (design §1.3). The system needs stemmed full-text search, fuzzy/partial matching for
  technical English terms and typos, structured scope filtering
  (`project_id`/`tenant_id`/`repo_id`/…), temporal validity windows, and bounded graph
  traversal — all over the same records.
- **Graph without a graph database.** Relations are modeled as an adjacency list with
  recursive CTEs, `max_depth` capped at 3, rather than a dedicated graph store
  (design §1.4). A relational engine that supports recursive CTEs is therefore required.
- **Spanish-first full-text.** The default FTS language is `spanish` with a per-record
  override via an `fts_language` column, because the majority of content and queries are
  Latin-American Spanish (design §1.5).
- **Single operational target for the MVP.** Phase 1 deliberately ships one production
  datastore. A SQLite adapter and a dedicated vector store were both consciously deferred
  to Phase 2 (design §10 exclusions table).
- **Cross-aggregate references.** Relations point at memories, decisions, *or*
  heuristics, so a single foreign key cannot express the target — referential integrity
  for `relations.source_id`/`target_id` is enforced in the application layer instead
  (documented directly on the table:
  `migrations/postgres/001_initial_schema.up.sql` lines 260–262, citing design §6).

## Decision

Use **PostgreSQL 16** as the single authoritative store for Phase 1, and implement
retrieval inside Postgres using built-in full-text search plus the `pg_trgm` extension.

Concretely:

- **One relational store, no second database.** All six aggregates plus
  `domain_events`, `api_keys`, and `retrieval_feedback` live in Postgres. No separate
  graph DB and no separate vector DB in Phase 1.
- **Full-text search via `tsvector`/`tsquery`.** Each searchable table carries a
  `search_vector TSVECTOR` column maintained by a `BEFORE INSERT OR UPDATE` trigger that
  builds a weighted vector — e.g. for `memories`: summary (`A`), tags (`B`),
  content (`C`) — using the row's own `fts_language`
  (`migrations/postgres/001_initial_schema.up.sql` lines 77–91). A GIN index
  (`idx_memories_fts`) backs it.
- **Trigram fallback via `pg_trgm`.** `CREATE EXTENSION IF NOT EXISTS pg_trgm`
  (migration 001 line 8) plus a GIN trigram index on `content`
  (`idx_memories_trgm`) provides fuzzy/partial matching that the stemmer cannot —
  proper nouns, acronyms, mixed-language technical terms (see also
  [`docs/operations/OPERATIONS.md`](../operations/OPERATIONS.md) troubleshooting §4).
- **Spanish default, per-record override.** `fts_language REGCONFIG NOT NULL DEFAULT
  'spanish'` (migration 001 line 33); the query path uses
  `plainto_tsquery('spanish', …)` by default (design §1.5).
- **Graph as adjacency list.** A `relations` table with explicit `relation_type`
  (migration 001 lines 237–258), traversed by recursive CTEs in the persistence layer,
  rather than a graph database.
- **Vector search is a port with a no-op adapter.** The embedding port exists but no use
  case depends on it; `internal/adapters/outbound/embeddings/noop.go` is the only
  implementation, and the schema reserves the activation point as a comment
  (`-- Phase 2: CREATE EXTENSION IF NOT EXISTS vector;`, design §6 / migration 001).

> TODO(verify): The planning and design documents establish *what* was chosen (Postgres,
> FTS, trigram, adjacency-list graph) and the forces behind each, but I did not find an
> explicit prose comparison of "PostgreSQL vs. SQLite vs. a dedicated graph/vector store"
> framed as a rejected-alternatives evaluation. The rationale below is reconstructed from
> the design's stated forces and exclusions, not from a dedicated trade-off section.
> Confirm with the design authors whether a fuller alternatives analysis exists before
> treating this section as the canonical record.

## Consequences

### Enables

- **One query plane for hybrid retrieval.** FTS, trigram, scope filters, temporal
  filters, and graph traversal all run as SQL against one store — no cross-store joins
  or data sync. The hot-path lookup (`FindActiveByTopicKey`) resolves in ~0.035 ms on an
  index scan, and the index-vs-seq-scan speedup is measured at ~689× in a pure-SQL
  benchmark ([`docs/retrieval-tuning.md`](../retrieval-tuning.md)).
- **Operational simplicity for the MVP.** A single Postgres instance (deployed via the
  Helm chart, see `docs/operations/OPERATIONS.md`) is the only stateful dependency.
  Readiness is a single `db` ping.
- **Transactional integrity across aggregates.** A `TransactionManager`
  (`tx_manager_pg.go`) lets multi-step use cases (e.g. record-decision-with-relation)
  commit atomically.
- **Explainable ranking.** Because ranking signals are SQL-derived (`ts_rank`,
  `similarity()`, recency, importance, type boosts), the response can expose a
  per-signal `Ranking` explanation without a black-box model.

### Defers / costs

- **No semantic (vector) similarity in Phase 1.** Retrieval is lexical: FTS stems and
  trigram matches surface results, but conceptually-related-but-lexically-different
  content will not match. The `vector` extension and embedding adapter are Phase 2
  (design §10).
- **Graph traversal is bounded and in-SQL.** Recursive CTEs with a hard `max_depth = 3`
  cap are sufficient for Phase 1 volumes; a dedicated graph DB adapter is reserved for
  Phase 2 "if volume demands" (design §1.4).
- **No database-level referential integrity for relations.** Because relations span
  aggregates, `source_id`/`target_id` carry no foreign keys; integrity is an
  application-layer responsibility — a deliberate, documented trade-off
  (migration 001 lines 260–262).
- **Spanish-stemmer blind spots.** `plainto_tsquery('spanish', …)` drops out-of-dictionary
  tokens (acronyms, identifiers), which can yield empty FTS results; the trigram index
  and exact `topic_key` lookups are the documented mitigations
  (`docs/operations/OPERATIONS.md` §4).
- **No SQLite / embedded option in Phase 1.** A SQLite adapter was explicitly excluded;
  PostgreSQL is the production target (design §10). Local development therefore requires
  a Postgres instance (or the testcontainers-backed integration test harness).

## References

- [Design §1.2–§1.5, §6, §10](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql)
- [`migrations/postgres/004_memories_topic_key_unique.up.sql`](../../migrations/postgres/004_memories_topic_key_unique.up.sql)
- [`docs/retrieval-tuning.md`](../retrieval-tuning.md)
- [`docs/operations/OPERATIONS.md`](../operations/OPERATIONS.md)
- Related: [ADR-0002 Retrieval (hybrid)](0002-retrieval-hybrid.md), [ADR-0004 Temporal/graph](0004-temporal-graph.md)
