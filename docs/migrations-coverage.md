# Migrations Coverage Audit (P0.3 — ADR-0005)

This document is the source of truth for "do the committed migrations under
`migrations/postgres/` produce **every** column / index / trigger that the
persistence adapters under `internal/adapters/outbound/persistence/` reference?"

It is updated whenever a new aggregate, adapter, or migration is introduced.
The CI workflow at `.github/workflows/migrations.yml` enforces the contract by
applying the migrations against a fresh Postgres and running the integration
test suite against the externally-migrated database (no in-test schema
creation).

## 1. Coverage matrix

Source files audited:

- `internal/adapters/outbound/persistence/memory_pg.go`
- `internal/adapters/outbound/persistence/decision_pg.go`
- `internal/adapters/outbound/persistence/heuristic_pg.go`
- `internal/adapters/outbound/persistence/relation_pg.go`
- `internal/adapters/outbound/persistence/purge_pg.go`
- `internal/adapters/outbound/persistence/feedback_pg.go`
- `internal/adapters/outbound/persistence/tx_manager_pg.go`
- `internal/adapters/outbound/persistence/helpers.go`

| Aggregate          | Adapter file        | Migration | Columns | Indexes | Triggers | Drift |
|--------------------|---------------------|-----------|---------|---------|----------|-------|
| memories           | `memory_pg.go`      | 001       | OK      | OK      | OK       | none  |
| decisions          | `decision_pg.go`    | 001       | OK      | OK      | OK       | none  |
| heuristics         | `heuristic_pg.go`   | 001       | OK      | OK      | OK       | none  |
| relations          | `relation_pg.go`    | 001       | OK      | OK      | n/a      | none  |
| purge_records      | `purge_pg.go`       | 001       | OK      | OK      | OK       | none  |
| project_profiles   | (no adapter yet)    | 001       | n/a     | n/a     | n/a      | none  |
| domain_events      | (no adapter yet)    | 001       | n/a     | n/a     | n/a      | none  |
| retrieval_feedback | `feedback_pg.go`    | 002       | OK      | OK      | n/a      | none  |

Notes:

- `project_profiles` and `domain_events` tables are present in `001_initial_schema.up.sql`
  but no Go adapter references them yet (Phase 1 scope). They will be exercised
  in a later sprint when the corresponding ports are implemented. The CI gate
  will catch any future adapter that adds columns/indexes not covered by a
  migration.
- "Triggers" column refers to `set_updated_at` and the per-aggregate FTS
  trigger (`*_fts_update`). `relations`, `domain_events`, and `retrieval_feedback`
  do not use FTS triggers; `relations` does have an `updated_at` trigger but no
  FTS, hence n/a for FTS-trigger purposes. `domain_events` is append-only so it
  has no `updated_at` trigger.
- Adapter INSERT column lists were cross-checked against migration column
  definitions on 2026-05-10. Every column appearing in an `INSERT INTO …`
  statement maps to a column declared in `001_*.up.sql` or `002_*.up.sql`.
- All indexes referenced via `WHERE` clauses or implicit lookup paths in the
  adapter SQL (e.g. `idx_memories_topic_key`, `idx_decisions_decision_key`,
  `idx_heuristics_active`) are declared in the migrations.

**Result: no drift.** The committed migrations alone produce the full schema
required by the integration test suite.

## 2. Goose-equivalent verification

Repository policy is to use the in-tree `cmd/migrate` binary rather than
`golang-migrate/migrate` or `pressly/goose`. The binary is dependency-free
(only `pgx` + stdlib), wraps each migration in a transaction, and tracks
applied versions in a `schema_migrations(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ)`
ledger. CLI:

```text
migrate -dir migrations/postgres -dsn <DSN> up      # apply pending migrations
migrate -dir migrations/postgres -dsn <DSN> status  # list versions on disk + ledger state
```

### Local verification

Spin up Postgres, build the binary, apply migrations, then run the integration
suite against the externally-migrated database (the same path CI uses):

```bash
docker run --rm -d --name memory-pg \
    -p 5432:5432 -e POSTGRES_PASSWORD=dev -e POSTGRES_USER=dev \
    -e POSTGRES_DB=memory_engine postgres:16-alpine
sleep 3

go build -o migrate ./cmd/migrate
./migrate -dir migrations/postgres \
    -dsn "postgres://dev:dev@localhost:5432/memory_engine?sslmode=disable" up

MEMORY_ENGINE_TEST_DSN="postgres://dev:dev@localhost:5432/memory_engine?sslmode=disable" \
    go test -tags=integration ./test/integration/... -count=1 -timeout 120s

docker rm -f memory-pg
```

### Success criteria

1. `migrate up` exits 0 and logs `applied` for each pending file (or `skip`
   if already applied).
2. The integration test suite passes against the externally-migrated DB. The
   test helper at `test/integration/testhelper/pg.go` performs a sanity check
   that the 8 expected tables are present (`memories`, `decisions`,
   `heuristics`, `relations`, `purge_records`, `project_profiles`,
   `domain_events`, `retrieval_feedback`) and fails fast if the gate did not
   migrate before tests ran.
3. `./migrate -dsn ... status` lists every `*.up.sql` file with `yes (...)`.

If any step fails, a migration is missing or out of sync with an adapter — see
the drift policy below.

## 3. Drift policy

Persistence adapters under `internal/adapters/outbound/persistence/` MUST NEVER
reference a column, index, or trigger that is not present in a committed
migration under `migrations/postgres/`. Concretely:

- A new column on an existing aggregate requires a new numbered migration
  (`NNN_add_<column>_to_<table>.up.sql` + `.down.sql`) — never edit a
  previously-committed migration in place.
- A new aggregate requires both a new migration pair AND an entry in the
  coverage matrix above.
- A new index referenced by an adapter (even implicitly via query shape)
  requires a corresponding `CREATE INDEX` in the same migration that introduces
  the query path.
- The CI gate at `.github/workflows/migrations.yml` is the enforcement
  mechanism: it boots a fresh Postgres, applies the committed migrations via
  `cmd/migrate up`, then runs the full integration suite. If any adapter
  references a missing schema element the suite fails and the PR is blocked.

The drift column in section 1 must read `none` for every row before merging.
If drift is found, the PR that introduced the adapter change must also
include the corresponding migration.

## 4. How to add a new migration

1. Create `migrations/postgres/NNN_<descriptive>.up.sql` with the schema
   changes. Pick `NNN` as the next available zero-padded integer.
2. Create the matching `migrations/postgres/NNN_<descriptive>.down.sql`
   with the inverse operations (drop indexes/triggers/tables, reverse-order).
3. Apply against a fresh DB locally:

   ```bash
   go build -o migrate ./cmd/migrate
   ./migrate -dir migrations/postgres -dsn "$DATABASE_URL" up
   ```

4. Update this document if a new aggregate or non-trivial index/trigger was
   introduced (add a row to the coverage matrix and confirm `Drift = none`).
5. Push the branch. The CI workflow `.github/workflows/migrations.yml` will
   gate the PR by re-running the full migrate-and-test loop in a clean
   Postgres 16 service container.
