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
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error
	WipeContent(ctx context.Context, id shared.RecordID) error
}
