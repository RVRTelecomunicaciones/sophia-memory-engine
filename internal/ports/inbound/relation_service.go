package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// CreateRelationCmd carries the data needed to create a graph relation.
type CreateRelationCmd struct {
	SourceID  shared.RecordID
	TargetID  shared.RecordID
	Type      shared.RelationType
	Metadata  map[string]any
	Scope     shared.Scope
	ValidFrom *time.Time
}

// CreateRelationResult is the outcome of a successful relation creation.
type CreateRelationResult struct {
	ID        shared.RecordID
	CreatedAt time.Time
}

// RelationQuery defines the filters for traversing the relation graph.
type RelationQuery struct {
	RecordID shared.RecordID
	Type     *shared.RelationType
	MaxDepth *int
	Scope    *shared.Scope
	ValidAt  *time.Time
}

// RelationResult is a single traversal result with depth and path information.
type RelationResult struct {
	Relation relation.Relation
	Depth    int
	Path     []shared.RecordID
}

// RelationService defines the inbound port for relation graph operations.
type RelationService interface {
	Create(ctx context.Context, cmd CreateRelationCmd) (*CreateRelationResult, error)
	GetFrom(ctx context.Context, query RelationQuery) ([]RelationResult, error)
	GetTo(ctx context.Context, query RelationQuery) ([]RelationResult, error)
}
