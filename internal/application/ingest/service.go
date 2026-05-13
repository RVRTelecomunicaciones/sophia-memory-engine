package ingest

import (
	"context"
	"fmt"
	"log/slog"

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

	// Branch on topic_key presence:
	//   - no topic_key → always INSERT a new row (legacy semantics).
	//   - topic_key set → UPSERT against the partial unique index
	//     idx_memories_topic_key_active_unique. Concurrent ingests with the
	//     same (scope, topic_key) converge to exactly ONE active row
	//     (ADR-0005 P1.3).
	//
	// Scope assertion (P1.5) above runs for BOTH branches, so an upsert
	// cannot mutate a row in another project's scope.
	finalID := record.ID
	if record.TopicKey != nil && *record.TopicKey != "" {
		id, inserted, upErr := s.memRepo.UpsertByTopicKey(ctx, record)
		if upErr != nil {
			return nil, fmt.Errorf("ingest: upsert by topic_key: %w", upErr)
		}
		finalID = id
		slog.DebugContext(ctx, "ingest upsert by topic_key",
			slog.String("topic_key", *record.TopicKey),
			slog.String("project_id", record.Scope.ProjectID),
			slog.String("id", id.String()),
			slog.Bool("inserted", inserted),
		)
	} else {
		if err := s.memRepo.Save(ctx, record); err != nil {
			return nil, fmt.Errorf("ingest: save: %w", err)
		}
	}

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeMemoryIngested,
		AggregateID:   finalID,
		AggregateType: "memory",
		Scope:         record.Scope,
		Payload:       nil,
		OccurredAt:    record.CreatedAt,
	})

	return &inbound.IngestMemoryResult{
		ID:        finalID,
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
	rec, err := s.memRepo.FindByID(ctx, scope, id)
	if err != nil {
		return nil, fmt.Errorf("ingest: get: %w", err)
	}
	return rec, nil
}

// GetByTopicKey retrieves the unique active memory record for the given
// project_id + topic_key. The auth scope (when present) MUST match the
// requested project_id or the repo lookup will return ErrNotFound (existence
// leak prevention — see ADR-0005 §P1.5).
//
// projectID is required because topic_key uniqueness is scoped per project.
func (s *Service) GetByTopicKey(ctx context.Context, projectID, topicKey string) (*memory.MemoryRecord, error) {
	if projectID == "" {
		return nil, shared.NewValidationError(shared.FieldError{
			Field:   "project_id",
			Message: "required",
		})
	}
	if topicKey == "" {
		return nil, shared.NewValidationError(shared.FieldError{
			Field:   "topic_key",
			Message: "required",
		})
	}

	scope := scopeFromCtxOrEmpty(ctx)
	// If auth is present, the requested project must match. Otherwise build
	// a scope from the requested project_id (internal flows / tests with no
	// auth context).
	if _, ok := authScope(ctx); ok {
		if scope.ProjectID != projectID {
			// Cross-project lookup masquerades as not-found.
			return nil, shared.ErrNotFound
		}
	} else {
		scope = shared.Scope{ProjectID: projectID}
	}

	rec, err := s.memRepo.FindActiveByTopicKey(ctx, scope, topicKey)
	if err != nil {
		return nil, fmt.Errorf("ingest: get_by_topic_key: %w", err)
	}
	return rec, nil
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
