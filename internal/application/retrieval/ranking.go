package retrieval

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
)

// RankingSignals holds the individual scoring signals for hybrid retrieval ranking.
type RankingSignals struct {
	FTSScore        float64
	TRGMScore       float64
	RecencyBoost    float64
	ImportanceScore float64
	TypeBoost       float64
	FreshnessBoost  float64
	ScopeExactness  float64
}

// ComputeFinalScore computes the weighted ranking score from individual signals.
func ComputeFinalScore(weights config.RankingWeights, signals RankingSignals) float64 {
	return weights.FTS*signals.FTSScore +
		weights.Trigram*signals.TRGMScore +
		weights.Recency*signals.RecencyBoost +
		weights.Importance*signals.ImportanceScore +
		weights.TypeBoost*signals.TypeBoost +
		weights.Freshness*signals.FreshnessBoost +
		weights.ScopeExactness*signals.ScopeExactness
}

// ComputeRecencyBoost returns a score between 0 and 1 based on age.
// Just created → ~1.0, 1 day ago → ~0.5, older → approaching 0.
func ComputeRecencyBoost(createdAt time.Time, now time.Time) float64 {
	days := now.Sub(createdAt).Hours() / 24.0
	if days < 0 {
		days = 0
	}
	return 1.0 / (1.0 + days)
}
