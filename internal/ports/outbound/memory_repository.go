package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// MemoryRepository persists and retrieves memory records.
type MemoryRepository interface {
	Save(ctx context.Context, record *memory.MemoryRecord) error
	FindByID(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	// FindLatestActiveByTopicKey returns the newest active record matching the
	// given topic key within the supplied scope. The scope's ProjectID is
	// required; any optional scope fields (TenantID, RepoID, AgentID,
	// SessionID, Environment) further narrow the match. Returns
	// shared.ErrNotFound when no active record matches.
	FindLatestActiveByTopicKey(ctx context.Context, scope shared.Scope, topicKey string) (*memory.MemoryRecord, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error
	WipeContent(ctx context.Context, id shared.RecordID) error
}
