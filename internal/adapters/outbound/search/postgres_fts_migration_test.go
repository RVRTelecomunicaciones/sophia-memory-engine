//go:build integration

package search_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestFTSMigration005_SwitchesToSimple verifies that migration 005 correctly
// updates fts_language from 'spanish' to 'simple' across all three FTS tables
// and alters the column default. It also verifies idempotency and round-trip.
func TestFTSMigration005_SwitchesToSimple(t *testing.T) {
	pool := setupFTSTestDB(t)
	ctx := context.Background()

	// --- Pre-seed: insert one row in each table with fts_language='spanish' ---
	// These rows simulate existing data before migration 005.
	memID := "01HWABCDEF0000000000000001"
	decID := "01HWABCDEF0000000000000002"
	heuID := "01HWABCDEF0000000000000003"

	_, err := pool.Exec(ctx, `
		INSERT INTO memories (id, type, content, fts_language, project_id, source, ingest_method, status, valid_from)
		VALUES ($1, 'episodic', 'the workflow orchestrates a deterministic consolidation pipeline', 'spanish',
		        'fts-test-project', 'fts-migration-test', 'direct', 'active', $2)`,
		memID, time.Now())
	require.NoError(t, err, "pre-seed memories row")

	_, err = pool.Exec(ctx, `
		INSERT INTO decisions (id, decision_key, title, description, rationale, fts_language, project_id,
		                       source, ingest_method, confidence_score, confidence_source)
		VALUES ($1, 'decision-fts-test', 'FTS switch decision', 'switch language to simple', 'simple is language-agnostic',
		        'spanish', 'fts-test-project', 'fts-migration-test', 'direct', 0.900, 'human')`,
		decID)
	require.NoError(t, err, "pre-seed decisions row")

	_, err = pool.Exec(ctx, `
		INSERT INTO heuristics (id, heuristic_key, condition, action, rationale, fts_language, project_id,
		                        source, ingest_method, confidence_score, confidence_source)
		VALUES ($1, 'heuristic-fts-test', 'when fts needed', 'use simple dictionary', 'simple indexes all languages',
		        'spanish', 'fts-test-project', 'fts-migration-test', 'direct', 0.800, 'human')`,
		heuID)
	require.NoError(t, err, "pre-seed heuristics row")

	// Verify pre-condition: all rows have fts_language='spanish'.
	assertFTSLanguage(t, ctx, pool, "memories", memID, "spanish")
	assertFTSLanguage(t, ctx, pool, "decisions", decID, "spanish")
	assertFTSLanguage(t, ctx, pool, "heuristics", heuID, "spanish")

	// --- Run migration 005 up ---
	runMigration005Up(t, pool)

	// --- Assert fts_language changed to 'simple' on all three tables ---
	assertFTSLanguage(t, ctx, pool, "memories", memID, "simple")
	assertFTSLanguage(t, ctx, pool, "decisions", decID, "simple")
	assertFTSLanguage(t, ctx, pool, "heuristics", heuID, "simple")

	// --- Assert column defaults are now 'simple' on all three tables ---
	assertColumnDefault(t, ctx, pool, "memories", "simple")
	assertColumnDefault(t, ctx, pool, "decisions", "simple")
	assertColumnDefault(t, ctx, pool, "heuristics", "simple")

	// --- Assert English FTS query returns the migrated row (search_vector rebuilt by trigger) ---
	assertFTSSearch(t, ctx, pool, "memories", "workflow", memID)
	assertFTSSearch(t, ctx, pool, "decisions", "switch", decID)
	assertFTSSearch(t, ctx, pool, "heuristics", "dictionary", heuID)

	// --- Idempotency: run 005.up a second time — must produce no error ---
	runMigration005Up(t, pool)

	// Row counts must not have changed.
	assertFTSLanguage(t, ctx, pool, "memories", memID, "simple")
	assertFTSLanguage(t, ctx, pool, "decisions", decID, "simple")
	assertFTSLanguage(t, ctx, pool, "heuristics", heuID, "simple")

	// --- Round-trip: down then up ---
	runMigration005Down(t, pool)

	// After down: fts_language back to 'spanish'.
	assertFTSLanguage(t, ctx, pool, "memories", memID, "spanish")
	assertFTSLanguage(t, ctx, pool, "decisions", decID, "spanish")
	assertFTSLanguage(t, ctx, pool, "heuristics", heuID, "spanish")

	// Column defaults must revert.
	assertColumnDefault(t, ctx, pool, "memories", "spanish")
	assertColumnDefault(t, ctx, pool, "decisions", "spanish")
	assertColumnDefault(t, ctx, pool, "heuristics", "spanish")

	// Re-apply up — must succeed cleanly.
	runMigration005Up(t, pool)

	assertFTSLanguage(t, ctx, pool, "memories", memID, "simple")
	assertFTSLanguage(t, ctx, pool, "decisions", decID, "simple")
	assertFTSLanguage(t, ctx, pool, "heuristics", heuID, "simple")
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func assertFTSLanguage(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, id, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx,
		`SELECT fts_language::text FROM `+table+` WHERE id = $1`, id).Scan(&got)
	require.NoError(t, err, "SELECT fts_language from %s id=%s", table, id)
	require.Equal(t, want, got, "fts_language on %s id=%s", table, id)
}

