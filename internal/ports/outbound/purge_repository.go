package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// PurgeRepository persists and retrieves hard-purge audit records.
type PurgeRepository interface {
	Save(ctx context.Context, record *purge.PurgeRecord) error
	FindByID(ctx context.Context, id shared.RecordID) (*purge.PurgeRecord, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error
}
