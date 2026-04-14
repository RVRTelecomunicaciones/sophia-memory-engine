package purge_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
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

func TestNewPurgeRecord_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	targetID := shared.NewRecordID()

	p, err := purge.NewPurgeRecord(
		targetID,
		"memory",
		shared.PurgeReasonSecretLeak,
		"admin@example.com",
		"API key exposed in episodic memory",
		validScope(t),
		clock,
	)

	require.NoError(t, err)
	assert.True(t, p.ID.IsValid())
	assert.True(t, p.TargetID.Equal(targetID))
	assert.Equal(t, "memory", p.TargetType)
	assert.Equal(t, shared.PurgeReasonSecretLeak, p.Reason)
	assert.Equal(t, "admin@example.com", p.RequestedBy)
	assert.Equal(t, "API key exposed in episodic memory", p.AuditNote)
	assert.Equal(t, shared.PurgeStatusPending, p.Status)
	assert.Nil(t, p.ExecutedAt)
	assert.Nil(t, p.ErrorDetail)
	assert.Equal(t, clock.Now(), p.CreatedAt)
}

func TestPurgeRecord_Execute(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	targetID := shared.NewRecordID()

	p, err := purge.NewPurgeRecord(
		targetID, "memory", shared.PurgeReasonCompliance,
		"admin@example.com", "GDPR request",
		validScope(t), clock,
	)
	require.NoError(t, err)

	// pending → executing
	err = p.MarkExecuting()
	require.NoError(t, err)
	assert.Equal(t, shared.PurgeStatusExecuting, p.Status)

	// executing → executed
	clock.Advance(5 * time.Second)
	artifacts := purge.PurgedArtifacts{
		FTSInvalidated:       true,
		EmbeddingInvalidated: true,
		CacheInvalidated:     true,
		RelationsRemoved:     3,
		DerivedInvalidated:   []shared.RecordID{shared.NewRecordID()},
	}

	err = p.MarkExecuted(artifacts, clock)
	require.NoError(t, err)
	assert.Equal(t, shared.PurgeStatusExecuted, p.Status)
	assert.NotNil(t, p.ExecutedAt)
	assert.Equal(t, clock.Now(), *p.ExecutedAt)
	assert.True(t, p.ArtifactsPurged.FTSInvalidated)
	assert.Equal(t, 3, p.ArtifactsPurged.RelationsRemoved)
}

func TestPurgeRecord_Execute_AlreadyExecuted(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	targetID := shared.NewRecordID()

	p, err := purge.NewPurgeRecord(
		targetID, "memory", shared.PurgeReasonSensitiveContent,
		"admin@example.com", "PII detected",
		validScope(t), clock,
	)
	require.NoError(t, err)

	require.NoError(t, p.MarkExecuting())
	require.NoError(t, p.MarkExecuted(purge.PurgedArtifacts{}, clock))

	// second execution must fail
	err = p.MarkExecuted(purge.PurgedArtifacts{}, clock)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrAlreadyExecuted))
}
