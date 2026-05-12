package relations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// --- Mocks ---

type mockRelationRepo struct {
	saved       []*relation.Relation
	saveErr     error
	traverseRes []outbound.TraverseResult
	traverseErr error
	traverseReq []outbound.TraverseQuery
	deleteCount int
	deleteErr   error
}

func (m *mockRelationRepo) Save(_ context.Context, rel *relation.Relation) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, rel)
	return nil
}

func (m *mockRelationRepo) FindFromSource(_ context.Context, _ shared.Scope, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}

func (m *mockRelationRepo) FindToTarget(_ context.Context, _ shared.Scope, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}

func (m *mockRelationRepo) Traverse(_ context.Context, q outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	m.traverseReq = append(m.traverseReq, q)
	return m.traverseRes, m.traverseErr
}

func (m *mockRelationRepo) DeleteByTarget(_ context.Context, _ shared.Scope, _ shared.RecordID) (int, error) {
	return m.deleteCount, m.deleteErr
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

func testCmd(t *testing.T) inbound.CreateRelationCmd {
	t.Helper()
	return inbound.CreateRelationCmd{
		SourceID: shared.NewRecordID(),
		TargetID: shared.NewRecordID(),
		Type:     shared.RelationTypeRelatesTo,
		Metadata: map[string]any{"reason": "test"},
		Scope:    testScope(t),
	}
}

// --- Tests ---

func TestRelationService_Create_Success(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	cmd := testCmd(t)
	result, err := svc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID.IsZero() {
		t.Error("expected non-zero ID")
	}
	if !result.CreatedAt.Equal(clock.FixedTime) {
		t.Errorf("expected created_at %v, got %v", clock.FixedTime, result.CreatedAt)
	}

	// Relation saved.
	if len(relRepo.saved) != 1 {
		t.Fatalf("expected 1 saved relation, got %d", len(relRepo.saved))
	}
	saved := relRepo.saved[0]
	if !saved.SourceID.Equal(cmd.SourceID) {
		t.Error("source_id mismatch")
	}
	if !saved.TargetID.Equal(cmd.TargetID) {
		t.Error("target_id mismatch")
	}
	if saved.Type != shared.RelationTypeRelatesTo {
		t.Errorf("expected type relates_to, got %s", saved.Type)
	}
	if saved.Metadata["reason"] != "test" {
		t.Error("metadata not applied")
	}

	// Event published.
	if len(eventPub.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventPub.published))
	}
	if eventPub.published[0].Type != shared.EventTypeRelationCreated {
		t.Errorf("unexpected event type: %s", eventPub.published[0].Type)
	}
}

func TestRelationService_Create_SelfReference(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	id := shared.NewRecordID()
	cmd := inbound.CreateRelationCmd{
		SourceID: id,
		TargetID: id,
		Type:     shared.RelationTypeRelatesTo,
		Scope:    testScope(t),
	}

	_, err := svc.Create(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected validation error for self-reference, got nil")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("expected ErrValidation, got: %v", err)
	}

	// Nothing saved.
	if len(relRepo.saved) != 0 {
		t.Errorf("expected no saved relations, got %d", len(relRepo.saved))
	}
}

func TestRelationService_Create_WithValidFrom(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	validFrom := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	cmd := testCmd(t)
	cmd.ValidFrom = &validFrom

	result, err := svc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID.IsZero() {
		t.Error("expected non-zero ID")
	}

	saved := relRepo.saved[0]
	if saved.Temporal.ValidFrom == nil || !saved.Temporal.ValidFrom.Equal(validFrom) {
		t.Errorf("expected valid_from %v, got %v", validFrom, saved.Temporal.ValidFrom)
	}
}

func TestRelationService_GetFrom_DefaultDepth(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	startID := shared.NewRecordID()
	targetID := shared.NewRecordID()
	scope := testScope(t)

	traverseResult := outbound.TraverseResult{
		Relation: relation.Relation{
			ID:       shared.NewRecordID(),
			SourceID: startID,
			TargetID: targetID,
			Type:     shared.RelationTypeRelatesTo,
			Scope:    scope,
		},
		Depth: 1,
		Path:  []shared.RecordID{startID, targetID},
	}

	relRepo := &mockRelationRepo{traverseRes: []outbound.TraverseResult{traverseResult}}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	results, err := svc.GetFrom(context.Background(), inbound.RelationQuery{
		RecordID: startID,
		Scope:    &scope,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Depth != 1 {
		t.Errorf("expected depth 1, got %d", results[0].Depth)
	}

	// Verify the traverse query used default depth 1.
	if len(relRepo.traverseReq) != 1 {
		t.Fatalf("expected 1 traverse call, got %d", len(relRepo.traverseReq))
	}
	if relRepo.traverseReq[0].MaxDepth != 1 {
		t.Errorf("expected default max_depth 1, got %d", relRepo.traverseReq[0].MaxDepth)
	}
	if relRepo.traverseReq[0].Direction != outbound.TraverseOutbound {
		t.Errorf("expected outbound direction, got %s", relRepo.traverseReq[0].Direction)
	}
}

func TestRelationService_GetFrom_CapsMaxDepth(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	scope := testScope(t)
	depth := 5
	_, err := svc.GetFrom(context.Background(), inbound.RelationQuery{
		RecordID: shared.NewRecordID(),
		MaxDepth: &depth,
		Scope:    &scope,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify depth was capped at 3.
	if len(relRepo.traverseReq) != 1 {
		t.Fatalf("expected 1 traverse call, got %d", len(relRepo.traverseReq))
	}
	if relRepo.traverseReq[0].MaxDepth != 3 {
		t.Errorf("expected capped max_depth 3, got %d", relRepo.traverseReq[0].MaxDepth)
	}
}

func TestRelationService_GetTo(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	sourceID := shared.NewRecordID()
	targetID := shared.NewRecordID()
	scope := testScope(t)

	traverseResult := outbound.TraverseResult{
		Relation: relation.Relation{
			ID:       shared.NewRecordID(),
			SourceID: sourceID,
			TargetID: targetID,
			Type:     shared.RelationTypeDependsOn,
			Scope:    scope,
		},
		Depth: 1,
		Path:  []shared.RecordID{targetID, sourceID},
	}

	relRepo := &mockRelationRepo{traverseRes: []outbound.TraverseResult{traverseResult}}
	eventPub := &mockEventPublisher{}

	svc := NewService(relRepo, eventPub, clock)

	relType := shared.RelationTypeDependsOn
	results, err := svc.GetTo(context.Background(), inbound.RelationQuery{
		RecordID: targetID,
		Type:     &relType,
		Scope:    &scope,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Relation.Type != shared.RelationTypeDependsOn {
		t.Errorf("expected depends_on type, got %s", results[0].Relation.Type)
	}

	// Verify direction is inbound.
	if relRepo.traverseReq[0].Direction != outbound.TraverseInbound {
		t.Errorf("expected inbound direction, got %s", relRepo.traverseReq[0].Direction)
	}

	// Verify type filter was passed.
	if len(relRepo.traverseReq[0].Types) != 1 || relRepo.traverseReq[0].Types[0] != shared.RelationTypeDependsOn {
		t.Errorf("expected type filter depends_on, got %v", relRepo.traverseReq[0].Types)
	}
}
