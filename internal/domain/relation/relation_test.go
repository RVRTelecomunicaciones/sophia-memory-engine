package relation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validScope(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("test-project")
	require.NoError(t, err)
	return s
}

func TestNewRelation_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	sourceID := shared.NewRecordID()
	targetID := shared.NewRecordID()

	r, err := relation.NewRelation(
		sourceID, targetID,
		shared.RelationTypeDependsOn,
		validScope(t),
		clock,
	)

	require.NoError(t, err)
	assert.True(t, r.ID.IsValid())
	assert.True(t, r.SourceID.Equal(sourceID))
	assert.True(t, r.TargetID.Equal(targetID))
	assert.Equal(t, shared.RelationTypeDependsOn, r.Type)
	assert.NotNil(t, r.Metadata)
	assert.Empty(t, r.Metadata)
	assert.Equal(t, shared.FreshnessLevelFresh, r.Temporal.Freshness)
	assert.Equal(t, clock.Now(), r.CreatedAt)
	assert.Equal(t, clock.Now(), r.UpdatedAt)
}

func TestNewRelation_NoSelfReference(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	id := shared.NewRecordID()

	_, err := relation.NewRelation(
		id, id,
		shared.RelationTypeRelatesTo,
		validScope(t),
		clock,
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewRelation_InvalidType(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	sourceID := shared.NewRecordID()
	targetID := shared.NewRecordID()

	_, err := relation.NewRelation(
		sourceID, targetID,
		shared.RelationType("bogus"),
		validScope(t),
		clock,
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}
