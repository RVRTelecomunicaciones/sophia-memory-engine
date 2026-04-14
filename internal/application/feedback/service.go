package feedback

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

var _ inbound.FeedbackService = (*Service)(nil)

// Service implements inbound.FeedbackService for retrieval feedback telemetry.
type Service struct {
	feedbackRepo outbound.FeedbackRepository
	eventPub     outbound.EventPublisher
	clock        shared.Clock
}

// NewService creates a new feedback Service with the given dependencies.
func NewService(
	feedbackRepo outbound.FeedbackRepository,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		feedbackRepo: feedbackRepo,
		eventPub:     eventPub,
		clock:        clock,
	}
}

// Submit validates and persists retrieval feedback, then publishes a domain event.
func (s *Service) Submit(ctx context.Context, cmd inbound.SubmitFeedbackCmd) (*shared.RetrievalFeedback, error) {
	fb, err := shared.NewRetrievalFeedback(
		cmd.Target,
		cmd.Type,
		cmd.Query,
		cmd.Scope,
		cmd.ResultIDs,
		cmd.SelectedIDs,
		cmd.Actor,
		s.clock,
	)
	if err != nil {
		return nil, err
	}

	if cmd.Metadata != nil {
		fb.Metadata = cmd.Metadata
	}

	if err := s.feedbackRepo.Save(ctx, fb); err != nil {
		return nil, err
	}

	eventType := resolveEventType(cmd.Target, cmd.Type)

	// At-most-once event publishing — ignore errors.
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		Type:          eventType,
		AggregateID:   fb.ID,
		AggregateType: "retrieval_feedback",
		Scope:         fb.Scope,
		Payload:       nil,
		OccurredAt:    fb.CreatedAt,
	})

	return fb, nil
}

// resolveEventType maps target + feedback type to the corresponding domain event type.
func resolveEventType(target shared.FeedbackTarget, feedbackType shared.FeedbackType) shared.EventType {
	switch {
	case target == shared.FeedbackTargetMemory && feedbackType == shared.FeedbackTypeHelpful:
		return shared.EventTypeMemoryHelpful
	case target == shared.FeedbackTargetMemory && feedbackType == shared.FeedbackTypeIrrelevant:
		return shared.EventTypeMemoryIrrelevant
	case target == shared.FeedbackTargetContext && feedbackType == shared.FeedbackTypeHelpful:
		return shared.EventTypeContextHelpful
	case target == shared.FeedbackTargetContext && feedbackType == shared.FeedbackTypeIrrelevant:
		return shared.EventTypeContextIrrelevant
	default:
		return shared.EventTypeContextIrrelevant // unreachable after validation
	}
}
