//go:build integration

// Package persistence — ADR-0005 P2.3 hot-path benchmark & EXPLAIN regression.
//
// The hot path is sophia-orchestator's apply-phase loadTasksList, which calls
// FindActiveByTopicKey(scope, topic_key) once per phase load. The benchmark
// here verifies:
//   1. The Postgres planner picks an index (Index Scan / Bitmap Heap Scan),
//      not a Seq Scan, for that query on a 10k-row table.
//   2. The indexed plan is ≥ 3× faster than a synthetic no-index baseline
//      (achieved by forcing the planner to a sequential scan via
//      enable_indexscan = off / enable_bitmapscan = off).
//
// These tests require Docker (testcontainers).
package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
)

// hotPathQuery is the SQL used by FindActiveByTopicKey, restated here so the
// EXPLAIN regression doesn't have to introspect the repository internals.
// Keep this in lockstep with memory_pg.go.
const hotPathQuery = `
	SELECT id FROM memories
	WHERE topic_key = $1
	  AND status = 'active'
	  AND project_id = $2
	  AND (tenant_id IS NULL OR tenant_id = $3)
	LIMIT 1`

// seedSize is the number of active memories seeded for the benchmark. 10k is
// large enough to give the planner a real choice between Seq Scan and Index
// Scan, and small enough to seed in < 30s on a laptop.
const seedSize = 10_000

// archivedSize is the number of archived rows added on top of seedSize, to
// pressure-test the partial index predicate (status = 'active').
const archivedSize = 2_000

// seedTimestamp anchors the ULID timestamps so reproducible IDs are produced.
// We don't use time.Now() (R12) — the seed is deterministic w.r.t. test order.
var seedTimestamp = time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

// seedMemories bulk-inserts seedSize active rows + archivedSize archived rows.
// Topic keys are spread across multiple projects so the planner sees a varied
// distribution. The benchmark target project (project-bench-042) holds ~100
// active rows with predictable topic_keys (those where i % 100 == 42).
//
// We pre-generate real ULID strings client-side (so the repo's
// RecordIDFromString validation passes) and use unnest-on-arrays for a single
// round-trip bulk INSERT. The FTS trigger is disabled for the duration of the
// seed to keep wall-clock under a few seconds.
func seedMemories(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Disable the FTS trigger during bulk load — the benchmarked query
	// doesn't use search_vector, and keeping the trigger on roughly triples
	// seed time.
	_, err := pool.Exec(ctx, `ALTER TABLE memories DISABLE TRIGGER trg_memories_fts`)
	if err != nil {
		t.Fatalf("disable fts trigger: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `ALTER TABLE memories ENABLE TRIGGER trg_memories_fts`)
	}()

	entropy := ulid.Monotonic(deterministicRand(), 0)
	tsMillis := uint64(seedTimestamp.UnixMilli())

	// Active rows.
	ids := make([]string, seedSize)
	topicKeys := make([]string, seedSize)
	projectIDs := make([]string, seedSize)
	contents := make([]string, seedSize)
	for i := 0; i < seedSize; i++ {
		ids[i] = ulid.MustNew(tsMillis, entropy).String()
		topicKeys[i] = fmt.Sprintf("sdd/p2.3/topic-%05d", i)
		projectIDs[i] = fmt.Sprintf("project-bench-%03d", i%100)
		contents[i] = fmt.Sprintf("content for memory %d — sdd quick brown fox lazy dog", i)
	}

	const activeSQL = `
		INSERT INTO memories (
			id, type, content, tags, topic_key, fts_language,
			project_id, source, ingest_method,
			freshness, importance_score, importance_computed_at, importance_factors,
			status, created_at, updated_at
		)
		SELECT
			ids.id, 'semantic', ids.content, ARRAY['bench','sdd']::text[],
			ids.topic_key, 'english'::regconfig,
			ids.project_id, 'bench-seed', 'direct',
			'fresh', 0.5, $1, '[]'::jsonb,
			'active', $1, $1
		FROM UNNEST($2::text[], $3::text[], $4::text[], $5::text[])
			AS ids(id, topic_key, project_id, content)`
	if _, err := pool.Exec(ctx, activeSQL, seedTimestamp, ids, topicKeys, projectIDs, contents); err != nil {
		t.Fatalf("seed active rows: %v", err)
	}

	// Archived rows.
	aIDs := make([]string, archivedSize)
	aTopicKeys := make([]string, archivedSize)
	aProjectIDs := make([]string, archivedSize)
	aContents := make([]string, archivedSize)
	for i := 0; i < archivedSize; i++ {
		aIDs[i] = ulid.MustNew(tsMillis, entropy).String()
		aTopicKeys[i] = fmt.Sprintf("sdd/p2.3/archived-%05d", i)
		aProjectIDs[i] = fmt.Sprintf("project-bench-%03d", i%100)
		aContents[i] = fmt.Sprintf("archived content %d", i)
	}

	const archivedSQL = `
		INSERT INTO memories (
			id, type, content, tags, topic_key, fts_language,
			project_id, source, ingest_method,
			freshness, importance_score, importance_computed_at, importance_factors,
			status, archived_by, archive_reason, created_at, updated_at
		)
		SELECT
			ids.id, 'semantic', ids.content, ARRAY['bench']::text[],
			ids.topic_key, 'english'::regconfig,
			ids.project_id, 'bench-seed', 'direct',
			'fresh', 0.3, $1, '[]'::jsonb,
			'archived', 'bench', 'seed-archive', $1, $1
		FROM UNNEST($2::text[], $3::text[], $4::text[], $5::text[])
			AS ids(id, topic_key, project_id, content)`
	if _, err := pool.Exec(ctx, archivedSQL, seedTimestamp, aIDs, aTopicKeys, aProjectIDs, aContents); err != nil {
		t.Fatalf("seed archived rows: %v", err)
	}

	// ANALYZE so the planner has fresh stats — without this on a freshly
	// loaded table the planner may pick a Seq Scan purely on default
	// estimates.
	if _, err := pool.Exec(ctx, `ANALYZE memories`); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}
}

