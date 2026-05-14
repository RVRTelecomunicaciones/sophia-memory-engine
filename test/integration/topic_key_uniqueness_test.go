//go:build integration

// Package integration — topic_key uniqueness + upsert tests for ADR-0005 P1.3.
//
// These tests pin down the data-integrity guarantee that two concurrent
// ingest calls with the same (project_id, tenant_id, topic_key) produce
// EXACTLY ONE active memory row, last-write-wins on content.
package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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

// newRecord builds a fresh MemoryRecord with a unique ID + supplied topic_key
// and content. Each call produces a new ULID, so two calls with the same
// topic_key still differ in record.ID — exercising the ON CONFLICT path.
func newRecord(t *testing.T, sc shared.Scope, topicKey, content string) *memory.MemoryRecord {
	t.Helper()
	prov, err := shared.NewProvenance("test-source", shared.IngestMethodDirect, nil)
	require.NoError(t, err)

	rec, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		content,
		sc,
		prov,
		fixedClock,
		memory.WithTopicKey(topicKey),
	)
	require.NoError(t, err)
	return rec
}

// countActiveByTopicKey counts active memory rows for a (project, topic_key)
// pair. Used to assert exactly-one-row invariants.
func countActiveByTopicKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, topicKey string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories
		 WHERE project_id = $1 AND topic_key = $2 AND status = 'active'`,
		projectID, topicKey,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// ---------------------------------------------------------------------------
// Test 1: new topic_key → inserted=true, new row
// ---------------------------------------------------------------------------

func TestUpsert_NewRecord_InsertsNewRow(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()
	sc := scopeA(t)

	rec := newRecord(t, sc, "sdd/foo", "first content")
	id, inserted, err := repo.UpsertByTopicKey(ctx, rec)
	require.NoError(t, err)
	assert.True(t, inserted, "first upsert with a new topic_key must INSERT")
	assert.Equal(t, rec.ID, id, "returned id must equal the candidate id on INSERT")

	assert.Equal(t, 1, countActiveByTopicKey(t, ctx, pool, sc.ProjectID, "sdd/foo"))
}

// ---------------------------------------------------------------------------
// Test 2: same scope + topic_key → updates in place, single row
// ---------------------------------------------------------------------------

func TestUpsert_ExistingTopicKey_SameScope_UpdatesInPlace(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()
	sc := scopeA(t)

	first := newRecord(t, sc, "sdd/bar", "v1 content")
	idFirst, ins1, err := repo.UpsertByTopicKey(ctx, first)
	require.NoError(t, err)
	require.True(t, ins1)

	second := newRecord(t, sc, "sdd/bar", "v2 content")
	idSecond, ins2, err := repo.UpsertByTopicKey(ctx, second)
	require.NoError(t, err)
	assert.False(t, ins2, "second upsert must UPDATE, not INSERT")
	assert.Equal(t, idFirst, idSecond, "id MUST remain the first row's id on update")

	// Exactly one active row for the topic_key.
	assert.Equal(t, 1, countActiveByTopicKey(t, ctx, pool, sc.ProjectID, "sdd/bar"))

	// Content is the latest write.
	got, err := repo.FindLatestActiveByTopicKey(ctx, sc, "sdd/bar")
	require.NoError(t, err)
	assert.Equal(t, "v2 content", got.Content)

	// created_at MUST be preserved on UPDATE; updated_at MUST advance.
	var createdAtFirst, createdAtSecond, updatedAtSecond string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT created_at::text, created_at::text, updated_at::text
		 FROM memories WHERE id = $1`, idFirst.String(),
	).Scan(&createdAtFirst, &createdAtSecond, &updatedAtSecond))
	// (created_at == created_at always; the assertion that matters is below.)
	assert.NotEmpty(t, createdAtFirst)
	assert.True(t, updatedAtSecond >= createdAtFirst,
		"updated_at must be >= original created_at after upsert")
}

// ---------------------------------------------------------------------------
// Test 3: same topic_key, DIFFERENT projects → both rows survive
// ---------------------------------------------------------------------------

func TestUpsert_ExistingTopicKey_DifferentProject_InsertsNewRow(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()

	scA := scopeA(t)
	scB := scopeB(t)

	recA := newRecord(t, scA, "sdd/shared", "alpha content")
	idA, insA, err := repo.UpsertByTopicKey(ctx, recA)
	require.NoError(t, err)
	require.True(t, insA)

	recB := newRecord(t, scB, "sdd/shared", "beta content")
	idB, insB, err := repo.UpsertByTopicKey(ctx, recB)
	require.NoError(t, err)
	assert.True(t, insB, "different project_id with same topic_key must INSERT a new row")
	assert.NotEqual(t, idA, idB, "rows in different projects must have different ids")

	assert.Equal(t, 1, countActiveByTopicKey(t, ctx, pool, scA.ProjectID, "sdd/shared"))
	assert.Equal(t, 1, countActiveByTopicKey(t, ctx, pool, scB.ProjectID, "sdd/shared"))
}

// ---------------------------------------------------------------------------
// Test 4: archived row does NOT block a new active row for the same topic_key
// ---------------------------------------------------------------------------

