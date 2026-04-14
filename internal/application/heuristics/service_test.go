package heuristics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sophia-engine/memory-engine/internal/application/heuristics"
	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// ---------------------------------------------------------------------------
// Mock: HeuristicRepository
// ---------------------------------------------------------------------------

type mockHeuristicRepo struct {
	saveFunc            func(ctx context.Context, rule *heuristic.HeuristicRule) error
	findByIDFunc        func(ctx context.Context, id shared.RecordID) (*heuristic.HeuristicRule, error)
	findActiveByKeyFunc func(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error)
	findByScopeFunc     func(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error)
	updateEnabledFunc   func(ctx context.Context, id shared.RecordID, enabled bool) error
	saved               []*heuristic.HeuristicRule
	disabledIDs         []shared.RecordID
}

func (m *mockHeuristicRepo) Save(ctx context.Context, rule *heuristic.HeuristicRule) error {
	m.saved = append(m.saved, rule)
	if m.saveFunc != nil {
		return m.saveFunc(ctx, rule)
	}
	return nil
}

func (m *mockHeuristicRepo) FindByID(ctx context.Context, id shared.RecordID) (*heuristic.HeuristicRule, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, shared.ErrNotFound
}

func (m *mockHeuristicRepo) FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error) {
	if m.findActiveByKeyFunc != nil {
		return m.findActiveByKeyFunc(ctx, key, scope)
	}
	return nil, shared.ErrNotFound
}

func (m *mockHeuristicRepo) FindByScope(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error) {
	if m.findByScopeFunc != nil {
		return m.findByScopeFunc(ctx, scope, enabled)
	}
	return nil, nil
}

