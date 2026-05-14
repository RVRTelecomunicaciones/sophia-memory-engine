//go:build integration

// Package integration contains integration tests that require a real PostgreSQL instance.
// Run with: go test -tags=integration ./test/integration/... -count=1
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var (
	fixedClock = shared.NewFixedClock(time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC))
)

func scopeA(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("project-alpha")
	require.NoError(t, err)
	return s
}

func scopeB(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("project-beta")
	require.NoError(t, err)
	return s
}

func scopeATenant1(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("project-alpha", shared.WithTenantID("tenant-one"))
	require.NoError(t, err)
	return s
}

func scopeATenant2(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("project-alpha", shared.WithTenantID("tenant-two"))
	require.NoError(t, err)
	return s
}

func insertMemory(t *testing.T, ctx context.Context, repo *persistence.MemoryPgRepository, sc shared.Scope) *memory.MemoryRecord {
	t.Helper()
	prov, err := shared.NewProvenance("test-source", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	rec, err := memory.NewMemoryRecord(shared.MemoryTypeSemantic, "content for scope test", sc, prov, fixedClock)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, rec))
	return rec
}

// ---------------------------------------------------------------------------
// Test: Memory cross-project isolation
// ---------------------------------------------------------------------------

// TestMemoryRepo_ScopeEnforcement_CrossProjectIsolation is the smoking-gun test:
// insert a record in project-alpha, then attempt to read it with project-beta scope.
// The read MUST return ErrNotFound — not the actual record.
func TestMemoryRepo_ScopeEnforcement_CrossProjectIsolation(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	alpha := scopeA(t)
	beta := scopeB(t)

	// Insert in alpha.
	rec := insertMemory(t, ctx, repo, alpha)

	// Fetch with alpha scope — should succeed.
	got, err := repo.FindByID(ctx, alpha, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, rec.ID, got.ID)

	// Fetch with beta scope — must return ErrNotFound (not the record, not ErrForbidden).
	got, err = repo.FindByID(ctx, beta, rec.ID)
	assert.ErrorIs(t, err, shared.ErrNotFound,
		"cross-project FindByID must return ErrNotFound to prevent existence leak")
	assert.Nil(t, got)
}

// TestMemoryRepo_ScopeEnforcement_TenantIsolation verifies tenant-level isolation
// within the same project: tenant-one cannot read tenant-two's records.
func TestMemoryRepo_ScopeEnforcement_TenantIsolation(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	t1 := scopeATenant1(t)
	t2 := scopeATenant2(t)
	noTenant := scopeA(t) // project-alpha with no tenant (legacy row)

	// Insert records in t1, t2, and a cross-tenant (no tenant_id).
	recT1 := insertMemory(t, ctx, repo, t1)
	recT2 := insertMemory(t, ctx, repo, t2)
	recNT := insertMemory(t, ctx, repo, noTenant)

	// t1 key reads its own record.
	got, err := repo.FindByID(ctx, t1, recT1.ID)
	require.NoError(t, err)
	assert.Equal(t, recT1.ID, got.ID)

	// t1 key cannot read t2's record.
	_, err = repo.FindByID(ctx, t1, recT2.ID)
	assert.ErrorIs(t, err, shared.ErrNotFound,
		"tenant-one must not read tenant-two's records in the same project")

	// t1 key CAN read the no-tenant row (NULL tenant_id is visible to all in project).
	got, err = repo.FindByID(ctx, t1, recNT.ID)
	require.NoError(t, err)
	assert.Equal(t, recNT.ID, got.ID)

	// t2 key cannot read t1's record.
	_, err = repo.FindByID(ctx, t2, recT1.ID)
	assert.ErrorIs(t, err, shared.ErrNotFound,
		"tenant-two must not read tenant-one's records in the same project")

	// t2 key CAN read the no-tenant row.
	got, err = repo.FindByID(ctx, t2, recNT.ID)
	require.NoError(t, err)
	assert.Equal(t, recNT.ID, got.ID)
}

// TestMemoryRepo_ScopeEnforcement_UpdateStatus_CrossProject verifies that
// UpdateStatus with a cross-project scope returns ErrNotFound and does NOT
// modify the original record.
func TestMemoryRepo_ScopeEnforcement_UpdateStatus_CrossProject(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	alpha := scopeA(t)
	beta := scopeB(t)

	rec := insertMemory(t, ctx, repo, alpha)

	// Attempt UpdateStatus with beta scope.
	err := repo.UpdateStatus(ctx, beta, rec.ID, shared.MemoryStatusArchived)
	assert.ErrorIs(t, err, shared.ErrNotFound,
		"cross-project UpdateStatus must return ErrNotFound")

	// Confirm the original record was NOT modified.
	got, err := repo.FindByID(ctx, alpha, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, shared.MemoryStatusActive, got.Status,
		"original record status must be unchanged after cross-project UpdateStatus attempt")
}

// TestMemoryRepo_ScopeEnforcement_WipeContent_CrossProject verifies that
// WipeContent with a cross-project scope returns ErrNotFound and does NOT
// wipe the original record.
func TestMemoryRepo_ScopeEnforcement_WipeContent_CrossProject(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	alpha := scopeA(t)
	beta := scopeB(t)

	rec := insertMemory(t, ctx, repo, alpha)

	// Attempt WipeContent with beta scope.
	err := repo.WipeContent(ctx, beta, rec.ID)
	assert.ErrorIs(t, err, shared.ErrNotFound,
		"cross-project WipeContent must return ErrNotFound")

	// Confirm the original record was NOT wiped.
	got, err := repo.FindByID(ctx, alpha, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "content for scope test", got.Content,
		"content must be unchanged after cross-project WipeContent attempt")
	assert.Equal(t, shared.MemoryStatusActive, got.Status)
}
