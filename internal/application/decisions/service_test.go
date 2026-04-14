package decisions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// --- Mocks ---

type mockDecisionRepo struct {
	saved             []*decision.Decision
	statusUpdates     []statusUpdate
	findByIDResult    *decision.Decision
	findByIDErr       error
	activeByKeyResult *decision.Decision
	activeByKeyErr    error
	findByKeyResult   []decision.Decision
	findByKeyErr      error
	activeByScopeResult []decision.Decision
	activeByScopeErr    error
	saveErr           error
	updateStatusErr   error
}

type statusUpdate struct {
	ID           shared.RecordID
	Status       shared.DecisionStatus
	SupersededBy *shared.RecordID
}

func (m *mockDecisionRepo) Save(_ context.Context, d *decision.Decision) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, d)
	return nil
}

func (m *mockDecisionRepo) FindByID(_ context.Context, _ shared.RecordID) (*decision.Decision, error) {
	return m.findByIDResult, m.findByIDErr
}

func (m *mockDecisionRepo) FindActiveByKey(_ context.Context, _ string, _ shared.Scope) (*decision.Decision, error) {
	return m.activeByKeyResult, m.activeByKeyErr
}

func (m *mockDecisionRepo) FindByKey(_ context.Context, _ string, _ shared.Scope) ([]decision.Decision, error) {
	return m.findByKeyResult, m.findByKeyErr
}

func (m *mockDecisionRepo) FindActiveByScope(_ context.Context, _ shared.Scope) ([]decision.Decision, error) {
	return m.activeByScopeResult, m.activeByScopeErr
}

func (m *mockDecisionRepo) UpdateStatus(_ context.Context, id shared.RecordID, status shared.DecisionStatus, supersededBy *shared.RecordID) error {
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	m.statusUpdates = append(m.statusUpdates, statusUpdate{ID: id, Status: status, SupersededBy: supersededBy})
	return nil
}

type mockRelationRepo struct {
	saved []*relation.Relation
	saveErr error
}

func (m *mockRelationRepo) Save(_ context.Context, rel *relation.Relation) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, rel)
	return nil
}

func (m *mockRelationRepo) FindFromSource(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}

func (m *mockRelationRepo) FindToTarget(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}

func (m *mockRelationRepo) Traverse(_ context.Context, _ outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	return nil, nil
}

func (m *mockRelationRepo) DeleteByTarget(_ context.Context, _ shared.RecordID) (int, error) {
	return 0, nil
}

type mockTxManager struct{}

