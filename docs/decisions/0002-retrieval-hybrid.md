# 2. Retrieval — hybrid lexical (FTS + trigram + scope + temporal + graph), no vector in Phase 1

## Status

Accepted — implemented in Phase 1.

Evidence of implementation:
the FTS search adapter
[`internal/adapters/outbound/search/postgres_fts.go`](../../internal/adapters/outbound/search/postgres_fts.go),
the composite ranking pipeline
[`internal/application/retrieval/ranking.go`](../../internal/application/retrieval/ranking.go)
and search service
[`internal/application/retrieval/search.go`](../../internal/application/retrieval/search.go),
the no-op embedding adapter
[`internal/adapters/outbound/embeddings/noop.go`](../../internal/adapters/outbound/embeddings/noop.go),
and the tuning record in
[`docs/retrieval-tuning.md`](../retrieval-tuning.md).

## Context

`sophia-orchestator`'s agents need to find the *right* memories, decisions, and
heuristics for a task, where content and queries are predominantly
Latin-American Spanish but laced with English technical terms, acronyms, and
identifiers. The forces that shaped the retrieval decision (per
[design §1.3, §8](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md))
were:

- **Hybrid is explicitly redefined for Phase 1.** "Phase 1 retrieval = FTS +
  trigram + scope + temporal + graph expansion. No vector similarity … Phase 1
  'hybrid' = multiple non-vector signals combined, NOT FTS + vector" (design
  §1.3). The word "hybrid" here means *many lexical/structural signals*, not
  lexical-plus-semantic.
- **Stemming alone is not enough.** A Spanish stemmer mishandles proper nouns,
  acronyms, mixed-language technical terms, and typos, so a fuzzy fallback is
  required: "`pg_trgm` as fallback for technical English terms, typos, partial
  matches" (design §1.3).
