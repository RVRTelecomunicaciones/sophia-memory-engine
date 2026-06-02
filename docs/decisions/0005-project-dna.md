# 5. Project DNA — a derived project profile (designed, partially built)

## Status

**Accepted as a design; only partially implemented in Phase 1.**

What exists today:
- The domain entity
  [`internal/domain/projectprofile/profile.go`](../../internal/domain/projectprofile/profile.go).
- The inbound and outbound port interfaces
  [`internal/ports/inbound/profile_service.go`](../../internal/ports/inbound/profile_service.go)
  and
  [`internal/ports/outbound/profile_repository.go`](../../internal/ports/outbound/profile_repository.go).
- The `project_profiles` table in
  [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql)
  (lines 313–343).

What does **not** exist yet (Phase 2):
- **No persistence adapter.** There is no `profile_pg.go`; the
  `project_profiles` table "is present in `001_initial_schema.up.sql` but no Go
  adapter references [it] yet (Phase 1 scope)"
  ([`docs/migrations-coverage.md`](../migrations-coverage.md) §1, row
  `project_profiles | (no adapter yet)`).
- **No application service.** `internal/application/projectdna/` contains only a
  `.gitkeep` placeholder — the `ProjectProfileService.Generate` use case is not
  implemented.
- **No wiring.** `GenerateProjectProfile` is "Manual invocation only, not
  worker-driven" and the only P3 use case, marked simply "Manual invocation
  only" in the implementation-scope table (design §4 line 352, §10 line 803).

This ADR therefore records a **designed concept with a built domain core and a
reserved schema**, not a fully wired feature.

## Context

"Project DNA" is the engine's derived, project-level summary of accumulated
memory — the answer to "what does this project *know about itself*". The forces
that shaped it were:

- **It is derived knowledge, regenerated — never hand-edited.** The design is
  explicit: "Derived knowledge only — regenerated, never manually edited"
  (design §2.6 line 218). The schema reflects this by storing provenance of the
  generation itself (`generated_at`, `source_time_from`, `source_time_to`,
  `stale_after`, `source_counts`) rather than user-authored content
  (migration 001 lines 322–326).
- **It aggregates the other aggregates.** A profile points at the project's
  active decisions and top heuristics and records detected patterns and
  architectural signals — `ActiveDecisions []RecordID`, `TopHeuristics
  []RecordID`, `Patterns []PatternEntry`, `ArchSignals []string`
  (design §2.6 lines 200–203; `profile.go` lines 33–47).
- **It is versioned and has a staleness clock.** `version` plus
  `UNIQUE(project_id, version)` (migration 001 lines 316, 333) and a
  `FreshnessState{GeneratedAt, SourceTimeRange, StaleAfter, SourceCounts}`
  (design §2.6 lines 210–216; `profile.go` lines 25–31) so the engine can tell
  when a profile is out of date.
- **Generation is expensive / lowest priority for the MVP.** It is the single
  P3 use case and is scoped to manual invocation in Phase 1, with worker-driven
  regeneration (using `StaleAfter` as the trigger) deferred to Phase 2
  (design §2.6 line 218, §4 line 352).

## Decision

Define **ProjectProfile ("project DNA") as a first-class derived aggregate** in
the domain and reserve its schema and ports in Phase 1, while deferring the
generation service, persistence adapter, and worker automation to Phase 2.

Concretely:

- **Domain entity is implemented.** `ProjectProfile` carries `ID`, `ProjectID`,
  `Version`, `Summary`, `ActiveDecisions`, `TopHeuristics`, `Patterns`,
  `ArchSignals`, `Freshness`, `Scope`, timestamps; `NewProjectProfile(...)`
  validates a required `project_id` and seeds an empty profile with
  `StaleAfter = now + 24h` and an empty `SourceCounts`
  (`profile.go` lines 33–86). Supporting types `PatternEntry`, `SourceCounts`,
  and `FreshnessState` are defined alongside (lines 9–31).

  > TODO(verify): The 24-hour `StaleAfter` default in `NewProjectProfile`
  > (`profile.go` line 79) is not stated as a requirement anywhere in the design
  > I read (the design says only that Phase 2 uses `StaleAfter` as a worker
  > trigger). Confirm whether 24 h is an intentional default or a placeholder.

