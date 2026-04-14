package feedback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// --- Mocks ---

type mockFeedbackRepo struct {
	saved   []*shared.RetrievalFeedback
	saveErr error
}

func (m *mockFeedbackRepo) Save(_ context.Context, fb *shared.RetrievalFeedback) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, fb)
	return nil
}

type mockEventPublisher struct {
	published []outbound.DomainEvent
}

func (m *mockEventPublisher) Publish(_ context.Context, event outbound.DomainEvent) error {
	m.published = append(m.published, event)
	return nil
}

// --- Helpers ---

func testScope(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("test-project")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testCmd(t *testing.T) inbound.SubmitFeedbackCmd {
	t.Helper()
	return inbound.SubmitFeedbackCmd{
		Target:      shared.FeedbackTargetMemory,
		Type:        shared.FeedbackTypeHelpful,
		Query:       "JWT authentication",
		Scope:       testScope(t),
		ResultIDs:   []shared.RecordID{shared.NewRecordID(), shared.NewRecordID()},
		SelectedIDs: []shared.RecordID{shared.NewRecordID()},
		Actor:       "agent:coder-1",
	}
}

// --- Tests ---

func TestFeedbackService_Submit_MemoryHelpful(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	repo := &mockFeedbackRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(repo, eventPub, clock)

	cmd := testCmd(t)
	result, err := svc.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID.IsZero() {
		t.Error("expected non-zero ID")
	}
	if result.Target != shared.FeedbackTargetMemory {
		t.Errorf("expected target memory, got %s", result.Target)
	}
	if result.FeedbackType != shared.FeedbackTypeHelpful {
		t.Errorf("expected type helpful, got %s", result.FeedbackType)
	}
	if result.Actor != "agent:coder-1" {
		t.Errorf("expected actor agent:coder-1, got %s", result.Actor)
	}
	if !result.CreatedAt.Equal(clock.FixedTime) {
		t.Errorf("expected created_at %v, got %v", clock.FixedTime, result.CreatedAt)
	}

	// Feedback saved.
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved feedback, got %d", len(repo.saved))
	}

	// Event published with correct type.
	if len(eventPub.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventPub.published))
	}
	if eventPub.published[0].Type != shared.EventTypeMemoryHelpful {
		t.Errorf("expected event type memory.helpful, got %s", eventPub.published[0].Type)
	}
	if eventPub.published[0].AggregateType != "retrieval_feedback" {
		t.Errorf("expected aggregate type retrieval_feedback, got %s", eventPub.published[0].AggregateType)
	}
}

func TestFeedbackService_Submit_ContextIrrelevant(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	repo := &mockFeedbackRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(repo, eventPub, clock)

	cmd := testCmd(t)
	cmd.Target = shared.FeedbackTargetContext
	cmd.Type = shared.FeedbackTypeIrrelevant

	result, err := svc.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Target != shared.FeedbackTargetContext {
		t.Errorf("expected target context, got %s", result.Target)
	}
	if result.FeedbackType != shared.FeedbackTypeIrrelevant {
		t.Errorf("expected type irrelevant, got %s", result.FeedbackType)
	}

	// Event published with correct type.
	if len(eventPub.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventPub.published))
	}
	if eventPub.published[0].Type != shared.EventTypeContextIrrelevant {
		t.Errorf("expected event type context.irrelevant, got %s", eventPub.published[0].Type)
	}
}

func TestFeedbackService_Submit_InvalidTarget(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	repo := &mockFeedbackRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(repo, eventPub, clock)

	cmd := testCmd(t)
	cmd.Target = shared.FeedbackTarget("invalid")

	_, err := svc.Submit(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected validation error for invalid target, got nil")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("expected ErrValidation, got: %v", err)
	}

	// Nothing saved.
	if len(repo.saved) != 0 {
		t.Errorf("expected no saved feedback, got %d", len(repo.saved))
	}

	// No events published.
	if len(eventPub.published) != 0 {
		t.Errorf("expected no events, got %d", len(eventPub.published))
	}
}

func TestFeedbackService_Submit_EmptyActor(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	repo := &mockFeedbackRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(repo, eventPub, clock)

	cmd := testCmd(t)
	cmd.Actor = ""

	_, err := svc.Submit(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected validation error for empty actor, got nil")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("expected ErrValidation, got: %v", err)
	}

	// Nothing saved.
	if len(repo.saved) != 0 {
		t.Errorf("expected no saved feedback, got %d", len(repo.saved))
	}
}
