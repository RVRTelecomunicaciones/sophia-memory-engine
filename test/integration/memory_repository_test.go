//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncateMemoriesAfter is a helper that registers a t.Cleanup to TRUNCATE
// the `memories` table after the test finishes. The CI integration job
// reuses ONE pre-migrated Postgres database across all tests in this
// package (see testhelper.SetupTestDB), so without explicit cleanup a
// row left active by an earlier test collides with the partial unique
// index `idx_memories_topic_key_active_unique` (migration 004) when
// the next test tries to insert with the same (project_id, topic_key).
//
// CASCADE drops dependent rows in `memory_relations` etc. so the table
// remains in a fresh state for the next test.
func truncateMemoriesAfter(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "TRUNCATE memories CASCADE")
	})
}

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

func TestMemoryRepository_FindLatestActiveByTopicKey_ReturnsLatestUpsert(t *testing.T) {
	// After migration 004 (P1.3 topic_key uniqueness) the partial unique
	// index idx_memories_topic_key_active_unique guarantees at most ONE
	// active row per (project_id, tenant_id, topic_key). Two raw INSERTs
	// with the same triple are rejected; the production Ingest service
	// goes through UpsertByTopicKey which UPDATEs the existing active row
	// in-place. This test exercises that exact flow.
	pool := testhelper.SetupTestDB(t)
	truncateMemoriesAfter(t, pool)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	older := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	newer := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))

	_, _, err := repo.UpsertByTopicKey(ctx, older)
	require.NoError(t, err)
	_, inserted, err := repo.UpsertByTopicKey(ctx, newer)
	require.NoError(t, err)
	require.False(t, inserted, "second upsert with same topic_key must UPDATE, not INSERT")

	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope, "sdd/x/tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	// In-place upsert preserves the original row's ID but carries the
	// content of the latest upsert. Validate both: ID stayed, payload
	// reflects the newest ingest.
	assert.Equal(t, older.ID, got.ID, "in-place upsert keeps the original ID")
	assert.Equal(t, newer.Content, got.Content, "row carries the newest upsert's content")
}

func TestMemoryRepository_FindLatestActiveByTopicKey_ExcludesArchived(t *testing.T) {
	// The partial unique index excludes archived rows from its uniqueness
	// check (WHERE status = 'active'), so the supersession pattern is:
	// archive the existing active row first, then INSERT a fresh active
	// row with the same topic_key. This test exercises the bottom half
	// of that flow (no upsert path) and validates that archived rows are
	// correctly excluded from FindLatestActiveByTopicKey.
	pool := testhelper.SetupTestDB(t)
	truncateMemoriesAfter(t, pool)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	// Insert an early record, then archive it — this clears the unique
	// index so the next INSERT for the same (project, topic_key) succeeds.
	earlier := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	require.NoError(t, repo.Save(ctx, earlier))

	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateStatus(ctx, scope, earlier.ID, shared.MemoryStatusArchived))

	// Insert a later active record for the SAME topic_key — allowed
	// because earlier is now archived (not counted by the partial index).
	later := memoryFixture(t, "proj-1", "sdd/x/tasks", time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	require.NoError(t, repo.Save(ctx, later))

	got, err := repo.FindLatestActiveByTopicKey(ctx, scope, "sdd/x/tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, later.ID, got.ID, "archived record must be excluded; later active wins")
	assert.Equal(t, shared.MemoryStatusActive, got.Status)
}

func TestMemoryRepository_FindLatestActiveByTopicKey_ProjectIsolation(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	truncateMemoriesAfter(t, pool)
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
	// Migration 004's partial unique index keys on
	// (project_id, COALESCE(tenant_id, ''), topic_key) — repo_id is NOT
	// part of the uniqueness scope. So an active topic_key collision in
	// the same (project, tenant) is forbidden even when repo_ids differ.
	// We validate the OPTIONAL repo_id scope filter on a single active
	// row: the query must honor `scope.RepoID` when set and reject other
	// repos.
	pool := testhelper.SetupTestDB(t)
	truncateMemoriesAfter(t, pool)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	rec := memoryFixture(t, "proj-1", "sdd/x/tasks",
		time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		shared.WithRepoID("repo-A"),
	)
	require.NoError(t, repo.Save(ctx, rec))

	// Without repo filter: the single active row is returned.
	scopeAll, err := shared.NewScope("proj-1")
	require.NoError(t, err)
	got, err := repo.FindLatestActiveByTopicKey(ctx, scopeAll, "sdd/x/tasks")
	require.NoError(t, err)
	assert.Equal(t, rec.ID, got.ID)

	// With matching repo filter: same row is returned.
	scopeA, err := shared.NewScope("proj-1", shared.WithRepoID("repo-A"))
	require.NoError(t, err)
	got, err = repo.FindLatestActiveByTopicKey(ctx, scopeA, "sdd/x/tasks")
	require.NoError(t, err)
	assert.Equal(t, rec.ID, got.ID)

	// With a different repo filter: scope mismatch → not found.
	scopeOther, err := shared.NewScope("proj-1", shared.WithRepoID("repo-B"))
	require.NoError(t, err)
	_, err = repo.FindLatestActiveByTopicKey(ctx, scopeOther, "sdd/x/tasks")
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrNotFound))

	// With an unknown repo filter: idem — not found.
	scopeMissing, err := shared.NewScope("proj-1", shared.WithRepoID("repo-C"))
	require.NoError(t, err)
	_, err = repo.FindLatestActiveByTopicKey(ctx, scopeMissing, "sdd/x/tasks")
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}
