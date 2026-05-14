// Command migrate applies SQL migrations under migrations/postgres against a
// target PostgreSQL database.
//
// Usage:
//
//	migrate -dir migrations/postgres -dsn "postgres://user:pass@host:port/db?sslmode=disable" up
//	migrate -dsn "$DATABASE_URL" up                  # default dir = ./migrations/postgres
//	migrate -dsn "$DATABASE_URL" status              # list applied migrations
//
// Migrations are *.up.sql files lexicographically sorted (e.g. 001_..., 002_...).
// A schema_migrations(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ) ledger
// tracks applied versions so re-runs are idempotent. Each migration is wrapped
// in its own transaction.
//
// This command is the project's "goose up" equivalent for local + CI use.
// It is intentionally dependency-free (only pgx + stdlib) so the CI gate can
// run without bootstrapping additional toolchains.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const ledgerSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dir := flag.String("dir", "migrations/postgres", "directory containing *.up.sql / *.down.sql files")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL DSN (defaults to $DATABASE_URL)")
	timeoutSec := flag.Int("timeout", 60, "per-migration timeout in seconds")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: migrate [-dir <path>] [-dsn <url>] [-timeout <seconds>] <up|status>")
		os.Exit(2)
	}
	cmd := args[0]

	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: -dsn is required (or set DATABASE_URL)")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second*10)
	defer cancel()

	conn, err := pgx.Connect(ctx, *dsn)
	if err != nil {
		slog.Error("connect failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	if _, err := conn.Exec(ctx, ledgerSQL); err != nil {
		slog.Error("create ledger failed", "error", err)
		os.Exit(1)
	}

	switch cmd {
	case "up":
		if err := runUp(ctx, conn, *dir, time.Duration(*timeoutSec)*time.Second); err != nil {
			slog.Error("migrate up failed", "error", err)
			os.Exit(1)
		}
	case "status":
		if err := runStatus(ctx, conn, *dir); err != nil {
			slog.Error("migrate status failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s (expected 'up' or 'status')\n", cmd)
		os.Exit(2)
	}
}

// runUp applies every *.up.sql file under dir whose version is not yet
// recorded in schema_migrations. Each migration runs in its own transaction.
func runUp(ctx context.Context, conn *pgx.Conn, dir string, perMigrationTimeout time.Duration) error {
	files, err := listUpFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		slog.Warn("no migrations found", "dir", dir)
		return nil
	}

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return err
	}

	for _, f := range files {
		version := versionOf(f)
		if _, ok := applied[version]; ok {
			slog.Info("skip (already applied)", "version", version)
			continue
		}

		path := filepath.Join(dir, f)
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		mctx, cancel := context.WithTimeout(ctx, perMigrationTimeout)
		err = applyOne(mctx, conn, version, string(body))
		cancel()
		if err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
		slog.Info("applied", "version", version, "file", f)
	}
	return nil
}

func applyOne(ctx context.Context, conn *pgx.Conn, version, sql string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// runStatus prints the migrations on disk and whether each has been applied.
func runStatus(ctx context.Context, conn *pgx.Conn, dir string) error {
	files, err := listUpFiles(dir)
	if err != nil {
		return err
	}
	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return err
	}

	fmt.Println("VERSION\tFILE\tAPPLIED")
	for _, f := range files {
		v := versionOf(f)
		mark := "no"
		if t, ok := applied[v]; ok {
			mark = "yes (" + t.Format(time.RFC3339) + ")"
		}
		fmt.Printf("%s\t%s\t%s\n", v, f, mark)
	}
	return nil
}

func listUpFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// versionOf strips the .up.sql suffix and returns the stable identifier
// stored in schema_migrations (e.g. "001_initial_schema").
func versionOf(filename string) string {
	return strings.TrimSuffix(filename, ".up.sql")
}

func loadApplied(ctx context.Context, conn *pgx.Conn) (map[string]time.Time, error) {
	rows, err := conn.Query(ctx, `SELECT version, applied_at FROM schema_migrations`)
	if err != nil {
		// On a brand new DB the ledger may not exist yet if this is called
		// before runUp creates it; treat the empty case explicitly.
		if isMissingLedger(err) {
			return map[string]time.Time{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := map[string]time.Time{}
	for rows.Next() {
		var v string
		var t time.Time
		if err := rows.Scan(&v, &t); err != nil {
			return nil, err
		}
		out[v] = t
	}
	return out, rows.Err()
}

func isMissingLedger(err error) bool {
	if err == nil {
		return false
	}
	// pgx wraps the SQLSTATE in the message; rather than depending on the
	// pg error code constants we check the substring once.
	return errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "schema_migrations")
}
