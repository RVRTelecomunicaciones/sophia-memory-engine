package testhelper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// envExternalDSN, when set, tells SetupTestDB to connect to an externally
// managed PostgreSQL (already migrated by `cmd/migrate`) instead of starting
// a testcontainers instance and applying migrations inside the test process.
//
// This is the path the CI migrations gate uses: a Postgres service container
// is provisioned and migrated by `cmd/migrate up` before integration tests
// run, proving that `001_*.up.sql` + `002_*.up.sql` alone produce the schema
// every adapter requires (no in-test schema creation).
const envExternalDSN = "MEMORY_ENGINE_TEST_DSN"

// SetupTestDB returns a connection pool for integration tests.
//
//   - If MEMORY_ENGINE_TEST_DSN is set, it connects to that DSN and assumes
//     the schema has already been migrated externally (CI gate path).
//   - Otherwise, it starts a fresh Postgres 16 container via testcontainers
//     and applies the SQL migrations directly (developer laptop path).
//
// The pool is closed automatically when the test finishes.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	if dsn := os.Getenv(envExternalDSN); dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatalf("connect to external test DB at %s: %v", envExternalDSN, err)
		}
		t.Cleanup(func() { pool.Close() })
		// Sanity check the schema is in place. If the migration gate failed
		// to run before the tests, surface a clear error here.
		var n int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name IN ('memories','decisions','heuristics','relations',
				'purge_records','project_profiles','domain_events','retrieval_feedback')`).Scan(&n); err != nil {
			t.Fatalf("schema check on external DB: %v", err)
		}
		if n < 8 {
			t.Fatalf("external DB schema incomplete: only %d/8 expected tables present "+
				"(did `cmd/migrate up` run before the tests?)", n)
		}
		return pool
	}

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("memory_engine_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	runMigrations(t, pool)
	return pool
}

func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	migPath := findMigrationPath(t)

	migrations := []string{
		"001_initial_schema.up.sql",
		"002_retrieval_feedback.up.sql",
	}

	for _, m := range migrations {
		sql, err := os.ReadFile(filepath.Join(migPath, m))
		if err != nil {
			t.Fatalf("read migration %s: %v", m, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("run migration %s: %v", m, err)
		}
	}
}

func findMigrationPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../migrations/postgres",
		"../../../migrations/postgres",
		"../../../../migrations/postgres",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "001_initial_schema.up.sql")); err == nil {
			return c
		}
	}
	t.Fatal("could not find migrations/postgres directory")
	return ""
}
