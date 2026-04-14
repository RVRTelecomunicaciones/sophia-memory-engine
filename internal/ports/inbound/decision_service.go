package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// RecordDecisionCmd carries the data needed to record a new decision.
type RecordDecisionCmd struct {
	DecisionKey string
	Title       string
	Description string
	Rationale   string
	Evidence    []shared.EvidenceRef
	FTSLanguage *string
	Scope       shared.Scope
	Provenance  shared.Provenance
	Confidence  shared.Confidence
	ValidFrom   *time.Time
}

// RecordDecisionResult is the outcome of a successful decision recording.
type RecordDecisionResult struct {
	ID         shared.RecordID
	Version    int
	Superseded *shared.RecordID
	CreatedAt  time.Time
}

// DecisionHistoryQuery defines the filters for retrieving decision history.
type DecisionHistoryQuery struct {
	DecisionKey string
	Scope       shared.Scope
}

// ContradictDecisionCmd carries the data needed to mark a decision as contradicted.
type ContradictDecisionCmd struct {
	TargetID       shared.RecordID
	ContradictedBy shared.RecordID
	Reason         string
	Provenance     shared.Provenance
}

// DecisionService defines the inbound port for decision lifecycle operations.
type DecisionService interface {
	Record(ctx context.Context, cmd RecordDecisionCmd) (*RecordDecisionResult, error)
	Get(ctx context.Context, id shared.RecordID) (*decision.Decision, error)
	GetHistory(ctx context.Context, query DecisionHistoryQuery) ([]decision.Decision, error)
	Contradict(ctx context.Context, cmd ContradictDecisionCmd) error
}
