package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// HeuristicRepository persists and retrieves heuristic rules.
type HeuristicRepository interface {
	Save(ctx context.Context, rule *heuristic.HeuristicRule) error
	FindByID(ctx context.Context, id shared.RecordID) (*heuristic.HeuristicRule, error)
	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error)
	FindByScope(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error)
	UpdateEnabled(ctx context.Context, id shared.RecordID, enabled bool) error
}
