package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// TraverseDirection indicates which edge direction to follow during graph traversal.
type TraverseDirection string

const (
	TraverseOutbound TraverseDirection = "outbound"
	TraverseInbound  TraverseDirection = "inbound"
	TraverseBoth     TraverseDirection = "both"
)

// TraverseQuery defines the parameters for a graph traversal from a starting node.
type TraverseQuery struct {
	StartID         shared.RecordID
	Direction       TraverseDirection
	MaxDepth        int
	Types           []shared.RelationType
	Scope           *shared.Scope
	ValidAt         *time.Time
	ExcludeStatuses []string
}

// TraverseResult represents a single relation found during traversal, with depth and path info.
type TraverseResult struct {
	Relation relation.Relation
	Depth    int
	Path     []shared.RecordID
}

// RelationRepository persists and retrieves typed directed relations between entities.
type RelationRepository interface {
	Save(ctx context.Context, rel *relation.Relation) error
	FindFromSource(ctx context.Context, sourceID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)
	FindToTarget(ctx context.Context, targetID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)
	Traverse(ctx context.Context, query TraverseQuery) ([]TraverseResult, error)
	DeleteByTarget(ctx context.Context, targetID shared.RecordID) (int, error)
}