// deterministicRand returns a reader that emits a fixed bit pattern. ULID
// monotonic mode only uses entropy on the first ID per timestamp tick, so a
// stable reader is fine.
func deterministicRand() *staticReader { return &staticReader{} }

type staticReader struct{ n int }

func (r *staticReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r.n)
		r.n++
	}
	return len(p), nil
}

// -----------------------------------------------------------------------------
// EXPLAIN regression test
// -----------------------------------------------------------------------------

// TestExplain_FindActiveByTopicKey_UsesIndex asserts that the production
// hot-path query never falls back to a Seq Scan after migrations are applied.
//
// This test is the architectural guard: if a future migration drops or alters
// the index that the planner currently uses, this test must fail loudly.
func TestExplain_FindActiveByTopicKey_UsesIndex(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	seedMemories(t, pool)

	plan := getExplainPlan(t, pool, hotPathQuery,
		"sdd/p2.3/topic-00042", "project-bench-042", nil)
	t.Logf("EXPLAIN plan:\n%s", plan)

	if strings.Contains(plan, "Seq Scan on memories") {
		t.Fatalf("hot-path query fell back to Seq Scan — index missing or planner mis-estimating. Plan:\n%s", plan)
	}

	indexed := strings.Contains(plan, "Index Scan") ||
		strings.Contains(plan, "Bitmap Index Scan") ||
		strings.Contains(plan, "Bitmap Heap Scan") ||
		strings.Contains(plan, "Index Only Scan")
	if !indexed {
		t.Fatalf("hot-path query plan does not contain Index/Bitmap Scan node. Plan:\n%s", plan)
	}
}

