package outbound

import (
	"context"
	"fmt"
)

// MetricsDelta carries the additive delta values sent to orch's
// PATCH /api/v1/skills/{id}/metrics endpoint.
// All integer fields are additive counters. AvgRetryReduction is a float that
// overwrites the stored value when non-zero.
type MetricsDelta struct {
	SuccessDelta           int     `json:"success_delta"`
	FailureDelta           int     `json:"failure_delta"`
	TestsPassedDelta       int     `json:"tests_passed_delta"`
	RollbackDelta          int     `json:"rollback_delta"`
	DeprecatedAPIHitsDelta int     `json:"deprecated_api_hits_delta"`
	UsageDelta             int     `json:"usage_delta"`
	AvgRetryReduction      float64 `json:"avg_retry_reduction"`
}

// SkillMetrics mirrors the orch skill metrics snapshot returned by GetSkill.
type SkillMetrics struct {
	UsageCount        int     `json:"usage_count"`
	SuccessCount      int     `json:"success_count"`
	FailureCount      int     `json:"failure_count"`
	TestsPassedCount  int     `json:"tests_passed_count"`
	DeprecatedAPIHits int     `json:"deprecated_api_hits"`
	RollbackCount     int     `json:"rollback_count"`
	AvgRetryReduction float64 `json:"avg_retry_reduction"`
}

// SkillSnapshot is the memory-engine's local representation of an orch skill,
// as returned by GET /api/v1/skills/{id}. Field names mirror the orch JSON
// wire format exactly.
type SkillSnapshot struct {
	SkillID   string       `json:"skill_id"`
	Status    string       `json:"status"`
	RiskLevel string       `json:"risk_level"`
	Version   string       `json:"version"`
	Metrics   SkillMetrics `json:"metrics"`
}

// SkillUsageRow mirrors a single row from orch's
// GET /api/v1/skills/usage?change_id=... response.
type SkillUsageRow struct {
	SkillUsageID  string `json:"skill_usage_id"`
	ChangeID      string `json:"change_id"`
	PhaseType     string `json:"phase_type"`
	SkillID       string `json:"skill_id"`
	SkillVersion  string `json:"skill_version"`
	Outcome       string `json:"outcome"`
	ApplyAttempts int    `json:"apply_attempts"`
}

// HTTPStatusError is returned by the HTTP adapter when the remote server
// responds with a non-2xx status that is NOT a transient 5xx error
// (e.g., 404, 422). It carries the HTTP status code for the caller to inspect.
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("orchhttp: remote returned HTTP %d: %s", e.StatusCode, e.Message)
}

// SkillsClient is the outbound port for interacting with the orch skills API.
// Implementations MUST send the configured API key on every request.
// The HTTP adapter retries transient 5xx failures up to MaxRetries times with
// exponential backoff.
type SkillsClient interface {
	// PatchMetrics applies additive delta counters to the named skill.
	// Returns *HTTPStatusError on 4xx; returns a generic error after all
	// retries are exhausted on 5xx.
	PatchMetrics(ctx context.Context, skillID string, delta MetricsDelta) error

	// PatchStatus transitions the named skill's lifecycle status.
	// Returns *HTTPStatusError on 4xx.
	PatchStatus(ctx context.Context, skillID string, status string, reason string) error

	// GetSkill returns the current snapshot of the named skill.
	// Returns *HTTPStatusError on 4xx.
	GetSkill(ctx context.Context, skillID string) (*SkillSnapshot, error)

	// GetUsage returns all skill_usage rows for the given change_id.
	GetUsage(ctx context.Context, changeID string) ([]SkillUsageRow, error)
}

// ProposerSkillView is the wider snapshot used ONLY by the governance proposer.
// It embeds the narrow SkillSnapshot (used by the worker's status-transition path)
// and adds the additive fields served by orch's GET /api/v1/skills/{id} since M3 (D-M3-2).
// Go's default json.Unmarshal silently drops unknown fields when decoding into
// the narrow SkillSnapshot, so the worker's existing decode path is unaffected.
type ProposerSkillView struct {
	SkillSnapshot
	SkillName   string         `json:"skill_name"`
	Scope       map[string]any `json:"scope"`
	AppliesWhen map[string]any `json:"applies_when"`
}

// ProposerSkillsClient is the governance proposer's outbound port for fetching
// the richer skill view. It is a separate interface from SkillsClient so the
// worker's narrow GetSkill path is never widened (D-M3-2).
type ProposerSkillsClient interface {
	// GetSkillWide returns the full proposer view of the named skill, including
	// the additive fields (skill_name, scope, applies_when) not present in
	// SkillSnapshot. Returns *HTTPStatusError on 4xx.
	GetSkillWide(ctx context.Context, skillID string) (*ProposerSkillView, error)
}
