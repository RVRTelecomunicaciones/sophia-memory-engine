package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// DecisionRepository persists and retrieves decision records.
//
// Scope enforcement contract:
//   - Save: scope is embedded in the record; the application layer asserts it
//     matches auth scope before calling Save.
//   - FindByID: requires explicit auth-derived scope so the SQL layer can add
//     "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)".
//     A miss (including cross-project access) returns shared.ErrNotFound.
//   - FindActiveByKey, FindByKey, FindActiveByScope: already carry a scope
//     parameter used as a query filter; the implementation enforces it in SQL.
//   - UpdateStatus: requires scope so updates cannot affect records owned by
//     another project.
type DecisionRepository interface {
	// Save persists a new decision. Scope is embedded in d.Scope.
	Save(ctx context.Context, d *decision.Decision) error

	// FindByID retrieves a decision scoped to authScope.
	// Returns shared.ErrNotFound for missing or out-of-scope records.
	FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*decision.Decision, error)

	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*decision.Decision, error)
	FindByKey(ctx context.Context, key string, scope shared.Scope) ([]decision.Decision, error)
	FindActiveByScope(ctx context.Context, scope shared.Scope) ([]decision.Decision, error)

	// UpdateStatus changes the status of a scoped decision.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.DecisionStatus, supersededBy *shared.RecordID) error
}
