//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRelation(t *testing.T, sourceID, targetID shared.RecordID, relType shared.RelationType, projectID string) *relation.Relation {
	t.Helper()
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, err := shared.NewScope(projectID)
	require.NoError(t, err)
	rel, err := relation.NewRelation(sourceID, targetID, relType, scope, clock)
	require.NoError(t, err)
	return rel
}

func TestRelationRepository_SaveAndFindFromSource(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	src := shared.NewRecordID()
	tgt1 := shared.NewRecordID()
	tgt2 := shared.NewRecordID()

	rel1 := makeRelation(t, src, tgt1, shared.RelationTypeRelatesTo, "proj-1")
	rel2 := makeRelation(t, src, tgt2, shared.RelationTypeDependsOn, "proj-1")

	require.NoError(t, repo.Save(ctx, rel1))
	require.NoError(t, repo.Save(ctx, rel2))

	scope, _ := shared.NewScope("proj-1")

	// Find all from source (scoped to proj-1)
	results, err := repo.FindFromSource(ctx, scope, src, nil)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Find with type filter
	relType := shared.RelationTypeRelatesTo
	results, err = repo.FindFromSource(ctx, scope, src, &relType)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, shared.RelationTypeRelatesTo, results[0].Type)
}

func TestRelationRepository_FindToTarget(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	src1 := shared.NewRecordID()
	src2 := shared.NewRecordID()
	tgt := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, src1, tgt, shared.RelationTypeReferences, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, src2, tgt, shared.RelationTypeReferences, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.FindToTarget(ctx, scope, tgt, nil)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestRelationRepository_Traverse_Depth1(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	// A → B → C
	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, b, c, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  1,
		Scope:     &scope,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1, "depth 1 should only return A→B")
	assert.Equal(t, 1, results[0].Depth)
}

func TestRelationRepository_Traverse_Depth2(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	// A → B → C → D
	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()
	d := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, b, c, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, c, d, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  2,
		Scope:     &scope,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2, "depth 2 should return A→B and B→C")
	assert.Equal(t, 1, results[0].Depth)
	assert.Equal(t, 2, results[1].Depth)
}

func TestRelationRepository_Traverse_HardCapDepth3(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	// A → B → C → D → E (chain of 4)
	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()
	d := shared.NewRecordID()
	e := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, b, c, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, c, d, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, d, e, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	// Request depth 10 — should be capped to 3
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  10,
		Scope:     &scope,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3, "hard cap at depth 3: A→B, B→C, C→D")
	assert.Equal(t, 3, results[2].Depth)
	// E should NOT be included
	for _, r := range results {
		assert.False(t, r.Relation.TargetID.Equal(e), "E should not be reachable at depth 3")
	}
}

func TestRelationRepository_Traverse_CyclePrevention(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	// A → B → C → A (cycle)
	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, b, c, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, c, a, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  3,
		Scope:     &scope,
	})
	require.NoError(t, err)
	// Should get A→B (depth 1), B→C (depth 2), but NOT C→A (cycle)
	assert.Len(t, results, 2, "cycle should be prevented: A→B, B→C only")
	for _, r := range results {
		assert.False(t, r.Relation.TargetID.Equal(a), "A should not be revisited")
	}
}

func TestRelationRepository_Traverse_ScopeFilter(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	// Relation in proj-1
	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeRelatesTo, "proj-1")))
	// Relation in proj-2
	require.NoError(t, repo.Save(ctx, makeRelation(t, a, c, shared.RelationTypeRelatesTo, "proj-2")))

	scope1, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  1,
		Scope:     &scope1,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1, "only proj-1 relation should be returned")
	assert.True(t, results[0].Relation.TargetID.Equal(b))
}

func TestRelationRepository_Traverse_TemporalFilter(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))

	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	// A→B: valid from Jan 1 to Jun 1
	scope, _ := shared.NewScope("proj-1")
	relAB, _ := relation.NewRelation(a, b, shared.RelationTypeRelatesTo, scope, clock,
		relation.WithValidFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	)
	relAB.Temporal.ValidUntil = new(time.Time)
	*relAB.Temporal.ValidUntil = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.Save(ctx, relAB))

	// A→C: valid from Jul 1, no end
	relAC, _ := relation.NewRelation(a, c, shared.RelationTypeRelatesTo, scope, clock,
		relation.WithValidFrom(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
	)
	require.NoError(t, repo.Save(ctx, relAC))

	// Query at Apr 14 — should only get A→B
	queryTime := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  1,
		Scope:     &scope,
		ValidAt:   &queryTime,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1, "only A→B should be valid at Apr 14")
	assert.True(t, results[0].Relation.TargetID.Equal(b))
}

func TestRelationRepository_Traverse_TypeFilter(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	require.NoError(t, repo.Save(ctx, makeRelation(t, a, b, shared.RelationTypeSupersedes, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, a, c, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseOutbound,
		MaxDepth:  1,
		Scope:     &scope,
		Types:     []shared.RelationType{shared.RelationTypeSupersedes},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1, "only supersedes relation should be returned")
	assert.Equal(t, shared.RelationTypeSupersedes, results[0].Relation.Type)
}

func TestRelationRepository_Traverse_Inbound(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	a := shared.NewRecordID()
	b := shared.NewRecordID()
	c := shared.NewRecordID()

	// B → A, C → A
	require.NoError(t, repo.Save(ctx, makeRelation(t, b, a, shared.RelationTypeReferences, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, c, a, shared.RelationTypeReferences, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	results, err := repo.Traverse(ctx, outbound.TraverseQuery{
		StartID:   a,
		Direction: outbound.TraverseInbound,
		MaxDepth:  1,
		Scope:     &scope,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2, "both B and C reference A")
}

func TestRelationRepository_DeleteByTarget(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	target := shared.NewRecordID()
	src1 := shared.NewRecordID()
	src2 := shared.NewRecordID()
	other := shared.NewRecordID()

	// Relations involving target
	require.NoError(t, repo.Save(ctx, makeRelation(t, src1, target, shared.RelationTypeRelatesTo, "proj-1")))
	require.NoError(t, repo.Save(ctx, makeRelation(t, target, src2, shared.RelationTypeDependsOn, "proj-1")))
	// Unrelated relation
	require.NoError(t, repo.Save(ctx, makeRelation(t, src1, other, shared.RelationTypeRelatesTo, "proj-1")))

	scope, _ := shared.NewScope("proj-1")
	deleted, err := repo.DeleteByTarget(ctx, scope, target)
	require.NoError(t, err)
	assert.Equal(t, 2, deleted, "should delete 2 relations involving target")

	// Verify unrelated relation still exists
	remaining, err := repo.FindFromSource(ctx, scope, src1, nil)
	require.NoError(t, err)
	assert.Len(t, remaining, 1, "unrelated relation should survive")
}

func TestRelationRepository_DeleteByTarget_NoRelations(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewRelationPgRepository(pool)
	ctx := context.Background()

	scope, _ := shared.NewScope("proj-1")
	deleted, err := repo.DeleteByTarget(ctx, scope, shared.NewRecordID())
	require.NoError(t, err)
	assert.Equal(t, 0, deleted, "no relations to delete is not an error")
}
