package persistence

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that APIKeyPgRepository implements APIKeyRepository.
var _ outbound.APIKeyRepository = (*APIKeyPgRepository)(nil)

// APIKeyPgRepository is the PostgreSQL implementation of the APIKeyRepository port.
type APIKeyPgRepository struct {
	pool *pgxpool.Pool
}

// NewAPIKeyPgRepository creates a new APIKeyPgRepository backed by the given connection pool.
func NewAPIKeyPgRepository(pool *pgxpool.Pool) *APIKeyPgRepository {
	return &APIKeyPgRepository{pool: pool}
}

// FindByHash retrieves an APIKey by its SHA-256 hex digest.
// Returns auth.ErrNotFound if no matching row exists.
// The status column is returned as-is; revocation checks happen in the application layer.
func (r *APIKeyPgRepository) FindByHash(ctx context.Context, sha256Hex string) (*auth.APIKey, error) {
	const query = `
		SELECT id, key_hash, tenant_id, project_id, description, status, last_used_at, created_at
		FROM   api_keys
		WHERE  key_hash = $1`

	row := r.pool.QueryRow(ctx, query, sha256Hex)

	var (
		id          string
		keyHash     string
		tenantID    *string
		projectID   string
		description *string
		status      string
		lastUsedAt  *time.Time
		createdAt   time.Time
	)

	err := row.Scan(&id, &keyHash, &tenantID, &projectID, &description, &status, &lastUsedAt, &createdAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, auth.ErrNotFound
		}
		return nil, err
	}

	tenantIDStr := ""
	if tenantID != nil {
		tenantIDStr = *tenantID
	}
	descStr := ""
	if description != nil {
		descStr = *description
	}

	return auth.NewAPIKey(id, keyHash, tenantIDStr, projectID, descStr, status, lastUsedAt, createdAt)
}

// TouchLastUsed updates last_used_at for the given key ID.
// This is a best-effort write; errors are logged but not propagated.
// Callers should invoke this from a fire-and-forget goroutine.
func (r *APIKeyPgRepository) TouchLastUsed(ctx context.Context, keyID string, at time.Time) error {
	const query = `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`

	_, err := r.pool.Exec(ctx, query, at, keyID)
	if err != nil {
		slog.Warn("auth: failed to touch last_used_at",
			"key_id", keyID,
			"error", err,
		)
	}
	return err
}
