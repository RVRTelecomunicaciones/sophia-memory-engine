package consolidation

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// promotionThresholds holds the minimum metric values required for promotion.
// All integer fields must be exactly met (equality for failure/rollback/deprecated)
// or exceeded (for success/tests). AvgRetryReduction must be >= the threshold.
type promotionThresholds struct {
	SuccessCount      int
	TestsPassedCount  int
	FailureCount      int     // must be 0
	RollbackCount     int     // must be 0
	DeprecatedAPIHits int     // must be 0
	AvgRetryReduction float64 // minimum (ignored when 0)
}

// thresholdsForRisk returns the promotion thresholds for the given risk level.
// Per D-M2-6 (V4.1 §6.1):
//   - low: success>=1, tests_passed>=1 (failure==0 implied)
//   - medium/high/critical: success>=2, tests>=2, failure==0, rollback==0,
//     deprecated_api_hits==0, avg_retry_reduction>=0.20
//
// High and critical are NOT relaxed from medium — same thresholds apply.
func thresholdsForRisk(riskLevel string) promotionThresholds {
	switch riskLevel {
	case "low":
		return promotionThresholds{
			SuccessCount:     1,
			TestsPassedCount: 1,
		}
	case "medium", "high", "critical":
		return promotionThresholds{
			SuccessCount:      2,
			TestsPassedCount:  2,
			FailureCount:      0,
			RollbackCount:     0,
			DeprecatedAPIHits: 0,
			AvgRetryReduction: 0.20,
		}
	default:
		// Unknown risk level → never promote (safe default).
		return promotionThresholds{SuccessCount: 999}
	}
}

// Promoter evaluates whether a candidate skill qualifies for promotion to
// "validated" based on the risk-level-specific thresholds (V4.1 §6.1).
// It satisfies the SkillPromoter interface.
type Promoter struct{}

// NewPromoter constructs a Promoter.
func NewPromoter() *Promoter { return &Promoter{} }

// Evaluate checks whether snap meets the promotion thresholds.
// Returns ("validated", true) when all thresholds are met, ("", false) otherwise.
// Only "candidate" skills are evaluated — all other statuses return false.
func (p *Promoter) Evaluate(_ context.Context, snap *outbound.SkillSnapshot) (string, bool) {
	if snap.Status != "candidate" {
		return "", false
	}

	// Any non-zero failure count blocks promotion regardless of risk level.
	if snap.Metrics.FailureCount > 0 {
		return "", false
	}

	t := thresholdsForRisk(snap.RiskLevel)

	if snap.Metrics.SuccessCount < t.SuccessCount {
		return "", false
	}
	if snap.Metrics.TestsPassedCount < t.TestsPassedCount {
		return "", false
	}
	if snap.Metrics.RollbackCount > t.RollbackCount {
		return "", false
	}
	if snap.Metrics.DeprecatedAPIHits > t.DeprecatedAPIHits {
		return "", false
	}
	if t.AvgRetryReduction > 0 && snap.Metrics.AvgRetryReduction < t.AvgRetryReduction {
		return "", false
	}

	return "validated", true
}
