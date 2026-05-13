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
//   - FindByID, UpdateStatus, WipeContent: the caller supplies the auth-derived
//     scope explicitly. The implementation MUST add
//     "AND project_id = $N AND (tenant_id IS NULL OR tenant_id = $M)"
//     to every SELECT/UPDATE/DELETE. A miss returns shared.ErrNotFound — never
//     shared.ErrScopeForbidden — to prevent existence leaks.
type MemoryRepository interface {
	// Save persists a new memory record. Scope is embedded in record.Scope.
	Save(ctx context.Context, record *memory.MemoryRecord) error

	// UpsertByTopicKey inserts the record, or — when an active row already
	// exists in the same scope (project_id + tenant_id) with the same
	// topic_key — updates that row's content + provenance + temporal fields
	// in place. The returned id is the existing row's id on an update;
	// `inserted` is true iff a new row was created. Implementations MUST rely
	// on the partial unique index introduced in migration 004 (ADR-0005 P1.3)
	// so that concurrent calls converge to exactly one active row.
	//
	// Scope assertion (P1.5) MUST be performed by the application layer
	// BEFORE this method is called.
	UpsertByTopicKey(ctx context.Context, record *memory.MemoryRecord) (id shared.RecordID, inserted bool, err error)

	// FindByID retrieves a memory record scoped to authScope.
	// Returns shared.ErrNotFound if the record does not exist or belongs to a
	// different project (cross-project reads look identical to missing rows).
	FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*memory.MemoryRecord, error)

	// FindActiveByTopicKey retrieves the unique active memory record for a
	// (scope, topic_key) tuple. Returns shared.ErrNotFound when no active row
	// matches within the supplied scope.
	FindActiveByTopicKey(ctx context.Context, scope shared.Scope, topicKey string) (*memory.MemoryRecord, error)

	// UpdateStatus transitions the status of a scoped memory record.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.MemoryStatus) error

	// WipeContent hard-purges content of a scoped memory record.
	// Returns shared.ErrNotFound if the record does not exist within scope.
	WipeContent(ctx context.Context, scope shared.Scope, id shared.RecordID) error
}