// getExplainPlan runs EXPLAIN (ANALYZE, BUFFERS) against the supplied query and
// returns the plan as one concatenated string. We use EXPLAIN ANALYZE (not
// plain EXPLAIN) so the test surfaces real timing if the plan is somehow
// suboptimal — the assertion is on node types, not timings.
func getExplainPlan(t testing.TB, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"EXPLAIN (ANALYZE, BUFFERS) "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN rows: %v", err)
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Benchmark
// -----------------------------------------------------------------------------

// Benchmark_FindActiveByTopicKey measures ns/op for the hot path on a 10k-row
// table. The benchmark is parameterised by index-availability mode:
//
//   - "indexed" — let the planner choose; expected Index/Bitmap Scan
//   - "no_index" — force Seq Scan via SET enable_indexscan = off /
//     enable_bitmapscan = off (synthetic baseline)
//
// Each iteration runs the FULL Go → pgx → Postgres → pgx → Go round-trip via
// MemoryPgRepository.FindActiveByTopicKey. The Go-side latency (~200µs on
// localhost) dominates the report, so we ALSO emit `pure_sql_ns` per-iter
// via EXPLAIN ANALYZE side-channels (see TestSQL_HotPathSpeedup below) for a
// network-noise-free speedup ratio.
//
// Run via:
//
//   go test -bench=Benchmark_FindActiveByTopicKey -tags=integration \
//     ./internal/adapters/outbound/persistence/...
func Benchmark_FindActiveByTopicKey(b *testing.B) {
	pool := testhelper.SetupTestDB(b)
	seedMemories(b, pool)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	scope, err := shared.NewScope("project-bench-042")
	if err != nil {
		b.Fatalf("scope: %v", err)
	}

	// "indexed" exercises the REAL production hot path:
	// MemoryPgRepository.FindActiveByTopicKey(ctx, scope, topic_key).
	// This is the Go-side end-to-end cost the orchestator pays per apply-phase
	// load.
	b.Run("indexed", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tk := fmt.Sprintf("sdd/p2.3/topic-%05d", (i*97)%seedSize)
			_, err := repo.FindLatestActiveByTopicKey(ctx, scope, tk)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				b.Fatalf("find: %v", err)
			}
		}
	})

	// "no_index" pins a single connection, forces Seq Scan via SET, and runs
	// the SAME hot query directly on that connection. The repo path can't be
	// used here because pool.Acquire returns a different conn each call —
	// the SET would not apply to the repo's chosen conn. The raw-query path
	// is the closest faithful baseline.
	b.Run("no_index", func(b *testing.B) {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			b.Fatalf("acquire: %v", err)
		}
		defer conn.Release()

		for _, stmt := range []string{
			"SET enable_indexscan = off",
			"SET enable_bitmapscan = off",
			"SET enable_indexonlyscan = off",
		} {
			if _, err := conn.Exec(ctx, stmt); err != nil {
				b.Fatalf("set planner flag: %v", err)
			}
		}
		defer func() {
			for _, stmt := range []string{
				"RESET enable_indexscan",
				"RESET enable_bitmapscan",
				"RESET enable_indexonlyscan",
			} {
				_, _ = conn.Exec(ctx, stmt)
			}
		}()

		// Capture and log the no_index plan so a future regression (e.g.,
		// PG version that ignores the flag) fails the assertion below.
		noIdxPlan := getExplainPlanOnConn(b, conn, ctx, hotPathQuery,
			"sdd/p2.3/topic-00042", scope.ProjectID, nil)
		b.Logf("no_index plan:\n%s", noIdxPlan)
		if !strings.Contains(noIdxPlan, "Seq Scan") {
			b.Fatalf("no_index baseline failed to force Seq Scan — planner flags ineffective. Plan:\n%s", noIdxPlan)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tk := fmt.Sprintf("sdd/p2.3/topic-%05d", (i*97)%seedSize)
			var id string
			err := conn.QueryRow(ctx, hotPathQuery, tk, scope.ProjectID, nil).Scan(&id)
			if err != nil && !strings.Contains(err.Error(), "no rows") {
				b.Fatalf("query: %v", err)
			}
		}
	})
}