- **Ports are declared.** Inbound `ProjectProfileService` exposes
  `Generate(ctx, GenerateProfileCmd)` and `Get(ctx, GetProfileQuery)`
  (design §5.1 lines 393–395; `internal/ports/inbound/profile_service.go`);
  outbound `ProjectProfileRepository` exposes `Save` and `FindLatest`
  (design §5.2 line 412; `internal/ports/outbound/profile_repository.go` lines
  10–13).

- **Schema is reserved.** `project_profiles` stores the entity plus its
  generation provenance and `UNIQUE(project_id, version)`
  (migration 001 lines 313–343). It is covered by the migrations CI gate, which
  asserts all 8 tables — including `project_profiles` — exist after migrating
  (`docs/migrations-coverage.md` §2, success criterion 2).

- **Generation, persistence, and automation are NOT implemented in Phase 1.**
  No `profile_pg.go` adapter, no `projectdna` application service (the directory
  is an empty `.gitkeep`), and the use case is "Manual invocation only"
  (`docs/migrations-coverage.md` §1; design §4 line 352, §10 line 803).

  The command/query types `GenerateProfileCmd{ProjectID, Scope}` and
  `GetProfileQuery{ProjectID, Scope}` are defined on the inbound port
  (`internal/ports/inbound/profile_service.go` lines 10–25), but no type
  implements `ProjectProfileService`.

> TODO(verify): No HTTP/SDK route or inbound adapter handler for
> `GenerateProjectProfile` was traced. Confirm whether an inbound adapter route
> exists or is also Phase 2.

## Consequences

### Enables

- **A stable contract to build against.** The entity, ports, and `project_profiles`
  schema are fixed, so the Phase 2 service and adapter can be added without
  reshaping the domain or running a schema migration for the core columns
  (`profile.go`, the two port files, migration 001 lines 313–343).
- **CI already guards the table.** The migrations gate fails fast if
  `project_profiles` is missing, so the reserved schema cannot silently drift
  out of the migration set (`docs/migrations-coverage.md` §2).
- **Provenance-first design.** Because the schema stores `source_time_from/to`,
  `stale_after`, and `source_counts`, any future generation run is auditable and
  its staleness is computable from stored data (migration 001 lines 322–326).

### Defers / costs

- **Not usable end-to-end in Phase 1.** With no service and no adapter, a profile
  cannot actually be generated or persisted through the application — the feature
  is design + scaffold only (`docs/migrations-coverage.md` §1, "no adapter yet";
  empty `internal/application/projectdna/`).
- **`project_profiles` is dead schema until Phase 2.** The table and its indexes
  exist but are exercised by nothing except the table-presence sanity check; the
  coverage matrix marks its columns/indexes/triggers as `n/a` (no adapter)
  (`docs/migrations-coverage.md` §1).
- **Manual-only, no freshness automation.** Even once a `Generate` path exists,
  Phase 1 scope is manual invocation; worker-driven regeneration keyed on
  `StaleAfter` is explicitly Phase 2 (design §2.6 line 218, §4 line 352).
- **Risk of contract drift before first use.** Ports and entity were written
  ahead of a consumer, so the `Generate`/`Get` signatures are unproven against a
  real adapter and may need revision when Phase 2 implements them.

## References

- [Design §2.6 (ProjectProfile entity), §4 (P3 use case PP1), §5.1–§5.2 (ports), §6 (schema), §10 (Phase 1 scope: manual only)](../superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md)
- [`docs/migrations-coverage.md`](../migrations-coverage.md) — `project_profiles | (no adapter yet)` row and notes (§1)
- [`migrations/postgres/001_initial_schema.up.sql`](../../migrations/postgres/001_initial_schema.up.sql) lines 313–343 — `project_profiles` table
- [`internal/domain/projectprofile/profile.go`](../../internal/domain/projectprofile/profile.go) — implemented domain entity
- [`internal/ports/inbound/profile_service.go`](../../internal/ports/inbound/profile_service.go), [`internal/ports/outbound/profile_repository.go`](../../internal/ports/outbound/profile_repository.go) — declared ports
- Related: [ADR-0003 Storage choice](0003-storage-choice.md), [ADR-0004 Temporal/graph](0004-temporal-graph.md)
