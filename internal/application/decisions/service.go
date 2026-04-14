package decisions

import (
	"context"
	"errors"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

var _ inbound.DecisionService = (*Service)(nil)

// Service implements inbound.DecisionService for decision lifecycle operations.
type Service struct {
	decRepo  outbound.DecisionRepository
	relRepo  outbound.RelationRepository
	txMgr    outbound.TransactionManager
	eventPub outbound.EventPublisher
	clock    shared.Clock
}

// NewService creates a new decision Service with the given dependencies.
func NewService(
	decRepo outbound.DecisionRepository,
	relRepo outbound.RelationRepository,
	txMgr outbound.TransactionManager,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		decRepo:  decRepo,
		relRepo:  relRepo,
		txMgr:    txMgr,
		eventPub: eventPub,
		clock:    clock,
	}
}

// Record creates a new decision, optionally superseding an existing active one.
func (s *Service) Record(ctx context.Context, cmd inbound.RecordDecisionCmd) (*inbound.RecordDecisionResult, error) {
	// Look for existing active decision with same key+scope.
	existing, err := s.decRepo.FindActiveByKey(ctx, cmd.DecisionKey, cmd.Scope)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}

	version := 1
	if existing != nil {
		version = existing.Version + 1
	}

	newDec, err := decision.NewDecision(
		cmd.DecisionKey,
		cmd.Title,
		cmd.Description,
		cmd.Rationale,
		cmd.Evidence,
		cmd.Scope,
		cmd.Provenance,
		cmd.Confidence,
		s.clock,
		version,
	)
	if err != nil {
		return nil, err
	}

	var superseded *shared.RecordID

	if existing != nil {
		// Supersede old in a transaction.
		err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
			if err := existing.Supersede(newDec.ID); err != nil {
				return err
			}
			if err := s.decRepo.UpdateStatus(txCtx, existing.ID, existing.Status, existing.SupersededBy); err != nil {
				return err
			}
			if err := s.decRepo.Save(txCtx, newDec); err != nil {
				return err
			}

			rel, err := relation.NewRelation(
				newDec.ID,
				existing.ID,
				shared.RelationTypeSupersedes,
				cmd.Scope,
				s.clock,
			)
			if err != nil {
				return err
			}
			return s.relRepo.Save(txCtx, rel)
		})
		if err != nil {
			return nil, err
		}
		superseded = &existing.ID
	} else {
		if err := s.decRepo.Save(ctx, newDec); err != nil {
			return nil, err
		}
	}

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeDecisionRecorded,
		AggregateID:   newDec.ID,
		AggregateType: "decision",
		Scope:         newDec.Scope,
		Payload:       nil,
		OccurredAt:    newDec.CreatedAt,
	})

	return &inbound.RecordDecisionResult{
		ID:         newDec.ID,
		Version:    newDec.Version,
		Superseded: superseded,
		CreatedAt:  newDec.CreatedAt,
	}, nil
}

// Get retrieves a decision by ID.
func (s *Service) Get(ctx context.Context, id shared.RecordID) (*decision.Decision, error) {
	return s.decRepo.FindByID(ctx, id)
}

// GetHistory retrieves all versions of a decision by key and scope.
func (s *Service) GetHistory(ctx context.Context, query inbound.DecisionHistoryQuery) ([]decision.Decision, error) {
	return s.decRepo.FindByKey(ctx, query.DecisionKey, query.Scope)
}

// Contradict marks a decision as contradicted and creates a contradicts relation.
func (s *Service) Contradict(ctx context.Context, cmd inbound.ContradictDecisionCmd) error {
	target, err := s.decRepo.FindByID(ctx, cmd.TargetID)
	if err != nil {
		return err
	}

	if err := target.Contradict(); err != nil {
		return err
	}

	if err := s.decRepo.UpdateStatus(ctx, cmd.TargetID, shared.DecisionStatusContradicted, nil); err != nil {
		return err
	}

	rel, err := relation.NewRelation(
		cmd.ContradictedBy,
		cmd.TargetID,
		shared.RelationTypeContradicts,
		target.Scope,
		s.clock,
	)
	if err != nil {
		return err
	}

	if err := s.relRepo.Save(ctx, rel); err != nil {
		return err
	}

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeDecisionContradicted,
		AggregateID:   cmd.TargetID,
		AggregateType: "decision",
		Scope:         target.Scope,
		Payload:       nil,
		OccurredAt:    s.clock.Now(),
	})

	return nil
}