// getExplainPlanOnConn runs EXPLAIN on an already-acquired pooled connection
// so per-connection planner overrides apply. Returns the plan as text.
func getExplainPlanOnConn(t testing.TB, conn *pgxpool.Conn, ctx context.Context, query string, args ...any) string {
	t.Helper()
	rows, err := conn.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN rows: %v", err)
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Pure-SQL speedup test (server-side timing, network noise removed)
// -----------------------------------------------------------------------------

// TestSQL_HotPathSpeedup_IndexVsSeqScan asserts the indexed plan is ≥ 3× faster
// than the forced-Seq-Scan baseline, measured ENTIRELY on the Postgres side
// via a PL/pgSQL loop. Because both halves run inside the same server-side
// loop, network and pgx-framing overhead cancel out and only the planner's
// choice differs.
//
// This is the rigorous "≥ 3× speedup" claim for ADR-0005 P2.3. The Go-side
// Benchmark_FindActiveByTopicKey is informational; this test is authoritative.
func TestSQL_HotPathSpeedup_IndexVsSeqScan(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	seedMemories(t, pool)
	ctx := context.Background()

	// Use a plain SQL function (NOT plpgsql) so the planner re-plans for
	// each call honoring the current session's enable_indexscan setting.
	// PL/pgSQL would cache the first plan and re-use it for subsequent
	// calls, defeating the no_index baseline.
	//
	// The function takes an array of topic_keys and runs the hot query
	// once per element via a JOIN. Single-statement = single-plan, but a
	// fresh plan every call. The total time scales linearly with the
	// number of topic_keys, so iterations are effectively array length.
	createFn := `
		CREATE OR REPLACE FUNCTION bench_hotpath(tks TEXT[]) RETURNS DOUBLE PRECISION AS $$
			SELECT extract(epoch FROM (clock_timestamp() - t0)) * 1000.0
			FROM (
				SELECT clock_timestamp() AS t0
			) AS s
			CROSS JOIN LATERAL (
				SELECT COUNT(*) FROM unnest(tks) AS u(tk)
				LEFT JOIN LATERAL (
					SELECT m.id FROM memories m
					WHERE m.topic_key = u.tk
					  AND m.status = 'active'
					  AND m.project_id = 'project-bench-042'
					  AND (m.tenant_id IS NULL OR m.tenant_id = NULL)
					LIMIT 1
				) hit ON TRUE
			) AS r;
		$$ LANGUAGE sql;`
	if _, err := pool.Exec(ctx, createFn); err != nil {
		t.Fatalf("create bench fn: %v", err)
	}

	const iterations = 5000

	// Build the topic_keys array once: deterministic pseudo-random walk
	// through the seedSize active keys, modulo collisions allowed.
	tks := make([]string, iterations)
	for i := 0; i < iterations; i++ {
		tks[i] = fmt.Sprintf("sdd/p2.3/topic-%05d", (i*97)%seedSize)
	}

	// Acquire a single connection so SET overrides stay confined.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	// Warm cache before each measurement (one untimed pass) to remove
	// first-call cold-cache noise.
	if _, err := conn.Exec(ctx, "SELECT bench_hotpath($1)", tks[:200]); err != nil {
		t.Fatalf("warm: %v", err)
	}

	measure := func(label string) float64 {
		var ms float64
		if err := conn.QueryRow(ctx,
			"SELECT bench_hotpath($1)", tks,
		).Scan(&ms); err != nil {
			t.Fatalf("%s measure: %v", label, err)
		}
		return ms
	}

	// 1. Indexed run (planner free to choose).
	for _, stmt := range []string{
		"RESET enable_indexscan",
		"RESET enable_bitmapscan",
		"RESET enable_indexonlyscan",
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("reset planner flag: %v", err)
		}
	}
	indexedMs := measure("indexed")
	indexedPerOpNs := indexedMs * 1_000_000.0 / float64(iterations)

	// 2. No-index run (forced Seq Scan).
	for _, stmt := range []string{
		"SET enable_indexscan = off",
		"SET enable_bitmapscan = off",
		"SET enable_indexonlyscan = off",
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("set planner flag: %v", err)
		}
	}
	noIndexMs := measure("no_index")
	noIndexPerOpNs := noIndexMs * 1_000_000.0 / float64(iterations)

	speedup := noIndexMs / indexedMs
	t.Logf("ADR-0005 P2.3 pure-SQL speedup (iterations=%d):", iterations)
	t.Logf("  indexed:  %.3f ms total, %.0f ns/op", indexedMs, indexedPerOpNs)
	t.Logf("  no_index: %.3f ms total, %.0f ns/op", noIndexMs, noIndexPerOpNs)
	t.Logf("  speedup:  %.2f×", speedup)

	if speedup < 3.0 {
		t.Fatalf("ADR-0005 P2.3 acceptance failed: indexed speedup is %.2f× — must be ≥ 3.0×. "+
			"Index missing or planner mis-estimating the hot query.", speedup)
	}
}

