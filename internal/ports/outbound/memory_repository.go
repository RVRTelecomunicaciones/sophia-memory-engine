package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// MemoryRepository persists and retrieves memory records.
//
// Scope enforcement contract:
//   - Save: scope comes from record.Scope; the application layer MUST assert
//     record.Scope matches the auth context before calling Save.
//   - FindByID, FindLatestActiveByTopicKey, UpdateStatus, WipeContent: the
//     caller supplies the auth-derived scope explicitly. The implementation
//     MUST add
//     "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)"
//     to every SELECT/UPDATE/DELETE. A miss returns shared.ErrNotFound — never
//     shared.ErrScopeForbidden — to prevent existence leaks.
type MemoryRepository interface {
	// Save persists a new memory record. Scope is embedded in record.Scope.
	Save(ctx context.Context, record *memory.MemoryRecord) error

	// FindByID retrieves a memory record scoped to authScope.
	// Returns shared.ErrNotFound if the record does not exist or belongs to a
	// different project (cross-project reads look identical to missing rows).
	FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*memory.MemoryRecord, error)

	// FindLatestActiveByTopicKey returns the newest active record matching the
	// given topic key within the supplied scope. The scope's ProjectID is
	// required; any optional scope fields (TenantID, RepoID, AgentID,
	// SessionID, Environment) further narrow the match. Returns
	// shared.ErrNotFound when no active record matches.
	FindLatestActiveByTopicKey(ctx context.Context, scope shared.Scope, topicKey string) (*memory.MemoryRecord, error)

	// UpdateStatus transitions the status of a scoped memory record.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.MemoryStatus) error

	// WipeContent hard-purges content of a scoped memory record.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	WipeContent(ctx context.Context, scope shared.Scope, id shared.RecordID) error
}
