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

// GetByTopicKey retrieves the latest active memory record matching the given
// topic key within the supplied scope. Validates that ProjectID and TopicKey
// are non-empty, then delegates to the repository, which returns
// shared.ErrNotFound when no active record matches.
func (s *Service) GetByTopicKey(ctx context.Context, query inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
	var fields []shared.FieldError
	if query.ProjectID == "" {
		fields = append(fields, shared.FieldError{Field: "project_id", Message: "required"})
	}
	if query.TopicKey == "" {
		fields = append(fields, shared.FieldError{Field: "topic_key", Message: "required"})
	}
	if len(fields) > 0 {
		return nil, shared.NewValidationError(fields...)
	}

	var opts []shared.ScopeOption
	if query.TenantID != nil {
		opts = append(opts, shared.WithTenantID(*query.TenantID))
	}
	if query.RepoID != nil {
		opts = append(opts, shared.WithRepoID(*query.RepoID))
	}
	if query.AgentID != nil {
		opts = append(opts, shared.WithAgentID(*query.AgentID))
	}
	if query.SessionID != nil {
		opts = append(opts, shared.WithSessionID(*query.SessionID))
	}
	if query.Environment != nil {
		opts = append(opts, shared.WithEnvironment(*query.Environment))
	}

	scope, err := shared.NewScope(query.ProjectID, opts...)
	if err != nil {
		return nil, err
	}

	return s.memRepo.FindLatestActiveByTopicKey(ctx, scope, query.TopicKey)
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
