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

// SetupTestDB starts a PostgreSQL container, runs migrations, and returns a connection pool.
// The container and pool are automatically cleaned up when the test finishes.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

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
		"003_create_api_keys.up.sql",
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
