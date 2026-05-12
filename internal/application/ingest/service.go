package ingest

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
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

// authScope derives the authoritative scope from the request context.
// Returns the auth-derived scope and true if the context carries auth credentials.
// When no auth context is present (e.g., in tests with no middleware) the function
// returns false and the caller decides how to proceed.
func authScope(ctx context.Context) (shared.Scope, bool) {
	ac, ok := auth.FromContext(ctx)
	if !ok {
		return shared.Scope{}, false
	}
	s := shared.Scope{
		ProjectID: ac.ProjectID,
	}
	if ac.TenantID != "" {
		s.TenantID = &ac.TenantID
	}
	return s, true
}

// assertScopeMatch returns ErrScopeForbidden when the request's scope does not
// match the authenticated scope. The error body intentionally omits which field
// mismatched to prevent information leakage.
func assertScopeMatch(authS shared.Scope, requestS shared.Scope) error {
	if authS.ProjectID != requestS.ProjectID {
		return shared.ErrScopeForbidden
	}
	// TenantID: if the auth scope has a tenant, the request must match it.
	// A request with no tenant into a tenanted auth scope is also forbidden.
	if authS.TenantID != nil {
		if requestS.TenantID == nil || *authS.TenantID != *requestS.TenantID {
			return shared.ErrScopeForbidden
		}
	}
	return nil
}

// Ingest creates a new memory record, persists it, indexes it for search,
// and publishes a domain event (at-most-once, event failures are ignored).
//
// Scope assertion: when an auth context is present, the request's scope MUST
// match the authenticated (project_id, tenant_id). A mismatch returns
// ErrScopeForbidden (HTTP 403). This prevents a project-A key from inserting
// records into project B.
func (s *Service) Ingest(ctx context.Context, cmd inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error) {
	// Scope assertion: guard write operations against cross-project injection.
	if aScope, ok := authScope(ctx); ok {
		if err := assertScopeMatch(aScope, cmd.Scope); err != nil {
			return nil, err
		}
	}

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

// Get retrieves a memory record by ID within the auth scope.
// When an auth context is present the repo receives the auth-derived scope so
// the SQL WHERE clause enforces project isolation at the persistence layer.
// Cross-project reads return ErrNotFound (not ErrScopeForbidden) to prevent
// existence leaks.
func (s *Service) Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	scope := scopeFromCtxOrEmpty(ctx)
	return s.memRepo.FindByID(ctx, scope, id)
}

// Archive transitions a memory record to archived status.
// Both the fetch and the status update are scoped to the auth context, so a
// cross-project archive attempt returns ErrNotFound at the fetch step.
func (s *Service) Archive(ctx context.Context, cmd inbound.ArchiveMemoryCmd) error {
	scope := scopeFromCtxOrEmpty(ctx)

	record, err := s.memRepo.FindByID(ctx, scope, cmd.ID)
	if err != nil {
		return err
	}

	if err := record.Archive(cmd.RequestedBy, cmd.Reason); err != nil {
		return err
	}

	return s.memRepo.UpdateStatus(ctx, scope, cmd.ID, record.Status)
}

// scopeFromCtxOrEmpty returns the auth-derived scope when an auth context is
// present, or an empty Scope when not (e.g., health checks or internal workers
// that run outside of the HTTP middleware chain).
//
// An empty Scope passed to the repo means no additional WHERE filter beyond the
// primary key. This is safe for internal flows that do not originate from an
// external API call; the API middleware enforces auth before the handler fires,
// so production paths always carry a populated scope.
func scopeFromCtxOrEmpty(ctx context.Context) shared.Scope {
	s, _ := authScope(ctx)
	return s
}
