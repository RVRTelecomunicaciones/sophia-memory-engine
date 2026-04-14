package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// DecisionRepository persists and retrieves decision records.
type DecisionRepository interface {
	Save(ctx context.Context, d *decision.Decision) error
	FindByID(ctx context.Context, id shared.RecordID) (*decision.Decision, error)
	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*decision.Decision, error)
	FindByKey(ctx context.Context, key string, scope shared.Scope) ([]decision.Decision, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.DecisionStatus, supersededBy *shared.RecordID) error
}
