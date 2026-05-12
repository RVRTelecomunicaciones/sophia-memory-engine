//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devSecret is the canonical local dev plaintext key defined in the migration.
const devSecret = "memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef"

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestAPIKeyPgRepository_FindByHash_DevSeedKey(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewAPIKeyPgRepository(pool)
	ctx := context.Background()

	key, err := repo.FindByHash(ctx, hashOf(devSecret))
	require.NoError(t, err)
	assert.Equal(t, "01J0DEVKEY00000000000000XX", key.ID)
	assert.Equal(t, auth.StatusActive, key.Status)
	assert.Equal(t, "sophia-dev", key.ProjectID)
	assert.Equal(t, "sophia-dev", key.TenantID)
}

func TestAPIKeyPgRepository_FindByHash_NotFound(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewAPIKeyPgRepository(pool)
	ctx := context.Background()

	_, err := repo.FindByHash(ctx, "0000000000000000000000000000000000000000000000000000000000000000")
	assert.ErrorIs(t, err, auth.ErrNotFound)
}

func TestAPIKeyPgRepository_TouchLastUsed_UpdatesTimestamp(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewAPIKeyPgRepository(pool)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	err := repo.TouchLastUsed(ctx, "01J0DEVKEY00000000000000XX", time.Now())
	require.NoError(t, err)

	// Re-fetch and confirm last_used_at was updated.
	key, err := repo.FindByHash(ctx, hashOf(devSecret))
	require.NoError(t, err)
	require.NotNil(t, key.LastUsedAt, "last_used_at must be set after TouchLastUsed")
	assert.True(t, key.LastUsedAt.After(before), "last_used_at must be after the before timestamp")
}

func TestAPIKeyPgRepository_FindByHash_RevokedKeyIsReturned(t *testing.T) {
	// The repo returns revoked rows — status check is the application's responsibility.
	pool := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert a revoked key directly.
	revokedSecret := "revoked00000000000000000000000000000000000000000000000000000000"
	revokedHash := hashOf(revokedSecret)
	_, err := pool.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, project_id, status) VALUES ($1, $2, $3, $4)`,
		"01JREVOKEDTEST00000000000A", revokedHash, "test-project", "revoked",
	)
	require.NoError(t, err)

	repo := persistence.NewAPIKeyPgRepository(pool)
	key, err := repo.FindByHash(ctx, revokedHash)
	require.NoError(t, err, "repo must return the row even if revoked")
	assert.Equal(t, auth.StatusRevoked, key.Status)
}
