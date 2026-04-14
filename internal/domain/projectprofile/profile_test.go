package projectprofile_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectProfile_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	scope, err := shared.NewScope("my-project")
	require.NoError(t, err)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	sourceRange, err := shared.NewTimeRange(from, to)
	require.NoError(t, err)

	p, err := projectprofile.NewProjectProfile(
		"my-project",
		"A Go memory engine project",
		scope,
		sourceRange,
		clock,
		1,
	)

	require.NoError(t, err)
	assert.True(t, p.ID.IsValid())
	assert.Equal(t, "my-project", p.ProjectID)
	assert.Equal(t, 1, p.Version)
	assert.Equal(t, "A Go memory engine project", p.Summary)
	assert.Empty(t, p.ActiveDecisions)
	assert.Empty(t, p.TopHeuristics)
	assert.Empty(t, p.Patterns)
	assert.Empty(t, p.ArchSignals)
	assert.Equal(t, clock.Now(), p.Freshness.GeneratedAt)
	assert.Equal(t, clock.Now().Add(24*time.Hour), p.Freshness.StaleAfter)
	assert.Equal(t, sourceRange, p.Freshness.SourceTimeRange)
	assert.Equal(t, clock.Now(), p.CreatedAt)
	assert.Equal(t, clock.Now(), p.UpdatedAt)

	// Empty projectID must fail
	_, err = projectprofile.NewProjectProfile(
		"", "summary", scope, sourceRange, clock, 1,
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}
