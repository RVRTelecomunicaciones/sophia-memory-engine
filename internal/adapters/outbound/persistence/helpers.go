package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ctxKey is an unexported type for context keys in this package.
type ctxKey string

// txKey is the context key used to store a pgx.Tx in the context.
const txKey ctxKey = "pg_tx"

// DBTX is the interface satisfied by both pgxpool.Pool and pgx.Tx for query execution.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// getConn returns the transaction from context if present, otherwise the pool.
func getConn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// ptrStr converts a *string to an any suitable for SQL parameters (nil-safe).
func ptrStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// ptrTime converts a *time.Time to an any suitable for SQL parameters (nil-safe).
func ptrTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