func (m *mockTxManager) WithTx(_ context.Context, fn func(ctx context.Context) error) error {
	return fn(context.Background())
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

func testProvenance(t *testing.T) shared.Provenance {
	t.Helper()
	p, err := shared.NewProvenance("test-source", shared.IngestMethodDirect, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testConfidence(t *testing.T) shared.Confidence {
	t.Helper()
	c, err := shared.NewConfidence(0.9, shared.ConfidenceSourceHuman)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testEvidence(t *testing.T) []shared.EvidenceRef {
	t.Helper()
	uri := "https://example.com/doc"
	ref, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	return []shared.EvidenceRef{ref}
}

func testCmd(t *testing.T) inbound.RecordDecisionCmd {
	t.Helper()
	return inbound.RecordDecisionCmd{
		DecisionKey: "auth/token-strategy",
		Title:       "Use JWT with rotation",
		Description: "JWT tokens with automatic rotation for API auth",
		Rationale:   "Industry standard, well-supported libraries, stateless",
		Evidence:    testEvidence(t),
		Scope:       testScope(t),
		Provenance:  testProvenance(t),
		Confidence:  testConfidence(t),
	}
}

func newService(decRepo *mockDecisionRepo, relRepo *mockRelationRepo, eventPub *mockEventPublisher, clock shared.Clock) *Service {
	return NewService(decRepo, relRepo, &mockTxManager{}, eventPub, clock)
}

// --- Tests ---

func TestDecisionService_Record_FirstVersion(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	decRepo := &mockDecisionRepo{activeByKeyErr: shared.ErrNotFound}
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := newService(decRepo, relRepo, eventPub, clock)

	result, err := svc.Record(context.Background(), testCmd(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
	if result.Superseded != nil {
		t.Error("expected no superseded, got non-nil")
	}
	if len(decRepo.saved) != 1 {
		t.Fatalf("expected 1 saved decision, got %d", len(decRepo.saved))
	}
	if decRepo.saved[0].DecisionKey != "auth/token-strategy" {
		t.Errorf("unexpected decision key: %s", decRepo.saved[0].DecisionKey)
	}
	if decRepo.saved[0].Status != shared.DecisionStatusActive {
		t.Errorf("expected active status, got %s", decRepo.saved[0].Status)
	}
	if len(eventPub.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventPub.published))
	}
	if eventPub.published[0].Type != shared.EventTypeDecisionRecorded {
		t.Errorf("unexpected event type: %s", eventPub.published[0].Type)
	}
	// No relation should be created for first version.
	if len(relRepo.saved) != 0 {
		t.Errorf("expected no relations, got %d", len(relRepo.saved))
	}
}

func TestDecisionService_Record_SupersedesPrevious(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := testScope(t)

	existing, err := decision.NewDecision(
		"auth/token-strategy", "Old title", "Old desc", "Old rationale",
		testEvidence(t), scope, testProvenance(t), testConfidence(t), clock, 1,
	)
	if err != nil {
		t.Fatal(err)
	}

	decRepo := &mockDecisionRepo{activeByKeyResult: existing}
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := newService(decRepo, relRepo, eventPub, clock)

	result, err := svc.Record(context.Background(), testCmd(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Version incremented.
	if result.Version != 2 {
		t.Errorf("expected version 2, got %d", result.Version)
	}
	// Superseded should point to old.
	if result.Superseded == nil {
		t.Fatal("expected superseded ID, got nil")
	}
	if !result.Superseded.Equal(existing.ID) {
		t.Errorf("superseded ID mismatch")
	}

	// Old decision status updated.
	if len(decRepo.statusUpdates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(decRepo.statusUpdates))
	}
	if decRepo.statusUpdates[0].Status != shared.DecisionStatusSuperseded {
		t.Errorf("expected superseded status, got %s", decRepo.statusUpdates[0].Status)
	}

	// New decision saved.
	if len(decRepo.saved) != 1 {
		t.Fatalf("expected 1 saved decision, got %d", len(decRepo.saved))
	}

	// Supersedes relation created.
	if len(relRepo.saved) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(relRepo.saved))
	}
	if relRepo.saved[0].Type != shared.RelationTypeSupersedes {
		t.Errorf("expected supersedes relation, got %s", relRepo.saved[0].Type)
	}
	if !relRepo.saved[0].SourceID.Equal(result.ID) {
		t.Error("relation source should be new decision")
	}
	if !relRepo.saved[0].TargetID.Equal(existing.ID) {
		t.Error("relation target should be old decision")
	}
}

func TestDecisionService_Record_VersionAutoIncrement(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := testScope(t)

	existing, err := decision.NewDecision(
		"auth/token-strategy", "V3 title", "V3 desc", "V3 rationale",
		testEvidence(t), scope, testProvenance(t), testConfidence(t), clock, 3,
	)
	if err != nil {
		t.Fatal(err)
	}

	decRepo := &mockDecisionRepo{activeByKeyResult: existing}
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := newService(decRepo, relRepo, eventPub, clock)

	result, err := svc.Record(context.Background(), testCmd(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != 4 {
		t.Errorf("expected version 4, got %d", result.Version)
	}
}

func TestDecisionService_Get_Success(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))

	expected, err := decision.NewDecision(
		"auth/token-strategy", "Title", "Desc", "Rationale",
		testEvidence(t), testScope(t), testProvenance(t), testConfidence(t), clock, 1,
	)
	if err != nil {
		t.Fatal(err)
	}

	decRepo := &mockDecisionRepo{findByIDResult: expected}
	svc := newService(decRepo, &mockRelationRepo{}, &mockEventPublisher{}, clock)

	got, err := svc.Get(context.Background(), expected.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.ID.Equal(expected.ID) {
		t.Errorf("ID mismatch")
	}
}

func TestDecisionService_Get_NotFound(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))

	decRepo := &mockDecisionRepo{findByIDErr: shared.ErrNotFound}
	svc := newService(decRepo, &mockRelationRepo{}, &mockEventPublisher{}, clock)

	_, err := svc.Get(context.Background(), shared.NewRecordID())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDecisionService_GetHistory(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := testScope(t)

	d1, _ := decision.NewDecision("k", "T1", "D1", "R1", testEvidence(t), scope, testProvenance(t), testConfidence(t), clock, 1)
	d2, _ := decision.NewDecision("k", "T2", "D2", "R2", testEvidence(t), scope, testProvenance(t), testConfidence(t), clock, 2)

	decRepo := &mockDecisionRepo{findByKeyResult: []decision.Decision{*d2, *d1}}
	svc := newService(decRepo, &mockRelationRepo{}, &mockEventPublisher{}, clock)

	results, err := svc.GetHistory(context.Background(), inbound.DecisionHistoryQuery{
		DecisionKey: "k",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Version != 2 {
		t.Errorf("expected first result version 2, got %d", results[0].Version)
	}
}

func TestDecisionService_Contradict(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))

	target, err := decision.NewDecision(
		"auth/token-strategy", "Title", "Desc", "Rationale",
		testEvidence(t), testScope(t), testProvenance(t), testConfidence(t), clock, 1,
	)
	if err != nil {
		t.Fatal(err)
	}

	decRepo := &mockDecisionRepo{findByIDResult: target}
	relRepo := &mockRelationRepo{}
	eventPub := &mockEventPublisher{}

	svc := newService(decRepo, relRepo, eventPub, clock)

	contradictedBy := shared.NewRecordID()
	err = svc.Contradict(context.Background(), inbound.ContradictDecisionCmd{
		TargetID:       target.ID,
		ContradictedBy: contradictedBy,
		Reason:         "New evidence",
		Provenance:     testProvenance(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Status updated.
	if len(decRepo.statusUpdates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(decRepo.statusUpdates))
	}
	if decRepo.statusUpdates[0].Status != shared.DecisionStatusContradicted {
		t.Errorf("expected contradicted status, got %s", decRepo.statusUpdates[0].Status)
	}

	// Contradicts relation created.
	if len(relRepo.saved) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(relRepo.saved))
	}
	if relRepo.saved[0].Type != shared.RelationTypeContradicts {
		t.Errorf("expected contradicts relation, got %s", relRepo.saved[0].Type)
	}

	// Event published.
	if len(eventPub.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventPub.published))
	}
	if eventPub.published[0].Type != shared.EventTypeDecisionContradicted {
		t.Errorf("unexpected event type: %s", eventPub.published[0].Type)
	}
}

func TestDecisionService_Contradict_NotActive(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))

	target, err := decision.NewDecision(
		"auth/token-strategy", "Title", "Desc", "Rationale",
		testEvidence(t), testScope(t), testProvenance(t), testConfidence(t), clock, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Already superseded — not active.
	supersededBy := shared.NewRecordID()
	_ = target.Supersede(supersededBy)

	decRepo := &mockDecisionRepo{findByIDResult: target}
	svc := newService(decRepo, &mockRelationRepo{}, &mockEventPublisher{}, clock)

	err = svc.Contradict(context.Background(), inbound.ContradictDecisionCmd{
		TargetID:       target.ID,
		ContradictedBy: shared.NewRecordID(),
		Reason:         "Should fail",
		Provenance:     testProvenance(t),
	})
	if !errors.Is(err, shared.ErrNotActive) {
		t.Errorf("expected ErrNotActive, got: %v", err)
	}
}
