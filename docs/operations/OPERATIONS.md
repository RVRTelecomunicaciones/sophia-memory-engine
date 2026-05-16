# Operations — sophia-memory-engine

Target audience: operator deploying in dev / staging / prod.

---

## Deploy

Install via the chart at `helm/sophia-memory-engine/`.

```bash
helm install sophia-memory-engine ./helm/sophia-memory-engine \
  --set secrets.dbHost=pg-memory-engine.svc.cluster.local \
  --set secrets.dbUser=memory_engine \
  --set secrets.dbPassword=<secret> \
  --set secrets.dbName=memory_engine \
  --set secrets.dbSslMode=require \
  --set apiKeys.bootstrapKeySha256=<sha256-of-dev-key>
```

Required values (chart will not function correctly without them):

| Value | Description | Example |
|---|---|---|
| `secrets.dbHost` | Postgres host | `pg-memory-engine.svc.cluster.local` |
| `secrets.dbUser` | Postgres user | `memory_engine` |
| `secrets.dbPassword` | Postgres password | — |
| `secrets.dbName` | Database name | `memory_engine` |
| `secrets.dbSslMode` | SSL mode | `require` (use `disable` only in dev) |
| `apiKeys.bootstrapKeySha256` | SHA-256 hash of the bootstrap API key seeded at startup | — |

Notes:
- `secrets.dbPort` defaults to `5432`. Override only if non-standard.
- `migrate.enabled` is `true` by default. The chart runs an init container using `ghcr.io/golang-migrate/migrate:v4.18.2`. In production environments where DDL changes must be reviewed before apply, set `migrate.enabled=false` and run migrations out-of-band (see the Migrations section below).
- `apiKeys.bootstrapKeyName` defaults to `orchestator`. Override if the consumer client has a different identity.
- In production, prefer External Secrets Operator pulling from Vault / AWS SM / GCP SM over committing values to a `values.yaml` file.

### Out-of-band migrations

```bash
# Using the migrate binary directly (matches cmd/migrate/main.go)
export DATABASE_URL="postgres://user:pass@host:5432/memory_engine?sslmode=require"
go run ./cmd/migrate -dsn "$DATABASE_URL" up
go run ./cmd/migrate -dsn "$DATABASE_URL" status
```

---

## Env vars

The service reads discrete `DB_*` vars — not a single DSN string. The `PORT` var controls the HTTP listener.

### Required

| Var | Controls | Example |
|---|---|---|
| `DB_HOST` | Postgres host | `pg-memory-engine.svc.cluster.local` |
| `DB_USER` | Postgres user | `memory_engine` |
| `DB_PASSWORD` | Postgres password | — |
| `DB_NAME` | Database name | `memory_engine` |
| `DB_SSLMODE` | TLS mode | `require` |

### Common tuning

| Var | Controls | Default |
|---|---|---|
| `PORT` | HTTP listener port | `8080` |
| `DB_PORT` | Postgres port | `5432` |

The retrieval weights, connection pool sizes (`MaxOpenConns=25`, `MaxIdleConns=5`), and context budget are code-defaults in `internal/infrastructure/config/config.go`. They are not currently overridable via env vars — changes require a recompile. See `DefaultConfig()` for all values.

---

## Healthchecks

Both probes are public (no `X-API-Key` required).

| Probe | Path | Method | Success | Failure |
|---|---|---|---|---|
| Liveness | `/health` | GET | 200 `{"status":"ok"}` | — (static, always 200) |
| Readiness | `/ready` | GET | 200 `{"status":"ready","checks":{"db":"ok"}}` | 503 `{"status":"degraded","checks":{"db":"<error>"}}` |

`/health` is a static response — it confirms the process is running and the HTTP listener is accepting connections.

`/ready` pings the Postgres pool with a 2-second timeout. It returns 503 if the ping fails or times out. The `checks.db` field in the response body contains the raw error string when degraded, which is useful for diagnosing connection refusals vs. auth failures.

Kubernetes probe configuration (matches `helm/sophia-memory-engine/values.yaml` defaults):

