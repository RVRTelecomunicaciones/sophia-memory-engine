package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// IngestMemoryCmd carries the data needed to ingest a new memory record.
type IngestMemoryCmd struct {
	Type        shared.MemoryType
	Content     string
	Summary     *string
	Tags        []string
	TopicKey    *string
	FTSLanguage *string
	Scope       shared.Scope
	Provenance  shared.Provenance
	ValidFrom   *time.Time
	ValidUntil  *time.Time
}

// IngestMemoryResult is the outcome of a successful memory ingestion.
type IngestMemoryResult struct {
	ID        shared.RecordID
	CreatedAt time.Time
}

// ArchiveMemoryCmd carries the data needed to archive an existing memory.
type ArchiveMemoryCmd struct {
	ID          shared.RecordID
	Reason      string
	RequestedBy string
}

// MemoryService defines the inbound port for memory lifecycle operations.
type MemoryService interface {
	Ingest(ctx context.Context, cmd IngestMemoryCmd) (*IngestMemoryResult, error)
	Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	Archive(ctx context.Context, cmd ArchiveMemoryCmd) error
}
