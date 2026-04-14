package persistence

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that PgTxManager implements TransactionManager.
var _ outbound.TransactionManager = (*PgTxManager)(nil)

// PgTxManager implements TransactionManager using a pgxpool.Pool.
type PgTxManager struct {
	pool *pgxpool.Pool
}

// NewPgTxManager creates a new PgTxManager backed by the given connection pool.
func NewPgTxManager(pool *pgxpool.Pool) *PgTxManager {
	return &PgTxManager{pool: pool}
}

// WithTx executes fn within a PostgreSQL transaction.
// The transaction is committed if fn returns nil, rolled back otherwise.
func (m *PgTxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	txCtx := context.WithValue(ctx, txKey, tx)

	if err := fn(txCtx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
