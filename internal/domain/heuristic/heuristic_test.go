package heuristic_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
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

func validProvenance(t *testing.T) shared.Provenance {
	t.Helper()
	p, err := shared.NewProvenance("agent-x", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	return p
}

func validConfidence(t *testing.T) shared.Confidence {
	t.Helper()
	c, err := shared.NewConfidence(0.9, shared.ConfidenceSourceAgent)
	require.NoError(t, err)
	return c
}

func TestNewHeuristicRule_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))

	h, err := heuristic.NewHeuristicRule(
		"prefer-interfaces",
		"when defining dependencies",
		"use interfaces instead of concrete types",
		"promotes testability and decoupling",
		validScope(t),
		validProvenance(t),
		validConfidence(t),
		clock,
		1,
	)

	require.NoError(t, err)
	assert.True(t, h.ID.IsValid())
	assert.Equal(t, "prefer-interfaces", h.HeuristicKey)
	assert.Equal(t, 1, h.Version)
	assert.Equal(t, "when defining dependencies", h.Condition)
	assert.Equal(t, "use interfaces instead of concrete types", h.Action)
	assert.Equal(t, "promotes testability and decoupling", h.Rationale)
	assert.True(t, h.Enabled)
	assert.Equal(t, "spanish", h.FTSLanguage)
	assert.Equal(t, shared.FreshnessLevelFresh, h.Temporal.Freshness)
	assert.Equal(t, clock.Now(), h.CreatedAt)
	assert.Equal(t, clock.Now(), h.UpdatedAt)
}

func TestNewHeuristicRule_RequiresConditionActionRationale(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))

	t.Run("missing key", func(t *testing.T) {
		_, err := heuristic.NewHeuristicRule(
			"", "cond", "act", "rat",
			validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("missing condition", func(t *testing.T) {
		_, err := heuristic.NewHeuristicRule(
			"key", "", "act", "rat",
			validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("missing action", func(t *testing.T) {
		_, err := heuristic.NewHeuristicRule(
			"key", "cond", "", "rat",
			validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("missing rationale", func(t *testing.T) {
		_, err := heuristic.NewHeuristicRule(
			"key", "cond", "act", "",
			validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("all missing", func(t *testing.T) {
		_, err := heuristic.NewHeuristicRule(
			"", "", "", "",
			validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})
}

func TestHeuristicRule_Toggle(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))

	h, err := heuristic.NewHeuristicRule(
		"key", "cond", "act", "rat",
		validScope(t), validProvenance(t), validConfidence(t), clock, 1,
	)
	require.NoError(t, err)
	assert.True(t, h.Enabled)

	h.SetEnabled(false)
	assert.False(t, h.Enabled)

	h.SetEnabled(true)
	assert.True(t, h.Enabled)
}

func TestHeuristicRule_IsActive_RespectsExpiry(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	expiry := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)

	h, err := heuristic.NewHeuristicRule(
		"key", "cond", "act", "rat",
		validScope(t), validProvenance(t), validConfidence(t), clock, 1,
		heuristic.WithValidUntil(expiry),
	)
	require.NoError(t, err)

	// Before expiry: active
	assert.True(t, h.IsActive(clock))

	// After expiry: inactive
	clock.Advance(25 * time.Hour)
	assert.False(t, h.IsActive(clock))
}

func TestHeuristicRule_IsActive_RespectsEnabled(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))

	h, err := heuristic.NewHeuristicRule(
		"key", "cond", "act", "rat",
		validScope(t), validProvenance(t), validConfidence(t), clock, 1,
	)
	require.NoError(t, err)

	assert.True(t, h.IsActive(clock))

	h.SetEnabled(false)
	assert.False(t, h.IsActive(clock))

	h.SetEnabled(true)
	assert.True(t, h.IsActive(clock))
}
