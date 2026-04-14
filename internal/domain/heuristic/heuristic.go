package heuristic

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// HeuristicRule represents a learned rule that guides agent behavior.
// It encodes a condition-action pair with confidence and temporal validity.
type HeuristicRule struct {
	ID           shared.RecordID
	HeuristicKey string
	Version      int
	Condition    string
	Action       string
	Rationale    string
	Scope        shared.Scope
	Provenance   shared.Provenance
	Confidence   shared.Confidence
	Enabled      bool
	Temporal     shared.TemporalMetadata
	FTSLanguage  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Option configures optional fields on a HeuristicRule during construction.
type Option func(*HeuristicRule)

// WithValidUntil sets the temporal ValidUntil field.
func WithValidUntil(t time.Time) Option {
	return func(h *HeuristicRule) {
		h.Temporal.ValidUntil = &t
	}
}

// WithEnabled overrides the default Enabled state.
func WithEnabled(enabled bool) Option {
	return func(h *HeuristicRule) {
		h.Enabled = enabled
	}
}

// WithFTSLanguage overrides the default FTS language.
func WithFTSLanguage(lang string) Option {
	return func(h *HeuristicRule) {
		h.FTSLanguage = lang
	}
}

// NewHeuristicRule creates a validated HeuristicRule with sensible defaults.
func NewHeuristicRule(
	key, condition, action, rationale string,
	scope shared.Scope,
	provenance shared.Provenance,
	confidence shared.Confidence,
	clock shared.Clock,
	version int,
	opts ...Option,
) (*HeuristicRule, error) {
	var errs []shared.FieldError

	if key == "" {
		errs = append(errs, shared.FieldError{
			Field:   "heuristic_key",
			Message: "required",
			Value:   key,
		})
	}

	if condition == "" {
		errs = append(errs, shared.FieldError{
			Field:   "condition",
			Message: "required",
			Value:   condition,
		})
	}

	if action == "" {
		errs = append(errs, shared.FieldError{
			Field:   "action",
			Message: "required",
			Value:   action,
		})
	}

	if rationale == "" {
		errs = append(errs, shared.FieldError{
			Field:   "rationale",
			Message: "required",
			Value:   rationale,
		})
	}

	if len(errs) > 0 {
		return nil, shared.NewValidationError(errs...)
	}

	now := clock.Now()

	h := &HeuristicRule{
		ID:           shared.NewRecordID(),
		HeuristicKey: key,
		Version:      version,
		Condition:    condition,
		Action:       action,
		Rationale:    rationale,
		Scope:        scope,
		Provenance:   provenance,
		Confidence:   confidence,
		Enabled:      true,
		Temporal:     shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		FTSLanguage:  "spanish",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h, nil
}

// SetEnabled toggles the enabled state of the heuristic rule.
func (h *HeuristicRule) SetEnabled(enabled bool) {
	h.Enabled = enabled
}

// IsActive returns true if the rule is enabled and not expired.
func (h *HeuristicRule) IsActive(clock shared.Clock) bool {
	if !h.Enabled {
		return false
	}
	return !h.Temporal.IsExpired(clock)
}
