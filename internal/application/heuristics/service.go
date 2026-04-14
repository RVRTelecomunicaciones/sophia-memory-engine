package heuristics

import (
	"context"
	"errors"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that Service implements HeuristicService.
var _ inbound.HeuristicService = (*Service)(nil)

// Service implements inbound.HeuristicService for heuristic lifecycle operations.
type Service struct {
	heurRepo outbound.HeuristicRepository
	txMgr    outbound.TransactionManager
	eventPub outbound.EventPublisher
	clock    shared.Clock
}

// NewService creates a new heuristics Service with the given dependencies.
func NewService(
	heurRepo outbound.HeuristicRepository,
	txMgr outbound.TransactionManager,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		heurRepo: heurRepo,
		txMgr:    txMgr,
		eventPub: eventPub,
		clock:    clock,
	}
}

// Create creates a new heuristic rule, optionally disabling the previous active version.
func (s *Service) Create(ctx context.Context, cmd inbound.CreateHeuristicCmd) (*inbound.CreateHeuristicResult, error) {
	// Determine enabled: if cmd.Enabled != nil use it, else true.
	enabled := true
	if cmd.Enabled != nil {
		enabled = *cmd.Enabled
	}

	// Build options from cmd.
	var opts []heuristic.Option
	if cmd.ValidUntil != nil {
		opts = append(opts, heuristic.WithValidUntil(*cmd.ValidUntil))
	}
	if cmd.FTSLanguage != nil {
		opts = append(opts, heuristic.WithFTSLanguage(*cmd.FTSLanguage))
	}
	opts = append(opts, heuristic.WithEnabled(enabled))

	// Find existing active rule for this key+scope.
	existing, err := s.heurRepo.FindActiveByKey(ctx, cmd.HeuristicKey, cmd.Scope)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}

	// Determine version.
	version := 1
	if existing != nil {
		version = existing.Version + 1
	}

	// Create the new heuristic rule.
	rule, err := heuristic.NewHeuristicRule(
		cmd.HeuristicKey, cmd.Condition, cmd.Action, cmd.Rationale,
		cmd.Scope, cmd.Provenance, cmd.Confidence,
		s.clock, version, opts...,
	)
	if err != nil {
		return nil, err
	}

	var disabledPrevious *shared.RecordID

	// If existing active AND new is enabled: wrap in transaction.
	if existing != nil && enabled {
		disabledPrevious = &existing.ID
		err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
			if err := s.heurRepo.UpdateEnabled(txCtx, existing.ID, false); err != nil {
				return err
			}
			return s.heurRepo.Save(txCtx, rule)
		})
		if err != nil {
			return nil, err
		}
	} else {
		// No existing or new is disabled: just save.
		if err := s.heurRepo.Save(ctx, rule); err != nil {
			return nil, err
		}
	}

	// Publish event (at-most-once — ignore errors).
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeHeuristicCreated,
		AggregateID:   rule.ID,
		AggregateType: "heuristic",
		Scope:         rule.Scope,
		Payload:       nil,
		OccurredAt:    rule.CreatedAt,
	})

	return &inbound.CreateHeuristicResult{
		ID:               rule.ID,
		Version:          rule.Version,
		DisabledPrevious: disabledPrevious,
		CreatedAt:        rule.CreatedAt,
	}, nil
}

// GetActive retrieves the current active heuristic rule by key and scope.
func (s *Service) GetActive(ctx context.Context, query inbound.GetActiveHeuristicQuery) (*heuristic.HeuristicRule, error) {
	return s.heurRepo.FindActiveByKey(ctx, query.HeuristicKey, query.Scope)
}

// ListByScope lists heuristic rules matching the given scope and optional enabled filter.
func (s *Service) ListByScope(ctx context.Context, query inbound.ListHeuristicsQuery) ([]heuristic.HeuristicRule, error) {
	return s.heurRepo.FindByScope(ctx, query.Scope, query.Enabled)
}

// Toggle enables or disables a heuristic rule.
// When enabling, it checks for conflicts with an existing active rule for the same key+scope.
func (s *Service) Toggle(ctx context.Context, cmd inbound.ToggleHeuristicCmd) error {
	if cmd.Enabled {
		// Find the heuristic by ID to get its key+scope.
		rule, err := s.heurRepo.FindByID(ctx, cmd.ID)
		if err != nil {
			return err
		}

		// Find current active by key+scope.
		active, err := s.heurRepo.FindActiveByKey(ctx, rule.HeuristicKey, rule.Scope)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}

		// If active exists and it's a different ID → conflict.
		if active != nil && !active.ID.Equal(cmd.ID) {
			return shared.ErrConflict
		}
	}

	if err := s.heurRepo.UpdateEnabled(ctx, cmd.ID, cmd.Enabled); err != nil {
		return err
	}

	// Publish toggle event (at-most-once — ignore errors).
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          shared.EventTypeHeuristicToggled,
		AggregateID:   cmd.ID,
		AggregateType: "heuristic",
		Payload:       map[string]bool{"enabled": cmd.Enabled},
		OccurredAt:    s.clock.Now(),
	})

	return nil
}
