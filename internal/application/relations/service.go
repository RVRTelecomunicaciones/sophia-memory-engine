package relations

import (
	"context"

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

// Create validates and persists a new relation, then publishes a domain event.
func (s *Service) Create(ctx context.Context, cmd inbound.CreateRelationCmd) (*inbound.CreateRelationResult, error) {
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

	tq := outbound.TraverseQuery{
		StartID:   query.RecordID,
		Direction: direction,
		MaxDepth:  maxDepth,
		Scope:     query.Scope,
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
