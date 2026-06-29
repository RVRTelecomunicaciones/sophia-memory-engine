package consolidation_test

import (
	"context"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// K.1: low-risk promotes at success=1,tests=1,failure=0; stays at success=0
// ---------------------------------------------------------------------------

func TestPromoter_LowRisk_PromotesAtThreshold(t *testing.T) {
	tests := []struct {
		name        string
		snap        *outbound.SkillSnapshot
		wantOK      bool
		wantStatus  string
	}{
		{
			name: "low promotes at success=1,tests=1,failure=0",
			snap: &outbound.SkillSnapshot{
				SkillID:   "skill-low-1",
				Status:    "candidate",
				RiskLevel: "low",
				Metrics: outbound.SkillMetrics{
					SuccessCount:     1,
					TestsPassedCount: 1,
					FailureCount:     0,
				},
			},
			wantOK:     true,
			wantStatus: "validated",
		},
		{
			name: "low stays candidate at success=0",
			snap: &outbound.SkillSnapshot{
				SkillID:   "skill-low-2",
				Status:    "candidate",
				RiskLevel: "low",
				Metrics: outbound.SkillMetrics{
					SuccessCount:     0,
					TestsPassedCount: 0,
					FailureCount:     0,
				},
			},
			wantOK: false,
		},
	}

	p := consolidation.NewPromoter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, ok := p.Evaluate(context.Background(), tc.snap)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantStatus, status)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// K.2: medium promotes at success=2,tests=2,failure=0,rollback=0,deprecated=0,retry>=0.20
//       stays candidate at success=1 (all else met)
// ---------------------------------------------------------------------------

func TestPromoter_MediumRisk_Thresholds(t *testing.T) {
	tests := []struct {
		name   string
		snap   *outbound.SkillSnapshot
		wantOK bool
	}{
		{
			name: "medium promotes at success=2 all else met",
			snap: &outbound.SkillSnapshot{
				Status:    "candidate",
				RiskLevel: "medium",
				Metrics: outbound.SkillMetrics{
					SuccessCount:      2,
					TestsPassedCount:  2,
					FailureCount:      0,
					RollbackCount:     0,
					DeprecatedAPIHits: 0,
					AvgRetryReduction: 0.25,
				},
			},
			wantOK: true,
		},
		{
			name: "medium stays candidate at success=1",
			snap: &outbound.SkillSnapshot{
				Status:    "candidate",
				RiskLevel: "medium",
				Metrics: outbound.SkillMetrics{
					SuccessCount:      1,
					TestsPassedCount:  2,
					FailureCount:      0,
					RollbackCount:     0,
					DeprecatedAPIHits: 0,
					AvgRetryReduction: 0.25,
				},
			},
			wantOK: false,
		},
		{
			name: "medium stays when avg_retry_reduction < 0.20",
			snap: &outbound.SkillSnapshot{
				Status:    "candidate",
				RiskLevel: "medium",
				Metrics: outbound.SkillMetrics{
					SuccessCount:      2,
					TestsPassedCount:  2,
					FailureCount:      0,
					RollbackCount:     0,
					DeprecatedAPIHits: 0,
					AvgRetryReduction: 0.15, // below threshold
				},
			},
			wantOK: false,
		},
	}

	p := consolidation.NewPromoter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := p.Evaluate(context.Background(), tc.snap)
			assert.Equal(t, tc.wantOK, ok, "unexpected promotion result for %s", tc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// K.3: high-risk uses same threshold as medium — success=1 does NOT promote
// ---------------------------------------------------------------------------

func TestPromoter_HighRisk_NotRelaxed(t *testing.T) {
	p := consolidation.NewPromoter()

	snap := &outbound.SkillSnapshot{
		Status:    "candidate",
		RiskLevel: "high",
		Metrics: outbound.SkillMetrics{
			SuccessCount:      1,
			TestsPassedCount:  1,
			FailureCount:      0,
			AvgRetryReduction: 0.25,
		},
	}
	_, ok := p.Evaluate(context.Background(), snap)
	assert.False(t, ok, "high-risk must not promote at success=1 (same threshold as medium: success>=2 required)")
}

// ---------------------------------------------------------------------------
// K.4: failure_count > 0 blocks promotion regardless of risk
// ---------------------------------------------------------------------------

func TestPromoter_FailureCountBlocksPromotion(t *testing.T) {
	p := consolidation.NewPromoter()

	snap := &outbound.SkillSnapshot{
		Status:    "candidate",
		RiskLevel: "low",
		Metrics: outbound.SkillMetrics{
			SuccessCount:     1,
			TestsPassedCount: 1,
			FailureCount:     1, // any failure blocks
		},
	}
	_, ok := p.Evaluate(context.Background(), snap)
	assert.False(t, ok, "failure_count>0 must block promotion")
}

// ---------------------------------------------------------------------------
// K.6: rollback_count >= 1 blocks promotion for medium/high/critical
//      Regression lock: promoter.go:79 guard must remain intact.
//      If any sub-test is RED on first run, the guard was already broken —
//      STOP and report; do NOT fix the promoter here (different change scope).
// ---------------------------------------------------------------------------

func TestPromoter_RollbackCount_BlocksPromotion_Regression(t *testing.T) {
	tests := []struct {
		name      string
		riskLevel string
		rollback  int
		wantOK    bool
	}{
		{
			name:      "medium rollback=1 → not promoted",
			riskLevel: "medium",
			rollback:  1,
			wantOK:    false,
		},
		{
			name:      "high rollback=1 → not promoted",
			riskLevel: "high",
			rollback:  1,
			wantOK:    false,
		},
		{
			name:      "critical rollback=1 → not promoted",
			riskLevel: "critical",
			rollback:  1,
			wantOK:    false,
		},
		{
			name:      "medium rollback=2 → not promoted",
			riskLevel: "medium",
			rollback:  2,
			wantOK:    false,
		},
		// POLICY: rollback gates promotion at EVERY risk level, including low.
		// The generic check (promoter.go:79) `RollbackCount > t.RollbackCount` with
		// the low-risk threshold's zero-value (0) blocks any rollback — consistent
		// with low already being gated on failure==0 (D-M2-6): a rollback is an
		// equally-strong negative signal. This is the intended behavior, confirmed
		// by operator decision and locked here (spec amended to match).
		{
			name:      "low rollback=1 → NOT promoted (rollback gates promotion at all risk levels)",
			riskLevel: "low",
			rollback:  1,
			wantOK:    false,
		},
	}

	p := consolidation.NewPromoter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var snap *outbound.SkillSnapshot
			switch tc.riskLevel {
			case "low":
				snap = &outbound.SkillSnapshot{
					Status:    "candidate",
					RiskLevel: "low",
					Metrics: outbound.SkillMetrics{
						SuccessCount:     1,
						TestsPassedCount: 1,
						FailureCount:     0,
						RollbackCount:    tc.rollback,
					},
				}
			default:
				snap = &outbound.SkillSnapshot{
					Status:    "candidate",
					RiskLevel: tc.riskLevel,
					Metrics: outbound.SkillMetrics{
						SuccessCount:      2,
						TestsPassedCount:  2,
						FailureCount:      0,
						RollbackCount:     tc.rollback,
						DeprecatedAPIHits: 0,
						AvgRetryReduction: 0.25,
					},
				}
			}
			_, ok := p.Evaluate(context.Background(), snap)
			assert.Equal(t, tc.wantOK, ok, "promotion result unexpected for: %s", tc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// K.5: non-candidate skill → no transition
// ---------------------------------------------------------------------------

func TestPromoter_NonCandidateSkill_NoTransition(t *testing.T) {
	p := consolidation.NewPromoter()

	for _, status := range []string{"active", "validated", "blocked"} {
		snap := &outbound.SkillSnapshot{
			Status:    status,
			RiskLevel: "low",
			Metrics: outbound.SkillMetrics{
				SuccessCount:     5,
				TestsPassedCount: 5,
			},
		}
		_, ok := p.Evaluate(context.Background(), snap)
		assert.False(t, ok, "status=%s must not trigger promoter", status)
	}
}
