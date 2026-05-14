//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryFixture builds a semantic memory record with the given topic key,
// project, and creation time so tests can assert ordering and scope.
func memoryFixture(
	t *testing.T,
	projectID, topicKey string,
	createdAt time.Time,
	scopeOpts ...shared.ScopeOption,
) *memory.MemoryRecord {
	t.Helper()
	clock := shared.NewFixedClock(createdAt)
	scope, err := shared.NewScope(projectID, scopeOpts...)
	require.NoError(t, err)
	prov, err := shared.NewProvenance("integration-test", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	rec, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"payload-"+topicKey+"-"+createdAt.Format(time.RFC3339Nano),
		scope,
		prov,
		clock,
		memory.WithTopicKey(topicKey),
	)
	require.NoError(t, err)
	return rec
}

func TestMemoryRepository_FindLatestActiveByTopicKey_ReturnsNewest(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	older := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	newer := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))

	require.NoError(t, repo.Save(ctx, older))
	require.NoError(t, repo.Save(ctx, newer))

	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope, "sdd/x/tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, newer.ID, got.ID, "should return the newer record")
}

func TestMemoryRepository_FindLatestActiveByTopicKey_ExcludesArchived(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	// Active older
	older := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	require.NoError(t, repo.Save(ctx, older))

	// Newer but archived
	newer := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	require.NoError(t, repo.Save(ctx, newer))
	require.NoError(t, repo.UpdateStatus(ctx, newer.ID, shared.MemoryStatusArchived))

	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope, "sdd/x/tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, older.ID, got.ID, "archived record must be excluded; older active wins")
	assert.Equal(t, shared.MemoryStatusActive, got.Status)
}

func TestMemoryRepository_FindLatestActiveByTopicKey_ProjectIsolation(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	// Same topic key, different projects
	rec1 := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	rec2 := memoryFixture(t, "proj-2", "sdd/x/tasks", time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	require.NoError(t, repo.Save(ctx, rec1))
	require.NoError(t, repo.Save(ctx, rec2))

	scope1, err := shared.NewScope("proj-1")
	require.NoError(t, err)

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope1, "sdd/x/tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, rec1.ID, got.ID, "must not leak proj-2 record into proj-1 query")
}

func TestMemoryRepository_FindLatestActiveByTopicKey_NotFound(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope, "does-not-exist")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestMemoryRepository_FindLatestActiveByTopicKey_OptionalScopeFilter(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	// Same project + topic, different repo_id values
	recA := memoryFixture(t, "proj-1", "sdd/x/tasks",
		time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		shared.WithRepoID("repo-A"),
	)
	recB := memoryFixture(t, "proj-1", "sdd/x/tasks",
		time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		shared.WithRepoID("repo-B"),
	)
	require.NoError(t, repo.Save(ctx, recA))
	require.NoError(t, repo.Save(ctx, recB))

	// Without repo filter: newest wins (recB)
	scopeAll, err := shared.NewScope("proj-1")
	require.NoError(t, err)
	got, err := repo.FindLatestActiveByTopicKey(ctx, scopeAll, "sdd/x/tasks")
	require.NoError(t, err)
	assert.Equal(t, recB.ID, got.ID)

	// With repo-A filter: only recA matches
	scopeA, err := shared.NewScope("proj-1", shared.WithRepoID("repo-A"))
	require.NoError(t, err)
	got, err = repo.FindLatestActiveByTopicKey(ctx, scopeA, "sdd/x/tasks")
	require.NoError(t, err)
	assert.Equal(t, recA.ID, got.ID)

	// With unknown repo: not found
	scopeMissing, err := shared.NewScope("proj-1", shared.WithRepoID("repo-C"))
	require.NoError(t, err)
	_, err = repo.FindLatestActiveByTopicKey(ctx, scopeMissing, "sdd/x/tasks")
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}
