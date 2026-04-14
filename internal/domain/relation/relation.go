package relation

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// Relation represents a typed, directed edge between two domain entities.
type Relation struct {
	ID        shared.RecordID
	SourceID  shared.RecordID
	TargetID  shared.RecordID
	Type      shared.RelationType
	Metadata  map[string]any
	Scope     shared.Scope
	Temporal  shared.TemporalMetadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WithMetadata sets metadata on the relation.
func WithMetadata(m map[string]any) func(*Relation) {
	return func(r *Relation) {
		r.Metadata = m
	}
}

// WithValidFrom sets the temporal ValidFrom field.
func WithValidFrom(t time.Time) func(*Relation) {
	return func(r *Relation) {
		r.Temporal.ValidFrom = &t
	}
}

// NewRelation creates a validated Relation with sensible defaults.
func NewRelation(
	sourceID, targetID shared.RecordID,
	relType shared.RelationType,
	scope shared.Scope,
	clock shared.Clock,
	opts ...func(*Relation),
) (*Relation, error) {
	var errs []shared.FieldError

	if sourceID.Equal(targetID) {
		errs = append(errs, shared.FieldError{
			Field:   "target_id",
			Message: "must differ from source_id (no self-reference)",
			Value:   targetID.String(),
		})
	}

	if !relType.IsValid() {
		errs = append(errs, shared.FieldError{
			Field:   "type",
			Message: "invalid relation type",
			Value:   relType,
		})
	}

	if len(errs) > 0 {
		return nil, shared.NewValidationError(errs...)
	}

	now := clock.Now()

	r := &Relation{
		ID:        shared.NewRecordID(),
		SourceID:  sourceID,
		TargetID:  targetID,
		Type:      relType,
		Metadata:  map[string]any{},
		Scope:     scope,
		Temporal:  shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}
