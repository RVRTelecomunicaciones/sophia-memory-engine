package relations

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

var _ inbound.RelationService = (*Service)(nil)

// Service implements inbound.RelationService for relation graph operations.
type Service struct {
	relRepo  outbound.RelationRepository
	eventPub outbound.EventPublisher
	clock    shared.Clock
}

// NewService creates a new relation Service with the given dependencies.
func NewService(
	relRepo outbound.RelationRepository,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		relRepo:  relRepo,
		eventPub: eventPub,
		clock:    clock,
	}
}

// scopeFromCtx derives the auth-derived scope from the request context.
func scopeFromCtx(ctx context.Context) shared.Scope {
	ac, ok := auth.FromContext(ctx)
	if !ok {
		return shared.Scope{}
	}
	s := shared.Scope{ProjectID: ac.ProjectID}
	if ac.TenantID != "" {
		s.TenantID = &ac.TenantID
	}
	return s
}

// assertScopeMatch returns ErrScopeForbidden when the request scope does not
// match the authenticated scope.
func assertScopeMatch(authS shared.Scope, requestS shared.Scope) error {
	if authS.ProjectID != requestS.ProjectID {
		return shared.ErrScopeForbidden
	}
	if authS.TenantID != nil {
		if requestS.TenantID == nil || *authS.TenantID != *requestS.TenantID {
			return shared.ErrScopeForbidden
		}
	}
	return nil
}

// Create validates and persists a new relation, then publishes a domain event.
//
// Scope assertion: when an auth context is present, the request's scope MUST
// match the authenticated (project_id, tenant_id).
func (s *Service) Create(ctx context.Context, cmd inbound.CreateRelationCmd) (*inbound.CreateRelationResult, error) {
	authS := scopeFromCtx(ctx)
	if authS.ProjectID != "" {
		if err := assertScopeMatch(authS, cmd.Scope); err != nil {
			return nil, err
		}
	}

	var opts []func(*relation.Relation)

	if cmd.Metadata != nil {
		opts = append(opts, relation.WithMetadata(cmd.Metadata))
	}
	if cmd.ValidFrom != nil {
		opts = append(opts, relation.WithValidFrom(*cmd.ValidFrom))
	}

	rel, err := relation.NewRelation(
		cmd.SourceID,
		cmd.TargetID,
		cmd.Type,
		cmd.Scope,
		s.clock,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	if err := s.relRepo.Save(ctx, rel); err != nil {
		return nil, err
	}

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeRelationCreated,
		AggregateID:   rel.ID,
		AggregateType: "relation",
		Scope:         rel.Scope,
		Payload:       nil,
		OccurredAt:    rel.CreatedAt,
	})

	return &inbound.CreateRelationResult{
		ID:        rel.ID,
		CreatedAt: rel.CreatedAt,
	}, nil
}

// GetFrom traverses the relation graph outbound from the given record.
func (s *Service) GetFrom(ctx context.Context, query inbound.RelationQuery) ([]inbound.RelationResult, error) {
	return s.traverse(ctx, query, outbound.TraverseOutbound)
}

// GetTo traverses the relation graph inbound to the given record.
func (s *Service) GetTo(ctx context.Context, query inbound.RelationQuery) ([]inbound.RelationResult, error) {
	return s.traverse(ctx, query, outbound.TraverseInbound)
}

// traverse builds a TraverseQuery and delegates to the repository, then converts results.
// The auth-derived scope is merged into the TraverseQuery.Scope so the SQL CTE enforces
// project isolation at the persistence layer.
func (s *Service) traverse(ctx context.Context, query inbound.RelationQuery, direction outbound.TraverseDirection) ([]inbound.RelationResult, error) {
	maxDepth := 1
	if query.MaxDepth != nil {
		maxDepth = *query.MaxDepth
	}
	if maxDepth > 3 {
		maxDepth = 3
	}
	if maxDepth < 1 {
		maxDepth = 1
	}

	// Use the auth scope as the traversal scope when available.
	// query.Scope carries optional sub-filters (repo, agent, etc.) from the handler;
	// we override its ProjectID and TenantID with the auth-authoritative values.
	authS := scopeFromCtx(ctx)
	traversalScope := query.Scope
	if authS.ProjectID != "" {
		if traversalScope == nil {
			traversalScope = &shared.Scope{}
		}
		traversalScope.ProjectID = authS.ProjectID
		traversalScope.TenantID = authS.TenantID
	}

	tq := outbound.TraverseQuery{
		StartID:   query.RecordID,
		Direction: direction,
		MaxDepth:  maxDepth,
		Scope:     traversalScope,
		ValidAt:   query.ValidAt,
	}

	if query.Type != nil {
		tq.Types = []shared.RelationType{*query.Type}
	}

	results, err := s.relRepo.Traverse(ctx, tq)
	if err != nil {
		return nil, err
	}

	out := make([]inbound.RelationResult, 0, len(results))
	for _, r := range results {
		out = append(out, inbound.RelationResult{
			Relation: r.Relation,
			Depth:    r.Depth,
			Path:     r.Path,
		})
	}

	return out, nil
}