func assertColumnDefault(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, wantLang string) {
	t.Helper()
	// column_default for a REGCONFIG column is stored as e.g. "'simple'::regconfig".
	var colDefault string
	err := pool.QueryRow(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = $1
		  AND column_name  = 'fts_language'`, table).Scan(&colDefault)
	require.NoError(t, err, "query column_default for %s.fts_language", table)
	require.Contains(t, colDefault, wantLang, "column_default for %s.fts_language should contain %q", table, wantLang)
}

func assertFTSSearch(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, term, expectedID string) {
	t.Helper()
	// Use plainto_tsquery('simple', ...) against search_vector.
	// The trigger should have rebuilt search_vector after the UPDATE.
	var foundID string
	err := pool.QueryRow(ctx,
		`SELECT id FROM `+table+` WHERE id = $1 AND search_vector @@ plainto_tsquery('simple', $2)`,
		expectedID, term).Scan(&foundID)
	require.NoError(t, err, "FTS search on %s for term %q", table, term)
	require.Equal(t, expectedID, foundID, "FTS search on %s must find row %s for term %q", table, expectedID, term)
}

func runMigration005Up(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	sql := readMigrationFile(t, "005_fts_simple.up.sql")
	_, err := pool.Exec(context.Background(), sql)
	require.NoError(t, err, "run 005_fts_simple.up.sql")
}

func runMigration005Down(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	sql := readMigrationFile(t, "005_fts_simple.down.sql")
	_, err := pool.Exec(context.Background(), sql)
	require.NoError(t, err, "run 005_fts_simple.down.sql")
}

func readMigrationFile(t *testing.T, name string) string {
	t.Helper()
	candidates := []string{
		"../../../../migrations/postgres",
		"../../../migrations/postgres",
	}
	for _, c := range candidates {
		p := filepath.Join(c, name)
		data, err := os.ReadFile(p)
		if err == nil {
			return string(data)
		}
	}
	t.Fatalf("could not find migration file %s (tried %v)", name, candidates)
	return ""
}

// setupFTSTestDB starts a fresh Postgres testcontainer and applies
// migrations 001–004 so migration 005 can be exercised in isolation.
func setupFTSTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("fts_migration_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "start postgres container for FTS migration test")
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "get connection string")

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "create pool")
	t.Cleanup(func() { pool.Close() })

	// Apply base migrations (001–004) so that tables and triggers exist.
	baseFiles := []string{
		"001_initial_schema.up.sql",
		"002_retrieval_feedback.up.sql",
		"003_create_api_keys.up.sql",
		"004_memories_topic_key_unique.up.sql",
	}
	for _, f := range baseFiles {
		sql := readMigrationFile(t, f)
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "apply base migration %s", f)
	}

	return pool
}
