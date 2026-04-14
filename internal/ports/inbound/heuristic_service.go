package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// CreateHeuristicCmd carries the data needed to create a new heuristic rule.
type CreateHeuristicCmd struct {
	HeuristicKey string
	Condition    string
	Action       string
	Rationale    string
	FTSLanguage  *string
	Scope        shared.Scope
	Provenance   shared.Provenance
	Confidence   shared.Confidence
	Enabled      *bool
	ValidUntil   *time.Time
}

// CreateHeuristicResult is the outcome of a successful heuristic creation.
type CreateHeuristicResult struct {
	ID               shared.RecordID
	Version          int
	DisabledPrevious *shared.RecordID
	CreatedAt        time.Time
}

// GetActiveHeuristicQuery defines the filters for retrieving the active heuristic by key.
type GetActiveHeuristicQuery struct {
	HeuristicKey string
	Scope        shared.Scope
}

// ListHeuristicsQuery defines the filters for listing heuristic rules.
type ListHeuristicsQuery struct {
	Scope   shared.Scope
	Enabled *bool
}

// ToggleHeuristicCmd carries the data needed to enable or disable a heuristic.
type ToggleHeuristicCmd struct {
	ID      shared.RecordID
	Enabled bool
}

// HeuristicService defines the inbound port for heuristic lifecycle operations.
type HeuristicService interface {
	Create(ctx context.Context, cmd CreateHeuristicCmd) (*CreateHeuristicResult, error)
	GetActive(ctx context.Context, query GetActiveHeuristicQuery) (*heuristic.HeuristicRule, error)
	ListByScope(ctx context.Context, query ListHeuristicsQuery) ([]heuristic.HeuristicRule, error)
	Toggle(ctx context.Context, cmd ToggleHeuristicCmd) error
}
