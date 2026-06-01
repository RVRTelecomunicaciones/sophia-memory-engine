# sophia-memory-engine

The knowledge layer of the Sophia agent ecosystem. It is a Go service that stores,
versions, and retrieves the durable memory of agents — episodic and semantic memories,
decisions, heuristics, and the relations between them — and serves them back to
`sophia-orchestator` over an HTTP REST API. Retrieval is powered by PostgreSQL
full-text search and trigram matching, scoped per project/tenant and aware of temporal
validity.

> **Status:** Phase 1 (synchronous HTTP + Postgres path) is **built and contract-verified**
> against the orchestrator. The asynchronous "intelligence" half (background workers,
> graph/vector adapters, MCP/SDK transports) is **designed but not yet implemented** —
> it exists as scaffolding. See [Phase 1 vs Phase 2](#phase-1-vs-phase-2) before relying
> on any feature.

## Quick path

```bash
# From INSIDE the repo (go test fails at the workspace root — there is a go.work above).
cd sophia-memory-engine

# 1. Apply migrations to a Postgres 16 instance.
export DATABASE_URL="postgres://user:pass@localhost:5432/memory_engine?sslmode=disable"
go run ./cmd/migrate -dsn "$DATABASE_URL" up

# 2. Run the service (reads discrete DB_* vars, NOT a DSN).
DB_HOST=localhost DB_USER=user DB_PASSWORD=pass DB_NAME=memory_engine \
DB_SSLMODE=disable PORT=8080 go run ./cmd/memory-engine

# 3. Verify it is alive.
curl localhost:8080/health   # → {"status":"ok"}
curl localhost:8080/ready    # → {"status":"ready","checks":{"db":"ok"}}
```

`/health` and `/ready` are public. Every `/api/v1/*` route requires an `X-API-Key`
header. A local dev key is seeded by migration `003` (see
[`migrations/postgres/003_create_api_keys.up.sql`](migrations/postgres/003_create_api_keys.up.sql) —
**rotate it before any non-local deployment**).

## Architecture at a glance

Hexagonal / clean architecture. Dependencies point inward; the domain knows nothing
about HTTP or Postgres.

```
sophia-orchestator
  → inbound adapter (HTTP REST)        internal/adapters/inbound/http
  → application use cases              internal/application/*
  → domain validation + invariants     internal/domain/*
  → outbound ports                      internal/ports/outbound
  → outbound adapters (Postgres, FTS)  internal/adapters/outbound/*
```

| Layer | Responsibility | Allowed to depend on |
|-------|----------------|----------------------|
| Domain | Entities, value objects, invariants | nothing (no HTTP, DB, frameworks) |
| Application | Use cases, transaction boundaries, context building | domain, ports |
| Ports | Repository / search / event interfaces | domain |
| Adapters | inbound (HTTP) + outbound (Postgres, FTS) | ports, domain |
| Infrastructure | config, logging, DB pool, migrations | — |

Full detail: [`docs/architecture.md`](docs/architecture.md) and the domain rules in
[`docs/domain-invariants.md`](docs/domain-invariants.md).

## Phase 1 vs Phase 2

The repository contains the **complete designed shape** of the system. Only Phase 1 is
implemented. The rest is intentional, tracked scaffolding (`.gitkeep` placeholders) so
the package layout matches the design.

### Built (Phase 1)

- **Inbound:** HTTP REST adapter (`internal/adapters/inbound/http`).
- **Outbound:** Postgres persistence for memories, decisions, heuristics, relations,
  purge records, API keys; Postgres FTS search index (`internal/adapters/outbound/persistence`,
  `internal/adapters/outbound/search`).
- **Storage:** PostgreSQL 16 with FTS (`tsvector`/`tsquery`, `spanish` default) and
  `pg_trgm` trigram fallback. Migrations under `migrations/postgres/`.
- **Retrieval:** FTS + trigram + scope + temporal + graph expansion (recursive CTE) +
  explainable ranking. **No vector similarity.** See [`docs/retrieval-tuning.md`](docs/retrieval-tuning.md).
- **Auth:** API-key authentication on all `/api/v1` routes.
- **Events:** in-process publisher writing to the `domain_events` table.
- **Embeddings:** a no-op stub implementing the port — no use case depends on it.

Per the design's own scope table, 13 of the 18 use cases (P0 + P1) are fully
implemented; 4 (purge request/execute, toggle heuristic, contradict decision) are
late-Phase-1/early-Phase-2, and project-profile generation is manual only.
(See [design §10](docs/superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md).)

### Scaffolded / planned (Phase 2+)

These directories are `.gitkeep` placeholders — **designed, not built**:

| Area | Path | Phase |
|------|------|-------|
| Background workers (freshness, importance, consolidation, contradiction, projectdna, purge cleanup) | `internal/jobs/*`, `internal/application/{freshness,consolidation,projectdna}` | 2 |
| Graph adapter (dedicated graph DB) | `internal/adapters/outbound/graph` | 2 (if volume demands) |
| Vector adapter (embeddings + similarity) | `internal/adapters/outbound/vector` | 2 |
| MCP read transport | `internal/adapters/inbound/mcp` | 2 |
| Go SDK transport | `internal/adapters/inbound/sdk` | 2 |
| Scheduler / queue / observability infra | `internal/infrastructure/{scheduler,queue,observability}` | 2 |

> **Note:** the design document lists the Go SDK adapter as a Phase 1 deliverable, but in
> the current tree `internal/adapters/inbound/sdk` is `.gitkeep` only. The README reflects
> the **code**, not the plan: SDK transport is scaffolding.

## HTTP API surface

Base path `/api/v1`, all routes behind `X-API-Key`. Registered in
[`internal/adapters/inbound/http/server.go`](internal/adapters/inbound/http/server.go)
and specified in [`api/openapi/memory-engine.yaml`](api/openapi/memory-engine.yaml).

The **orchestrator-contracted core (8 endpoints)** — the production sync path:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/memories` | Ingest a memory |
| `GET` | `/api/v1/memories/{id}` | Fetch one memory by ID |
| `GET` | `/api/v1/memories/by-topic-key` | Fetch the active memory for a topic key |
| `POST` | `/api/v1/memories/{id}/archive` | Archive a memory |
| `POST` | `/api/v1/search` | Hybrid (FTS + trigram) ranked search |
| `POST` | `/api/v1/search/context` | Build a token-budgeted context window |
| `POST` | `/api/v1/decisions` | Record a decision |
| `POST` | `/api/v1/relations` | Create a relation between records |

Additional registered routes (also live in Phase 1):

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/decisions/{id}` | Fetch a decision |
| `GET` | `/api/v1/decisions/history/{key}` | Decision version history |
| `POST` | `/api/v1/decisions/{id}/contradict` | Mark a decision contradicted |
| `POST` | `/api/v1/heuristics` | Create a heuristic |
| `GET` | `/api/v1/heuristics` | List heuristics by scope |
| `GET` | `/api/v1/heuristics/active/{key}` | Fetch the active heuristic for a key |
| `POST` | `/api/v1/heuristics/{id}/toggle` | Enable/disable a heuristic |
| `GET` | `/api/v1/relations/from/{id}` | Relations originating from a record |
| `GET` | `/api/v1/relations/to/{id}` | Relations pointing to a record |
| `POST` | `/api/v1/purge/request` | Request a purge (secret leak / compliance) |
| `POST` | `/api/v1/purge/{id}/execute` | Execute a pending purge |
| `POST` | `/api/v1/feedback` | Submit retrieval feedback |
| `GET` | `/health` | Liveness (public, static `200`) |
| `GET` | `/ready` | Readiness — pings Postgres (public) |

## Build, run, test

Commands are driven by the [`Makefile`](Makefile). **Run them from inside the repo** —
`go test ./...` fails at the workspace root because of the `go.work` one level up.

```bash
cd sophia-memory-engine

make test              # go test ./...
make test-unit         # domain + application only (no DB)
make test-integration  # testcontainers-backed Postgres; needs Docker
make openapi-validate  # OpenAPI spec drift test
make lint              # golangci-lint run ./...
```

Service entrypoints:

| Binary | Path | Role |
|--------|------|------|
| `memory-engine` | `cmd/memory-engine` | HTTP service |
| `migrate` | `cmd/migrate` | Apply/inspect migrations (`up`, `status`) |
| `workers` | `cmd/workers` | Background workers entrypoint (Phase 2 scaffolding) |

Required env vars (`DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`) and
deployment via the Helm chart are documented in
[`docs/operations/OPERATIONS.md`](docs/operations/OPERATIONS.md).

## Deeper docs

| Topic | Doc |
|-------|-----|
| Architecture & layer boundaries | [`docs/architecture.md`](docs/architecture.md) |
| Domain invariants | [`docs/domain-invariants.md`](docs/domain-invariants.md) |
| Retrieval ranking & tuning | [`docs/retrieval-tuning.md`](docs/retrieval-tuning.md) |
| Migrations coverage | [`docs/migrations-coverage.md`](docs/migrations-coverage.md) |
| Security & purge | [`docs/security-purge.md`](docs/security-purge.md) |
| Operations & deploy | [`docs/operations/OPERATIONS.md`](docs/operations/OPERATIONS.md) |
| Architecture decisions (ADRs) | [`docs/decisions/`](docs/decisions/) |
| Phase 1 master plan & design | [`docs/superpowers/`](docs/superpowers/) |
