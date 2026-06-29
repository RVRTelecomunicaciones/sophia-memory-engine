package consolidation

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Demoter evaluates whether an active skill should be demoted to "blocked"
// or "deprecated" based on the V4.1 §6.3 reachability table (D-M2-7).
//
// M3-reachable paths (skill-risk-instrumentation):
//   - active→blocked:     rollback_count >= 1 (first check — short-circuits ratio math)
//
// M2 reachable paths:
//   - active→blocked:     failure_count / max(usage_count, 1) > 0.15
//   - active→deprecated:  avg_retry_reduction < 0.05 (proxy via D-M2-11)
//
// M4+ unreachable paths (instrumentation gaps, noted in comments):
//   - active→deprecated:  deprecated_api_hits >= 1 (always 0 in M2/M3)
//   - active→deprecated:  last_stack_version mismatch (NULL in M2/M3)
//
// Precedence: "blocked" > "deprecated" when both conditions are simultaneously met.
// The Demoter satisfies the SkillDemoter interface.
type Demoter struct{}

// NewDemoter constructs a Demoter.
func NewDemoter() *Demoter { return &Demoter{} }

// failureRatioThreshold is the maximum acceptable failure ratio (D-M2-7).
const failureRatioThreshold = 0.15

// retryReductionThreshold is the minimum acceptable avg_retry_reduction (D-M2-7).
const retryReductionThreshold = 0.05

// Evaluate checks whether snap qualifies for demotion.
// Only "active" skills are evaluated — all other statuses return false.
// "blocked" takes precedence over "deprecated" when both conditions are met.
func (d *Demoter) Evaluate(_ context.Context, snap *outbound.SkillSnapshot) (string, bool) {
	if snap.Status != "active" {
		return "", false
	}

	// RollbackCount >= 1 → immediate block (M3-reachable via skill-risk-instrumentation).
	// This short-circuits before any ratio arithmetic, consistent with blocked-takes-precedence semantics.
	if snap.Metrics.RollbackCount >= 1 {
		return "blocked", true
	}

	// Check blocked condition first (higher precedence).
	usage := snap.Metrics.UsageCount
	if usage < 1 {
		usage = 1 // avoid division by zero; 1 failure → ratio 1.0 > threshold
	}
	failureRatio := float64(snap.Metrics.FailureCount) / float64(usage)
	shouldBlock := failureRatio > failureRatioThreshold

	// Check deprecated condition.
	// M4+ note: deprecated_api_hits and last_stack_version checks are unreachable
	// in M2 because those counters are never incremented (see D-M2-7 reachability table).
	shouldDeprecate := snap.Metrics.AvgRetryReduction < retryReductionThreshold &&
		snap.Metrics.AvgRetryReduction != 0 // zero means "not computed yet" — no demotion

	if shouldBlock {
		return "blocked", true
	}
	if shouldDeprecate {
		return "deprecated", true
	}
	return "", false
}
