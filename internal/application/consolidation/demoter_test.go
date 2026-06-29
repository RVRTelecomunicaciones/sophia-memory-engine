package consolidation_test

import (
	"context"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// L.1: failure_ratio > 0.15 → blocked
// L.2: failure_ratio <= 0.15 → no demotion
// ---------------------------------------------------------------------------

func TestDemoter_FailureRatio(t *testing.T) {
	tests := []struct {
		name       string
		failure    int
		usage      int
		wantOK     bool
		wantStatus string
	}{
		{
			name:       "failure=2,usage=10 ratio=0.20 → blocked",
			failure:    2,
			usage:      10,
			wantOK:     true,
			wantStatus: "blocked",
		},
		{
			name:    "failure=1,usage=10 ratio=0.10 → no demotion",
			failure: 1,
			usage:   10,
			wantOK:  false,
		},
		{
			name:   "failure=0,usage=10 ratio=0 → no demotion",
			failure: 0,
			usage:   10,
			wantOK:  false,
		},
	}

	d := consolidation.NewDemoter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := &outbound.SkillSnapshot{
				Status:    "active",
				RiskLevel: "medium",
				Metrics: outbound.SkillMetrics{
					FailureCount: tc.failure,
					UsageCount:   tc.usage,
				},
			}
			status, ok := d.Evaluate(context.Background(), snap)
			assert.Equal(t, tc.wantOK, ok, "unexpected demotion result")
			if tc.wantOK {
				assert.Equal(t, tc.wantStatus, status)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// L.3: avg_retry_reduction < 0.05 → deprecated (reachable M2 path via proxy)
// ---------------------------------------------------------------------------

func TestDemoter_LowRetryReduction_Deprecated(t *testing.T) {
	d := consolidation.NewDemoter()

	snap := &outbound.SkillSnapshot{
		Status:    "active",
		RiskLevel: "low",
		Metrics: outbound.SkillMetrics{
			AvgRetryReduction: 0.03, // below 0.05
			FailureCount:      0,
			UsageCount:        10,
		},
	}
	status, ok := d.Evaluate(context.Background(), snap)
	assert.True(t, ok, "avg_retry_reduction < 0.05 must trigger demotion")
	assert.Equal(t, "deprecated", status)
}

// ---------------------------------------------------------------------------
// L.4: both failure_ratio > 0.15 AND avg_retry_reduction < 0.05 → blocked (precedence)
// ---------------------------------------------------------------------------

func TestDemoter_BothConditions_BlockedTakesPrecedence(t *testing.T) {
	d := consolidation.NewDemoter()

	snap := &outbound.SkillSnapshot{
		Status:    "active",
		RiskLevel: "medium",
		Metrics: outbound.SkillMetrics{
			FailureCount:      3,
			UsageCount:        10, // ratio = 0.30 > 0.15
			AvgRetryReduction: 0.03, // < 0.05
		},
	}
	status, ok := d.Evaluate(context.Background(), snap)
	assert.True(t, ok)
	assert.Equal(t, "blocked", status, "blocked must take precedence over deprecated")
}

// ---------------------------------------------------------------------------
// L.6: rollback_count >= 1 → blocked (M3-reachable via skill-risk-instrumentation)
// ---------------------------------------------------------------------------

func TestDemoter_RollbackCount_Blocked(t *testing.T) {
	tests := []struct {
		name          string
		rollback      int
		failure       int
		usage         int
		retryReduction float64
		wantOK        bool
		wantStatus    string
	}{
		{
			name:       "rollback=1,failure=0,usage=10 → blocked",
			rollback:   1,
			failure:    0,
			usage:      10,
			wantOK:     true,
			wantStatus: "blocked",
		},
		{
			name:       "rollback=2 → blocked",
			rollback:   2,
			failure:    0,
			usage:      10,
			wantOK:     true,
			wantStatus: "blocked",
		},
		{
			name:    "rollback=0,failure=0,usage=10 → no demotion on rollback axis",
			rollback: 0,
			failure:  0,
			usage:    10,
			wantOK:  false,
		},
		{
			name:          "rollback=1 AND failureRatio>0.15 → blocked (rollback short-circuits, result still blocked)",
			rollback:      1,
			failure:       2,
			usage:         10, // ratio=0.20 > 0.15
			wantOK:        true,
			wantStatus:    "blocked",
		},
		{
			name:           "rollback=1 AND retryReduction=0.03 → blocked only (short-circuits deprecated eval)",
			rollback:       1,
			failure:        0,
			usage:          10,
			retryReduction: 0.03, // would trigger deprecated if rollback didn't fire first
			wantOK:         true,
			wantStatus:     "blocked",
		},
	}

	d := consolidation.NewDemoter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := &outbound.SkillSnapshot{
				Status:    "active",
				RiskLevel: "medium",
				Metrics: outbound.SkillMetrics{
					RollbackCount:     tc.rollback,
					FailureCount:      tc.failure,
					UsageCount:        tc.usage,
					AvgRetryReduction: tc.retryReduction,
				},
			}
			status, ok := d.Evaluate(context.Background(), snap)
			assert.Equal(t, tc.wantOK, ok, "unexpected demotion result for: %s", tc.name)
			if tc.wantOK {
				assert.Equal(t, tc.wantStatus, status)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// L.5: non-active skill → no demotion
// ---------------------------------------------------------------------------

func TestDemoter_NonActiveSkill_NoTransition(t *testing.T) {
	d := consolidation.NewDemoter()

	for _, status := range []string{"candidate", "validated", "blocked", "deprecated"} {
		snap := &outbound.SkillSnapshot{
			Status:    status,
			RiskLevel: "low",
			Metrics: outbound.SkillMetrics{
				FailureCount:      10,
				UsageCount:        10,
				AvgRetryReduction: 0.01,
			},
		}
		_, ok := d.Evaluate(context.Background(), snap)
		assert.False(t, ok, "status=%s must not trigger demoter", status)
	}
}
