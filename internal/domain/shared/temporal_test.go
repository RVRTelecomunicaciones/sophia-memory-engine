package shared_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestTemporalMetadata_ComputeFreshness_Expired(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(now)

	pastExpiry := now.Add(-24 * time.Hour)
	tm := shared.TemporalMetadata{
		ValidUntil: &pastExpiry,
	}

	result := tm.ComputeFreshness(shared.DefaultFreshnessConfig(), clock)
	assert.Equal(t, shared.FreshnessLevelExpired, result)
	assert.True(t, tm.IsExpired(clock))
}

func TestTemporalMetadata_ComputeFreshness_Stale(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(now)

	accessed := now.Add(-45 * 24 * time.Hour)
	tm := shared.TemporalMetadata{
		LastAccessed: &accessed,
	}

	result := tm.ComputeFreshness(shared.DefaultFreshnessConfig(), clock)
	assert.Equal(t, shared.FreshnessLevelStale, result)
}

func TestTemporalMetadata_ComputeFreshness_Aging(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(now)

	accessed := now.Add(-10 * 24 * time.Hour)
	tm := shared.TemporalMetadata{
		LastAccessed: &accessed,
	}

	result := tm.ComputeFreshness(shared.DefaultFreshnessConfig(), clock)
	assert.Equal(t, shared.FreshnessLevelAging, result)
}

func TestTemporalMetadata_ComputeFreshness_Fresh(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(now)

	accessed := now.Add(-1 * 24 * time.Hour)
	tm := shared.TemporalMetadata{
		LastAccessed: &accessed,
	}

	result := tm.ComputeFreshness(shared.DefaultFreshnessConfig(), clock)
	assert.Equal(t, shared.FreshnessLevelFresh, result)
}

func TestTemporalMetadata_ComputeFreshness_Timeless(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(now)

	tm := shared.TemporalMetadata{} // all nil

	result := tm.ComputeFreshness(shared.DefaultFreshnessConfig(), clock)
	assert.Equal(t, shared.FreshnessLevelFresh, result, "timeless memory (all nil) must be fresh")
	assert.False(t, tm.IsExpired(clock), "timeless memory must not be expired")
}
