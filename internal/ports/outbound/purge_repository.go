package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// PurgeRepository persists and retrieves hard-purge audit records.
//
// Scope enforcement contract:
//   - Save: scope is embedded in record.Scope; the application layer asserts it
//     matches the auth context before calling Save.
//   - FindByID: requires explicit auth-derived scope; implementation adds
//     "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)".
//     A miss (including cross-project access) returns shared.ErrNotFound.
//   - UpdateStatus: requires scope so status updates cannot touch another
//     project's purge record even when given a known ID.
type PurgeRepository interface {
	// Save persists a new PurgeRecord. Scope is embedded in record.Scope.
	Save(ctx context.Context, record *purge.PurgeRecord) error

	// FindByID retrieves a PurgeRecord scoped to authScope.
	// Returns shared.ErrNotFound for missing or out-of-scope records.
	FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*purge.PurgeRecord, error)

	// UpdateStatus changes status and artifacts of a scoped purge record.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error
}