- **Multiple signals must be combined explainably.** Phase 1 ranks on seven named
  signals (FTS relevance, trigram similarity, recency, importance, type boost,
  freshness, scope exactness) with configurable weights, and every result must
  carry a per-signal explanation (design §8 signals table, "EXPLAIN — attach
  RankingResponse per result").
- **Embeddings are deferred, not designed out.** "Embedding port exists as
  interface with noop stub, no use case depends on it" (design §1.3); vector
  search is a conscious Phase 1 exclusion justified by "FTS+trigram sufficient
  for initial volume" (design §10).
- **One hot path dominates the orchestator load.** Its apply phase calls
  `FindActiveByTopicKey(scope, topic_key)` once per phase and must resolve in
  microseconds on an index ([`docs/retrieval-tuning.md`](../retrieval-tuning.md)
  "Scope").

## Decision

Implement **hybrid lexical retrieval inside PostgreSQL** for Phase 1: full-text
search (`tsvector`/`tsquery`) plus `pg_trgm` trigram fuzzy matching as the
candidate set, refined by scope/status/temporal SQL filters, then ranked by a
configurable composite of seven signals — and provide the embedding port only as
a no-op stub.

Concretely:

- **FTS + trigram candidate generation in one SQL query.** The search adapter
  matches `search_vector @@ plainto_tsquery('spanish', $1) OR similarity(content,
  $1) > $3` with a default trigram threshold of `0.3`
  (`internal/adapters/outbound/search/postgres_fts.go` lines 46–47, 108–120). It
  returns `ts_rank`, `similarity()`, and a `ts_headline` snippet per row
  (lines 109–113).
- **Spanish stemming with trigram fallback.** The query path defaults to
  `plainto_tsquery('spanish', …)` and `ts_headline('spanish', …)`
  (`postgres_fts.go` lines 110–113), matching the Spanish-first FTS default
  (design §1.5; per-record override via `fts_language`, see
  [ADR-0003](0003-storage-choice.md)).
- **Scope, type, and temporal filters in the WHERE clause.** Scope filters
  (`tenant_id`, `repo_id`, `agent_id`, `session_id`, `environment`), a `type =
  ANY` filter, and a `created_at` time-range filter are appended only when
  present (`postgres_fts.go` lines 54–93), implementing the QUALIFY stage of the
  pipeline (design §8 "Search pipeline").
- **Composite, configurable ranking.** `ComputeFinalScore` is a weighted linear
  sum of seven signals — FTS, trigram, recency, importance, type boost,
  freshness, scope exactness — using `config.RankingWeights`, not hardcoded
  constants (`internal/application/retrieval/ranking.go` lines 61–78; weights map
  to the design §8 signals table: FTS 0.25, trigram 0.15, recency 0.12,
  importance 0.13, type boost 0.10, freshness 0.10, scope exactness 0.15).
- **Workload tuning lives in config, observable through existing DTO fields.**
  Three `sdd_*`-workload signals — `TopicKeyBoost` (1.5×), `SDDTypeIncrement`
  (+0.10), `TruncatedSnippetPenalty` (0.85×) — were added without changing the
  HTTP response shape ([`docs/retrieval-tuning.md`](../retrieval-tuning.md)
  "Ranking tuning" / "Public API shape — unchanged").
- **Vector search is a port with a no-op adapter.** The embedding port's only
  implementation always returns `ErrEmbeddingsNotConfigured`, and no use case
  calls it (`internal/adapters/outbound/embeddings/noop.go` lines 11–28); the
  schema reserves the activation point as a comment (`-- Phase 2: CREATE
  EXTENSION IF NOT EXISTS vector;`, design §6).

> TODO(verify): The implementation in `search.go` notes that the topic-key boost
> is *approximated* — there is no per-result side-channel lookup of the record's
> real `topic_key`, so the boost fires on a verbatim snippet match gated by an
> `sdd_*` type filter (`internal/application/retrieval/search.go` lines 38–49).
> `docs/retrieval-tuning.md` describes the boost as if it targets exact topic-key
> lookups. These are consistent in intent but the code is an approximation of the
> documented behavior — treat the CODE as authoritative and confirm the
> documented framing with the retrieval authors.

> TODO(verify): The design and tuning docs justify *why* lexical signals are used
> and *why* vectors are deferred ("sufficient for initial volume"), but I did not
> find a quantitative comparison of FTS+trigram recall/precision vs. an embedding
> baseline on this corpus. The "sufficient" claim is an engineering judgment
> recorded in design §10, not a measured benchmark. Confirm before treating it as
> empirically validated.

## Consequences

### Enables

- **Explainable ranking with no black box.** Every signal is SQL- or
  Go-derived (`ts_rank`, `similarity()`, recency, importance, type boost,
  freshness, scope exactness), so each result can expose a per-signal `Ranking`
  explanation (design §8 step 5; `ranking.go` weighted sum).
- **Robustness to Spanish-stemmer blind spots.** The trigram fallback surfaces
  acronyms, identifiers, and typos that `plainto_tsquery('spanish', …)` drops,
  using a GIN trigram index (design §1.3; see [ADR-0003](0003-storage-choice.md)).
- **Single-query hybrid, no cross-store sync.** FTS, trigram, scope, type, and
  temporal filters all run as one SQL statement against Postgres
  (`postgres_fts.go`), and the orchestator hot path resolves in ~0.035 ms with a
  measured ~689× index-vs-seq-scan speedup
  ([`docs/retrieval-tuning.md`](../retrieval-tuning.md) "Benchmarks").
- **Tunable per workload without API churn.** Ranking weights and the `sdd_*`
  boosts are config-driven and observable through existing response fields, so
  retuning needs no schema or DTO migration (retrieval-tuning.md "How to retune").

### Defers / costs

- **No semantic similarity in Phase 1.** Conceptually-related-but-lexically-
  different content will not match; the `vector` extension and embedding adapter
  are Phase 2 (design §10, `noop.go`).
- **Spanish-default coupling.** The query path hardcodes `'spanish'` in the
  adapter (`postgres_fts.go` lines 110–113) even though records carry a per-row
  `fts_language`. Non-Spanish queries lean on the trigram fallback. The design
  itself calls this "a pragmatic phase 1 decision, not a complete multilanguage
  solution" (design §1.5).
- **Phase 1 search adapter covers only `memories`.** Decisions and heuristics are
  intended to be folded in via `UNION ALL` at the service layer
  (`postgres_fts.go` lines 41–44, design §8 "Search pipeline"); the adapter
  itself queries the `memories` table only.
- **Approximated topic-key boost.** The `sdd_*` topic-key boost is a conservative
  snippet-based approximation, not an exact key match (see TODO above) — it lifts
  exact-key lookups but can miss when the key is not echoed verbatim in the
  snippet.

## References

- [Design §1.3, §1.5, §6, §8, §10](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`internal/adapters/outbound/search/postgres_fts.go`](../../internal/adapters/outbound/search/postgres_fts.go)
- [`internal/application/retrieval/ranking.go`](../../internal/application/retrieval/ranking.go)
- [`internal/application/retrieval/search.go`](../../internal/application/retrieval/search.go)
- [`internal/adapters/outbound/embeddings/noop.go`](../../internal/adapters/outbound/embeddings/noop.go)
- [`docs/retrieval-tuning.md`](../retrieval-tuning.md)
- Related: [ADR-0003 Storage choice](0003-storage-choice.md), [ADR-0001 Memory types](0001-memory-types.md)
