package retrieval_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
)

func TestComputeFinalScore(t *testing.T) {
	weights := config.RankingWeights{
		FTS:            0.25,
		Trigram:        0.15,
		Recency:        0.12,
		Importance:     0.13,
		TypeBoost:      0.10,
		Freshness:      0.10,
		ScopeExactness: 0.15,
	}

	signals := retrieval.RankingSignals{
		FTSScore:        0.8,
		TRGMScore:       0.6,
		RecencyBoost:    0.5,
		ImportanceScore: 0.7,
		TypeBoost:       0.5,
		FreshnessBoost:  1.0,
		ScopeExactness:  1.0,
	}

	expected := 0.25*0.8 + 0.15*0.6 + 0.12*0.5 + 0.13*0.7 + 0.10*0.5 + 0.10*1.0 + 0.15*1.0

	got := retrieval.ComputeFinalScore(weights, signals)

	assert.InDelta(t, expected, got, 1e-10)
}

func TestComputeRecencyBoost(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("just created", func(t *testing.T) {
		boost := retrieval.ComputeRecencyBoost(now, now)
		assert.InDelta(t, 1.0, boost, 1e-10)
	})

	t.Run("1 day ago", func(t *testing.T) {
		createdAt := now.Add(-24 * time.Hour)
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		assert.InDelta(t, 0.5, boost, 1e-10)
	})

	t.Run("30 days ago", func(t *testing.T) {
		createdAt := now.Add(-30 * 24 * time.Hour)
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		expected := 1.0 / (1.0 + 30.0)
		assert.InDelta(t, expected, boost, 1e-10)
		assert.True(t, math.Abs(boost-0.032258) < 0.001)
	})

	t.Run("future date clamped to 1.0", func(t *testing.T) {
		createdAt := now.Add(48 * time.Hour) // future
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		assert.InDelta(t, 1.0, boost, 1e-10)
	})
}
