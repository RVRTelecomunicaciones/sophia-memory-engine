package purge

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// PurgedArtifacts tracks what was affected by a purge execution.
type PurgedArtifacts struct {
	FTSInvalidated       bool
	EmbeddingInvalidated bool
	CacheInvalidated     bool
	RelationsRemoved     int
	DerivedInvalidated   []shared.RecordID
}

// PurgeRecord represents a hard-purge request with full audit trail.
type PurgeRecord struct {
	ID              shared.RecordID
	TargetID        shared.RecordID
	TargetType      string
	Reason          shared.PurgeReason
	RequestedBy     string
	Scope           shared.Scope
	Status          shared.PurgeStatus
	AuditNote       string
	ArtifactsPurged PurgedArtifacts
	ExecutedAt      *time.Time
	ErrorDetail     *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewPurgeRecord creates a validated PurgeRecord in pending state.
func NewPurgeRecord(
	targetID shared.RecordID,
	targetType string,
	reason shared.PurgeReason,
	requestedBy, auditNote string,
	scope shared.Scope,
	clock shared.Clock,
) (*PurgeRecord, error) {
	var errs []shared.FieldError

	if !reason.IsValid() {
		errs = append(errs, shared.FieldError{
			Field:   "reason",
			Message: "invalid purge reason",
			Value:   reason,
		})
	}

	if requestedBy == "" {
		errs = append(errs, shared.FieldError{
			Field:   "requested_by",
			Message: "required",
			Value:   requestedBy,
		})
	}

	if auditNote == "" {
		errs = append(errs, shared.FieldError{
			Field:   "audit_note",
			Message: "required",
			Value:   auditNote,
		})
	}

	if len(errs) > 0 {
		return nil, shared.NewValidationError(errs...)
	}

	now := clock.Now()

	return &PurgeRecord{
		ID:          shared.NewRecordID(),
		TargetID:    targetID,
		TargetType:  targetType,
		Reason:      reason,
		RequestedBy: requestedBy,
		Scope:       scope,
		Status:      shared.PurgeStatusPending,
		AuditNote:   auditNote,
		ArtifactsPurged: PurgedArtifacts{
			DerivedInvalidated: []shared.RecordID{},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// MarkExecuting transitions the purge from pending to executing.
// Returns ErrNotActive if the current status is not pending.
func (p *PurgeRecord) MarkExecuting() error {
	if p.Status != shared.PurgeStatusPending {
		return shared.ErrNotActive
	}
	p.Status = shared.PurgeStatusExecuting
	return nil
}

// MarkExecuted transitions the purge to executed, recording the artifacts purged.
// Returns ErrAlreadyExecuted if the purge was already executed.
func (p *PurgeRecord) MarkExecuted(artifacts PurgedArtifacts, clock shared.Clock) error {
	if p.Status == shared.PurgeStatusExecuted {
		return shared.ErrAlreadyExecuted
	}
	now := clock.Now()
	p.Status = shared.PurgeStatusExecuted
	p.ExecutedAt = &now
	p.ArtifactsPurged = artifacts
	return nil
}

// MarkFailed transitions the purge to failed state with an error detail.
func (p *PurgeRecord) MarkFailed(detail string) {
	p.Status = shared.PurgeStatusFailed
	p.ErrorDetail = &detail
}