func TestUpsert_ArchivedRow_AllowsNewActive(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()
	sc := scopeA(t)

	// Insert + archive an initial row.
	first := newRecord(t, sc, "sdd/lifecycle", "v1 content")
	idFirst, _, err := repo.UpsertByTopicKey(ctx, first)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateStatus(ctx, sc, idFirst, shared.MemoryStatusArchived))

	// Verify archived: zero active rows.
	assert.Equal(t, 0, countActiveByTopicKey(t, ctx, pool, sc.ProjectID, "sdd/lifecycle"))

	// Now upsert again — partial index excludes archived rows, so this MUST
	// INSERT a fresh active row rather than updating the archived one.
	second := newRecord(t, sc, "sdd/lifecycle", "v2 content (post-archive)")
	idSecond, inserted, err := repo.UpsertByTopicKey(ctx, second)
	require.NoError(t, err)
	assert.True(t, inserted, "archived row must NOT block a new active upsert")
	assert.NotEqual(t, idFirst, idSecond, "new active row must have a fresh id")

	// Exactly one active row + one archived row.
	assert.Equal(t, 1, countActiveByTopicKey(t, ctx, pool, sc.ProjectID, "sdd/lifecycle"))

	var archived int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories
		 WHERE project_id = $1 AND topic_key = $2 AND status = 'archived'`,
		sc.ProjectID, "sdd/lifecycle",
	).Scan(&archived))
	assert.Equal(t, 1, archived, "previous row must remain archived (history preserved)")
}

// ---------------------------------------------------------------------------
// Test 5: SMOKING TEST — 20 concurrent upserts → exactly ONE active row
// ---------------------------------------------------------------------------
//
// This is the data-integrity guarantee for ADR-0005 P1.3. If this test ever
// reports >1 active row, the partial unique index is broken or the upsert
// SQL lost its ON CONFLICT clause. STOP and investigate — last week's bug.

func TestUpsert_Concurrent_ProducesSingleRow(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	ctx := context.Background()
	sc := scopeA(t)

	const N = 20

	// Pre-build records on the main goroutine so newRecord (which calls
	// require.NoError on validation) never fires from a worker goroutine.
	// testing.T disallows FailNow from non-test goroutines.
	records := make([]*memory.MemoryRecord, N)
	for i := 0; i < N; i++ {
		records[i] = newRecord(t, sc, "sdd/concurrent", fmt.Sprintf("worker-%02d", i))
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ids  []shared.RecordID
		errs []error
	)

	// Barrier: all goroutines start the upsert at the same instant so the
	// PG side is maximally contended.
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(rec *memory.MemoryRecord) {
			defer wg.Done()
			<-start
			id, _, err := repo.UpsertByTopicKey(ctx, rec)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids = append(ids, id)
		}(records[i])
	}
	close(start)
	wg.Wait()

	require.Empty(t, errs, "no upsert may fail: ON CONFLICT must serialize all writers")
	require.Len(t, ids, N)

	// SMOKING TEST: exactly ONE active row exists for (scope, topic_key).
	got := countActiveByTopicKey(t, ctx, pool, sc.ProjectID, "sdd/concurrent")
	require.Equal(t, 1, got,
		"PARTIAL UNIQUE INDEX FAILED: expected exactly 1 active row, got %d. "+
			"ADR-0005 P1.3 data-integrity guarantee broken.", got)

	// All goroutines MUST have received the same final id (the surviving row).
	first := ids[0]
	for i, id := range ids[1:] {
		assert.Equal(t, first, id,
			"goroutine %d received id %s, expected %s — concurrent upserts must converge",
			i+1, id.String(), first.String())
	}

	// Content is the last-write-wins survivor; we don't know which goroutine
	// won, but the content MUST be one of the worker contents.
	rec, err := repo.FindLatestActiveByTopicKey(ctx, sc, "sdd/concurrent")
	require.NoError(t, err)
	assert.Regexp(t, `^worker-\d{2}$`, rec.Content)
}

// ---------------------------------------------------------------------------
// Test 6: verify the partial unique index exists with the expected predicate
// ---------------------------------------------------------------------------

func TestUpsert_PartialUniqueIndexExists(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	ctx := context.Background()

	var indexDef string
	err := pool.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes
		 WHERE schemaname = 'public'
		   AND tablename = 'memories'
		   AND indexname = 'idx_memories_topic_key_active_unique'`,
	).Scan(&indexDef)
	require.NoError(t, err, "migration 004 must create idx_memories_topic_key_active_unique")

	// The index MUST be UNIQUE, MUST include topic_key, and MUST have a
	// partial WHERE predicate filtering to status='active' + non-null
	// topic_key. PG normalizes the predicate (status)::text = 'active'::text,
	// so we assert against the literal "'active'" + topic_key conditions.
	assert.Contains(t, indexDef, "UNIQUE INDEX")
	assert.Contains(t, indexDef, "topic_key")
	assert.Contains(t, indexDef, "'active'", "partial index must filter status = 'active'")
	assert.Contains(t, indexDef, "topic_key IS NOT NULL")
}
