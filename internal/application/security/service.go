package security

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that Service implements PurgeService.
var _ inbound.PurgeService = (*Service)(nil)

// Service implements security purge operations.
type Service struct {
	purgeRepo outbound.PurgeRepository
	memRepo   outbound.MemoryRepository
	relRepo   outbound.RelationRepository
	searchIdx outbound.SearchIndex
	txMgr     outbound.TransactionManager
	eventPub  outbound.EventPublisher
	clock     shared.Clock
}

// NewService creates a new security purge service.
func NewService(
	purgeRepo outbound.PurgeRepository,
	memRepo outbound.MemoryRepository,
	relRepo outbound.RelationRepository,
	searchIdx outbound.SearchIndex,
	txMgr outbound.TransactionManager,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		purgeRepo: purgeRepo,
		memRepo:   memRepo,
		relRepo:   relRepo,
		searchIdx: searchIdx,
		txMgr:     txMgr,
		eventPub:  eventPub,
		clock:     clock,
	}
}

// Request creates a new purge request for the given target.
// Phase 1: only memory targets are supported.
func (s *Service) Request(ctx context.Context, cmd inbound.RequestPurgeCmd) (*purge.PurgeRecord, error) {
	// Phase 1: look up the target in the memory repo to determine type and scope.
	mem, err := s.memRepo.FindByID(ctx, cmd.TargetID)
	if err != nil {
		return nil, err
	}

	record, err := purge.NewPurgeRecord(
		cmd.TargetID,
		"memory",
		cmd.Reason,
		cmd.RequestedBy,
		cmd.AuditNote,
		mem.Scope,
		s.clock,
	)
	if err != nil {
		return nil, err
	}

	if err := s.purgeRepo.Save(ctx, record); err != nil {
		return nil, err
	}

	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypePurgeRequested,
		AggregateID:   record.ID,
		AggregateType: "purge",
		Scope:         record.Scope,
		Payload:       record,
		OccurredAt:    s.clock.Now(),
	})

	return record, nil
}

// Execute performs the atomic purge execution: wipes content, removes FTS, deletes relations.
func (s *Service) Execute(ctx context.Context, cmd inbound.ExecutePurgeCmd) (*purge.PurgeRecord, error) {
	record, err := s.purgeRepo.FindByID(ctx, cmd.PurgeID)
	if err != nil {
		return nil, err
	}

	if err := record.MarkExecuting(); err != nil {
		return nil, err
	}

	var artifacts purge.PurgedArtifacts

	txErr := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		if err := s.memRepo.WipeContent(txCtx, record.TargetID); err != nil {
			return err
		}

		if err := s.searchIdx.Remove(txCtx, record.TargetID); err != nil {
			return err
		}

		relCount, err := s.relRepo.DeleteByTarget(txCtx, record.TargetID)
		if err != nil {
			return err
		}

		artifacts = purge.PurgedArtifacts{
			FTSInvalidated:       true,
			RelationsRemoved:     relCount,
			DerivedInvalidated:   []shared.RecordID{},
		}

		if err := record.MarkExecuted(artifacts, s.clock); err != nil {
			return err
		}

		return s.purgeRepo.UpdateStatus(txCtx, record.ID, shared.PurgeStatusExecuted, &artifacts)
	})

	if txErr != nil {
		record.MarkFailed(txErr.Error())
		_ = s.purgeRepo.UpdateStatus(ctx, record.ID, shared.PurgeStatusFailed, nil)
		return record, txErr
	}

	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypePurgeExecuted,
		AggregateID:   record.ID,
		AggregateType: "purge",
		Scope:         record.Scope,
		Payload:       record,
		OccurredAt:    s.clock.Now(),
	})

	return record, nil
}
