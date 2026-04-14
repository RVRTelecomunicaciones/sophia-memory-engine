package decision

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// Decision represents a recorded architectural or design decision
// with evidence, confidence, and lifecycle state tracking.
type Decision struct {
	ID           shared.RecordID
	DecisionKey  string
	Version      int
	Title        string
	Description  string
	Rationale    string
	Evidence     []shared.EvidenceRef
	Scope        shared.Scope
	Provenance   shared.Provenance
	Temporal     shared.TemporalMetadata
	Confidence   shared.Confidence
	Status       shared.DecisionStatus
	SupersededBy *shared.RecordID
	FTSLanguage  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewDecision creates a validated Decision with sensible defaults.
func NewDecision(
	key, title, description, rationale string,
	evidence []shared.EvidenceRef,
	scope shared.Scope,
	provenance shared.Provenance,
	confidence shared.Confidence,
	clock shared.Clock,
	version int,
) (*Decision, error) {
	var errs []shared.FieldError

	if key == "" {
		errs = append(errs, shared.FieldError{
			Field:   "decision_key",
			Message: "required",
			Value:   key,
		})
	}

	if title == "" {
		errs = append(errs, shared.FieldError{
			Field:   "title",
			Message: "required",
			Value:   title,
		})
	}

	if description == "" {
		errs = append(errs, shared.FieldError{
			Field:   "description",
			Message: "required",
			Value:   description,
		})
	}

	if rationale == "" {
		errs = append(errs, shared.FieldError{
			Field:   "rationale",
			Message: "required",
			Value:   rationale,
		})
	}

	if len(evidence) == 0 {
		errs = append(errs, shared.FieldError{
			Field:   "evidence",
			Message: "at least one evidence reference is required",
			Value:   nil,
		})
	}

	if len(errs) > 0 {
		return nil, shared.NewValidationError(errs...)
	}

	now := clock.Now()

	return &Decision{
		ID:          shared.NewRecordID(),
		DecisionKey: key,
		Version:     version,
		Title:       title,
		Description: description,
		Rationale:   rationale,
		Evidence:    evidence,
		Scope:       scope,
		Provenance:  provenance,
		Temporal:    shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		Confidence:  confidence,
		Status:      shared.DecisionStatusActive,
		FTSLanguage: "spanish",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Supersede transitions the decision from active to superseded.
// Returns ErrNotActive if the decision is not in active state.
func (d *Decision) Supersede(by shared.RecordID) error {
	if d.Status != shared.DecisionStatusActive {
		return shared.ErrNotActive
	}

	d.Status = shared.DecisionStatusSuperseded
	d.SupersededBy = &by

	return nil
}

// Contradict transitions the decision from active to contradicted.
// Returns ErrNotActive if the decision is not in active state.
func (d *Decision) Contradict() error {
	if d.Status != shared.DecisionStatusActive {
		return shared.ErrNotActive
	}

	d.Status = shared.DecisionStatusContradicted

	return nil
}

// Archive transitions the decision to archived state.
// Valid source states: active, superseded, contradicted.
// Returns ErrNotActive if already archived.
func (d *Decision) Archive() error {
	switch d.Status {
	case shared.DecisionStatusActive, shared.DecisionStatusSuperseded, shared.DecisionStatusContradicted:
		d.Status = shared.DecisionStatusArchived
		return nil
	default:
		return shared.ErrNotActive
	}
}
