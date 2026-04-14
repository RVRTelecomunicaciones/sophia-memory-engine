package ingest

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

var _ inbound.MemoryService = (*Service)(nil)

// Service implements inbound.MemoryService for memory lifecycle operations.
type Service struct {
	memRepo   outbound.MemoryRepository
	searchIdx outbound.SearchIndex
	eventPub  outbound.EventPublisher
	clock     shared.Clock
}

// NewService creates a new ingest Service with the given dependencies.
func NewService(
	memRepo outbound.MemoryRepository,
	searchIdx outbound.SearchIndex,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		memRepo:   memRepo,
		searchIdx: searchIdx,
		eventPub:  eventPub,
		clock:     clock,
	}
}

// Ingest creates a new memory record, persists it, indexes it for search,
// and publishes a domain event (at-most-once, event failures are ignored).
func (s *Service) Ingest(ctx context.Context, cmd inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error) {
	var opts []memory.Option

	if cmd.ValidFrom != nil {
		opts = append(opts, memory.WithValidFrom(*cmd.ValidFrom))
	}
	if cmd.ValidUntil != nil {
		opts = append(opts, memory.WithValidUntil(*cmd.ValidUntil))
	}
	if cmd.Summary != nil {
		opts = append(opts, memory.WithSummary(*cmd.Summary))
	}
	if len(cmd.Tags) > 0 {
		opts = append(opts, memory.WithTags(cmd.Tags))
	}
	if cmd.TopicKey != nil {
		opts = append(opts, memory.WithTopicKey(*cmd.TopicKey))
	}
	if cmd.FTSLanguage != nil {
		opts = append(opts, memory.WithFTSLanguage(*cmd.FTSLanguage))
	}

	record, err := memory.NewMemoryRecord(cmd.Type, cmd.Content, cmd.Scope, cmd.Provenance, s.clock, opts...)
	if err != nil {
		return nil, err
	}

	if err := s.memRepo.Save(ctx, record); err != nil {
		return nil, err
	}

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeMemoryIngested,
		AggregateID:   record.ID,
		AggregateType: "memory",
		Scope:         record.Scope,
		Payload:       nil,
		OccurredAt:    record.CreatedAt,
	})

	return &inbound.IngestMemoryResult{
		ID:        record.ID,
		CreatedAt: record.CreatedAt,
	}, nil
}

// Get retrieves a memory record by ID. The repository handles ErrNotFound/ErrPurged.
func (s *Service) Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	return s.memRepo.FindByID(ctx, id)
}

// Archive transitions a memory record to archived status.
func (s *Service) Archive(ctx context.Context, cmd inbound.ArchiveMemoryCmd) error {
	record, err := s.memRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	if err := record.Archive(cmd.RequestedBy, cmd.Reason); err != nil {
		return err
	}

	return s.memRepo.UpdateStatus(ctx, cmd.ID, record.Status)
}
