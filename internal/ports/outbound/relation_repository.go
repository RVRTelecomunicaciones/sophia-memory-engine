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
	StartID          shared.RecordID
	Direction        TraverseDirection
	MaxDepth         int
	MaxFanoutPerNode int // max relations per source node in results (0 = no limit)
	MaxTotalResults  int // hard cap on total results returned (0 = default 100)
	Types            []shared.RelationType
	Scope            *shared.Scope
	ValidAt          *time.Time
	ExcludeStatuses  []string
}

// TraverseResult represents a single relation found during traversal, with depth and path info.
type TraverseResult struct {
	Relation relation.Relation
	Depth    int
	Path     []shared.RecordID
}

// RelationRepository persists and retrieves typed directed relations between entities.
//
// Scope enforcement contract:
//   - Save: scope is embedded in rel.Scope; application layer asserts it matches
//     auth scope before calling Save.
//   - FindFromSource, FindToTarget: require explicit auth-derived scope so the SQL
//     layer adds "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)".
//   - Traverse: scope is passed via TraverseQuery.Scope and enforced in SQL.
//   - DeleteByTarget: requires scope so a purge of project-A's record cannot
//     accidentally delete project-B's relations even when given a known ID.
type RelationRepository interface {
	// Save persists a new relation. Scope is embedded in rel.Scope.
	Save(ctx context.Context, rel *relation.Relation) error

	// FindFromSource returns all relations from sourceID within authScope.
	FindFromSource(ctx context.Context, scope shared.Scope, sourceID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)

	// FindToTarget returns all relations pointing to targetID within authScope.
	FindToTarget(ctx context.Context, scope shared.Scope, targetID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)

	// Traverse performs a recursive graph traversal. Scope is carried in query.Scope.
	Traverse(ctx context.Context, query TraverseQuery) ([]TraverseResult, error)

	// DeleteByTarget removes all relations for targetID within authScope.
	// Purge flows use the auth context's scope (the scope of the purge record),
	// regardless of the target's own scope, to prevent cross-project deletion.
	DeleteByTarget(ctx context.Context, scope shared.Scope, targetID shared.RecordID) (int, error)
}