```yaml
livenessProbe:
  httpGet:
    path: /health
  initialDelaySeconds: 10
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

## Logs and metrics

### Log format

The service uses `log/slog` with a JSON handler writing to `stdout`. Every log line is a JSON object. Example:

```json
{"time":"2026-05-15T20:00:00Z","level":"INFO","msg":"request","method":"POST","path":"/api/v1/search","duration_ms":12,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7"}
```

Request logs include `trace_id` and `span_id` when the caller sends a W3C `traceparent` header (PR #10). Without that header, `trace_id` / `span_id` are absent from the log line.

Log level is set at startup and is not hot-reloadable. The chart default is `info`. To increase verbosity, set `config.logLevel: debug` in the chart values and redeploy.

### Metrics

No Prometheus `/metrics` endpoint is exposed. The service does not import a Prometheus client. Metrics are derived from logs (duration_ms, status codes) via the log pipeline. If you need structured metrics, scrape the structured JSON logs with a log-to-metrics agent (e.g. Promtail + Loki, Vector).

### Slow queries

The Postgres FTS queries (`plainto_tsquery('spanish', ...)` + trigram `similarity()`) are the hot path. Enable `pg_stat_statements` on the database instance and query it for slow statements against the `memories` table. The `log_min_duration_statement` Postgres setting is the standard mechanism — the service itself does not add application-level query timing logs.

---

## Troubleshooting

### 1. Migration 004 unique-index violation on ingest

**Symptom:** `POST /api/v1/memories` returns 500. Postgres logs show:
```
ERROR: duplicate key value violates unique constraint "idx_memories_topic_key_active_unique"
```

**Cause:** Two ingest calls for the same `(project_id, topic_key)` arrived before the first was committed, or pre-004 data has duplicate active rows. Migration 004 adds a partial unique index on `(project_id, COALESCE(tenant_id,''), topic_key) WHERE topic_key IS NOT NULL AND status='active'`. The `COALESCE` is intentional — without it, two NULL-tenant rows with the same `topic_key` would bypass the constraint.

**Fix:**
- For pre-existing duplicates: migration 004 Step 1 archives older duplicates automatically. If you hit this on a DB that skipped the backfill (e.g. migration was applied while the backfill UPDATE was missed), run the CTE from `migrations/postgres/004_memories_topic_key_unique.up.sql` manually, then recreate the index.
- For concurrent ingests: use the `/memories/by-topic-key` endpoint to check for an active record first, or rely on the upsert behavior introduced in PR #7.

---

### 2. /ready returns 503 immediately after deploy

**Symptom:** Readiness probe fails, pod stays in `NotReady`, traffic withheld.

**Cause (most common):** Migrations init container failed or is still running. The HTTP server starts before the pool is healthy if the DB is unreachable.

**Fix:** Check init container logs:
```bash
kubectl logs <pod> -c migrate
```
Confirm `DATABASE_URL` (used by the init container) matches `DB_HOST` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` (used by the app). The init container takes a DSN string; the app takes discrete vars — they must agree on the same database.

---

### 3. Scope filter mismatch — cross-project memory leakage

**Symptom:** Search or context-build results include records from a different project. No error is returned.

**Cause:** The `project_id` field in the request body was empty or incorrect. All repository queries at the persistence layer enforce `AND project_id = $N` — but this guard only fires if the caller sends a non-empty `project_id`. An empty `project_id` will pass the `shared.NewScope` validation check, match all rows in the column, and return cross-project data.

**Fix:** Always pass a non-empty `project_id` (and `tenant_id` if multi-tenant) in every request. Validate this at the client before calling the API. The `BuildContext` endpoint validates scope explicitly and returns 400 on empty `project_id`; `Search` does not currently reject it — treat this as a client responsibility.

---

### 4. Spanish FTS returns zero results for queries that should match

**Symptom:** `POST /api/v1/search` with a Spanish query returns `[]` even though the content clearly exists.

**Cause:** The FTS index uses `plainto_tsquery('spanish', ...)` which applies the PostgreSQL Spanish stemmer (`pg_catalog.spanish`). Words not in the stemmer's dictionary (proper nouns, acronyms, mixed-language technical terms) are dropped entirely from the query vector. A query like `"SDD_EXPLORE"` becomes an empty tsquery and matches nothing.

**Fix options:**
- Use the `topic_key` lookup (`GET /memories/by-topic-key?topic_key=sdd/my-change/explore`) for exact lookups instead of FTS.
- Ensure the `pg_trgm` extension is enabled (`CREATE EXTENSION IF NOT EXISTS pg_trgm`) — trigram similarity runs in parallel and will catch partial matches even when FTS fails.
- For mixed-language content, write memories with a `type` that supports a type boost (`sdd_*` types get a 0.10 additive increment on the final score via `SDDTypeIncrement`).

---

### 5. trgm_score is 0.0 in search results (pre-fix symptom)

**Symptom:** Search response shows `trigram_score: 0` for all results even when content is similar to the query.

**Cause:** This was a bug in the FTS adapter — `similarity(content, $1)` was missing from the SQL SELECT, so trigram scores were hardcoded to 0.0. Fixed in commit `953ea50`.

**Fix:** Ensure you are on a commit at or after `953ea50`. If still on an older build, the trigram dimension of the ranking is effectively dead and the hybrid retrieval score degrades to FTS + recency + importance only.

---

<!-- Prometheus /metrics endpoint: not implemented as of May 2026. No content to document. -->
<!-- Log-level hot reload: not implemented; env var tuning for retrieval weights: not implemented. -->