func (m *mockHeuristicRepo) UpdateEnabled(ctx context.Context, id shared.RecordID, enabled bool) error {
	if !enabled {
		m.disabledIDs = append(m.disabledIDs, id)
	}
	if m.updateEnabledFunc != nil {
		return m.updateEnabledFunc(ctx, id, enabled)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mock: TransactionManager
// ---------------------------------------------------------------------------

type mockTxManager struct{}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// ---------------------------------------------------------------------------
// Mock: EventPublisher
// ---------------------------------------------------------------------------

type mockEventPublisher struct {
	events []outbound.DomainEvent
}

func (m *mockEventPublisher) Publish(_ context.Context, event outbound.DomainEvent) error {
	m.events = append(m.events, event)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

func newFixedClock() *shared.FixedClock {
	return shared.NewFixedClock(fixedTime)
}

func validScope(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("test-project")
	require.NoError(t, err)
	return s
}

func validProvenance(t *testing.T) shared.Provenance {
	t.Helper()
	p, err := shared.NewProvenance("test-source", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	return p
}

func validConfidence(t *testing.T) shared.Confidence {
	t.Helper()
	c, err := shared.NewConfidence(0.8, shared.ConfidenceSourceAgent)
	require.NoError(t, err)
	return c
}

func newTestService() (*heuristics.Service, *mockHeuristicRepo, *mockEventPublisher) {
	repo := &mockHeuristicRepo{}
	txMgr := &mockTxManager{}
	pub := &mockEventPublisher{}
	clock := newFixedClock()
	svc := heuristics.NewService(repo, txMgr, pub, clock)
	return svc, repo, pub
}

func newCreateCmd(t *testing.T) inbound.CreateHeuristicCmd {
	t.Helper()
	return inbound.CreateHeuristicCmd{
		HeuristicKey: "testing/always-integration",
		Condition:    "When changing DB code",
		Action:       "Run integration tests",
		Rationale:    "Mocks missed a real migration bug",
		Scope:        validScope(t),
		Provenance:   validProvenance(t),
		Confidence:   validConfidence(t),
	}
}

// ---------------------------------------------------------------------------
// Tests: Create
// ---------------------------------------------------------------------------

func TestHeuristicService_Create_FirstVersion(t *testing.T) {
	svc, repo, pub := newTestService()

	cmd := newCreateCmd(t)

	result, err := svc.Create(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.ID.IsValid())
	assert.Equal(t, 1, result.Version)
	assert.Nil(t, result.DisabledPrevious)
	assert.Equal(t, fixedTime, result.CreatedAt)

	// Verify saved.
	require.Len(t, repo.saved, 1)
	assert.Equal(t, cmd.HeuristicKey, repo.saved[0].HeuristicKey)
	assert.Equal(t, 1, repo.saved[0].Version)
	assert.True(t, repo.saved[0].Enabled)

	// Verify event published.
	require.Len(t, pub.events, 1)
	assert.Equal(t, shared.EventTypeHeuristicCreated, pub.events[0].Type)
}

func TestHeuristicService_Create_DisablesPrevious(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	existingRule, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "old condition", "old action", "old rationale",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 1,
	)
	require.NoError(t, err)

	repo.findActiveByKeyFunc = func(_ context.Context, key string, _ shared.Scope) (*heuristic.HeuristicRule, error) {
		if key == "testing/always-integration" {
			return existingRule, nil
		}
		return nil, shared.ErrNotFound
	}

	cmd := newCreateCmd(t)

	result, err := svc.Create(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Version)
	require.NotNil(t, result.DisabledPrevious)
	assert.True(t, result.DisabledPrevious.Equal(existingRule.ID))

	// Verify old was disabled.
	require.Len(t, repo.disabledIDs, 1)
	assert.True(t, repo.disabledIDs[0].Equal(existingRule.ID))

	// Verify new was saved.
	require.Len(t, repo.saved, 1)
	assert.Equal(t, 2, repo.saved[0].Version)
	assert.True(t, repo.saved[0].Enabled)
}

func TestHeuristicService_Create_VersionAutoIncrement(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	existingRule, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "condition", "action", "rationale",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 5,
	)
	require.NoError(t, err)

	repo.findActiveByKeyFunc = func(_ context.Context, key string, _ shared.Scope) (*heuristic.HeuristicRule, error) {
		if key == "testing/always-integration" {
			return existingRule, nil
		}
		return nil, shared.ErrNotFound
	}

	cmd := newCreateCmd(t)

	result, err := svc.Create(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, 6, result.Version)
	require.Len(t, repo.saved, 1)
	assert.Equal(t, 6, repo.saved[0].Version)
}

func TestHeuristicService_GetActive(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	rule, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "condition", "action", "rationale",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 1,
	)
	require.NoError(t, err)

	repo.findActiveByKeyFunc = func(_ context.Context, key string, _ shared.Scope) (*heuristic.HeuristicRule, error) {
		if key == "testing/always-integration" {
			return rule, nil
		}
		return nil, shared.ErrNotFound
	}

	got, err := svc.GetActive(context.Background(), inbound.GetActiveHeuristicQuery{
		HeuristicKey: "testing/always-integration",
		Scope:        validScope(t),
	})

	require.NoError(t, err)
	assert.True(t, got.ID.Equal(rule.ID))
}

func TestHeuristicService_GetActive_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	got, err := svc.GetActive(context.Background(), inbound.GetActiveHeuristicQuery{
		HeuristicKey: "nonexistent/key",
		Scope:        validScope(t),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestHeuristicService_ListByScope(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	rule1, err := heuristic.NewHeuristicRule(
		"testing/a", "c1", "a1", "r1",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 1,
	)
	require.NoError(t, err)

	rule2, err := heuristic.NewHeuristicRule(
		"testing/b", "c2", "a2", "r2",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 1,
	)
	require.NoError(t, err)

	repo.findByScopeFunc = func(_ context.Context, _ shared.Scope, _ *bool) ([]heuristic.HeuristicRule, error) {
		return []heuristic.HeuristicRule{*rule1, *rule2}, nil
	}

	rules, err := svc.ListByScope(context.Background(), inbound.ListHeuristicsQuery{
		Scope: validScope(t),
	})

	require.NoError(t, err)
	assert.Len(t, rules, 2)
}

func TestHeuristicService_Toggle_Enable_Conflict(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()

	// The rule to be toggled.
	targetRule, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "condition", "action", "rationale",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 1, heuristic.WithEnabled(false),
	)
	require.NoError(t, err)

	// A different rule that is currently active for the same key.
	activeRule, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "condition v2", "action v2", "rationale v2",
		validScope(t), validProvenance(t), validConfidence(t),
		clock, 2,
	)
	require.NoError(t, err)

	repo.findByIDFunc = func(_ context.Context, id shared.RecordID) (*heuristic.HeuristicRule, error) {
		if id.Equal(targetRule.ID) {
			return targetRule, nil
		}
		return nil, shared.ErrNotFound
	}

	repo.findActiveByKeyFunc = func(_ context.Context, key string, _ shared.Scope) (*heuristic.HeuristicRule, error) {
		if key == "testing/always-integration" {
			return activeRule, nil
		}
		return nil, shared.ErrNotFound
	}

	err = svc.Toggle(context.Background(), inbound.ToggleHeuristicCmd{
		ID:      targetRule.ID,
		Enabled: true,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrConflict))
}

func TestHeuristicService_Toggle_Disable(t *testing.T) {
	svc, repo, pub := newTestService()

	ruleID := shared.NewRecordID()

	var updatedID shared.RecordID
	var updatedEnabled bool
	repo.updateEnabledFunc = func(_ context.Context, id shared.RecordID, enabled bool) error {
		updatedID = id
		updatedEnabled = enabled
		return nil
	}

	err := svc.Toggle(context.Background(), inbound.ToggleHeuristicCmd{
		ID:      ruleID,
		Enabled: false,
	})

	require.NoError(t, err)
	assert.True(t, updatedID.Equal(ruleID))
	assert.False(t, updatedEnabled)

	// Verify event published.
	require.Len(t, pub.events, 1)
	assert.Equal(t, shared.EventTypeHeuristicToggled, pub.events[0].Type)
}
