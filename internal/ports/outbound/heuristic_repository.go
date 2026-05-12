package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// HeuristicRepository persists and retrieves heuristic rules.
//
// Scope enforcement contract:
//   - Save: scope is embedded in the record; the application layer asserts it
//     matches auth scope before calling Save.
//   - FindByID: requires explicit auth-derived scope; implementation adds
//     "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)".
//     A miss (including cross-project access) returns shared.ErrNotFound.
//   - FindActiveByKey, FindByScope: scope is the query filter; already enforced.
//   - UpdateEnabled: requires scope so the update cannot affect another project's
//     heuristic even when given a known ID.
type HeuristicRepository interface {
	// Save persists a new heuristic rule. Scope is embedded in rule.Scope.
	Save(ctx context.Context, rule *heuristic.HeuristicRule) error

	// FindByID retrieves a heuristic rule scoped to authScope.
	// Returns shared.ErrNotFound for missing or out-of-scope records.
	FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*heuristic.HeuristicRule, error)

	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error)
	FindByScope(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error)

	// UpdateEnabled sets the enabled flag on a scoped heuristic rule.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	UpdateEnabled(ctx context.Context, scope shared.Scope, id shared.RecordID, enabled bool) error
}
