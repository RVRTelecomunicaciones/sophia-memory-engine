package security_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sophia-engine/memory-engine/internal/application/security"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockPurgeRepo struct {
	saveFunc         func(ctx context.Context, record *purge.PurgeRecord) error
	findByIDFunc     func(ctx context.Context, id shared.RecordID) (*purge.PurgeRecord, error)
	updateStatusFunc func(ctx context.Context, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error
	saved            []*purge.PurgeRecord
}

func (m *mockPurgeRepo) Save(ctx context.Context, record *purge.PurgeRecord) error {
	m.saved = append(m.saved, record)
	if m.saveFunc != nil {
		return m.saveFunc(ctx, record)
	}
	return nil
}

func (m *mockPurgeRepo) FindByID(ctx context.Context, id shared.RecordID) (*purge.PurgeRecord, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, shared.ErrNotFound
}

func (m *mockPurgeRepo) UpdateStatus(ctx context.Context, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status, artifacts)
	}
	return nil
}

type mockMemoryRepo struct {
	findByIDFunc func(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	wipeFunc     func(ctx context.Context, id shared.RecordID) error
}

func (m *mockMemoryRepo) Save(_ context.Context, _ *memory.MemoryRecord) error { return nil }
func (m *mockMemoryRepo) FindByID(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, shared.ErrNotFound
}
func (m *mockMemoryRepo) UpdateStatus(_ context.Context, _ shared.RecordID, _ shared.MemoryStatus) error {
	return nil
}
func (m *mockMemoryRepo) WipeContent(ctx context.Context, id shared.RecordID) error {
	if m.wipeFunc != nil {
		return m.wipeFunc(ctx, id)
	}
	return nil
}

type mockRelationRepo struct {
	deleteByTargetFunc func(ctx context.Context, targetID shared.RecordID) (int, error)
}

func (m *mockRelationRepo) Save(_ context.Context, _ *relation.Relation) error { return nil }
func (m *mockRelationRepo) FindFromSource(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}
func (m *mockRelationRepo) FindToTarget(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}
func (m *mockRelationRepo) Traverse(_ context.Context, _ outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	return nil, nil
}
func (m *mockRelationRepo) DeleteByTarget(ctx context.Context, targetID shared.RecordID) (int, error) {
	if m.deleteByTargetFunc != nil {
		return m.deleteByTargetFunc(ctx, targetID)
	}
	return 0, nil
}

type mockSearchIndex struct {
	removeFunc func(ctx context.Context, id shared.RecordID) error
}

func (m *mockSearchIndex) Index(_ context.Context, _ outbound.SearchEntry) error { return nil }
func (m *mockSearchIndex) Remove(ctx context.Context, id shared.RecordID) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, id)
	}
	return nil
}
func (m *mockSearchIndex) Search(_ context.Context, _ outbound.FTSQuery) ([]outbound.FTSResult, error) {
	return nil, nil
}

type mockTxManager struct {
	withTxFunc func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.withTxFunc != nil {
		return m.withTxFunc(ctx, fn)
	}
	return fn(ctx)
}

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

func newTestMemory(t *testing.T) *memory.MemoryRecord {
	t.Helper()
	clock := newFixedClock()
	rec, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"sensitive content to purge",
		validScope(t),
		validProvenance(t),
		clock,
	)
	require.NoError(t, err)
	return rec
}

type testDeps struct {
	svc      *security.Service
	purgeR   *mockPurgeRepo
	memR     *mockMemoryRepo
	relR     *mockRelationRepo
	searchI  *mockSearchIndex
	txMgr    *mockTxManager
	eventPub *mockEventPublisher
	clock    *shared.FixedClock
}

func newTestService() testDeps {
	purgeR := &mockPurgeRepo{}
	memR := &mockMemoryRepo{}
	relR := &mockRelationRepo{}
	searchI := &mockSearchIndex{}
	txMgr := &mockTxManager{}
	eventPub := &mockEventPublisher{}
	clock := newFixedClock()

	svc := security.NewService(purgeR, memR, relR, searchI, txMgr, eventPub, clock)
	return testDeps{
		svc:      svc,
		purgeR:   purgeR,
		memR:     memR,
		relR:     relR,
		searchI:  searchI,
		txMgr:    txMgr,
		eventPub: eventPub,
		clock:    clock,
	}
}

// ---------------------------------------------------------------------------
// Tests: Request
// ---------------------------------------------------------------------------

func TestPurgeService_Request_Success(t *testing.T) {
	deps := newTestService()
	mem := newTestMemory(t)

	deps.memR.findByIDFunc = func(_ context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
		if id.Equal(mem.ID) {
			return mem, nil
		}
		return nil, shared.ErrNotFound
	}

	cmd := inbound.RequestPurgeCmd{
		TargetID:    mem.ID,
		Reason:      shared.PurgeReasonSecretLeak,
		RequestedBy: "user:russell",
		AuditNote:   "API key leaked in memory content",
	}

	result, err := deps.svc.Request(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.ID.IsValid())
	assert.Equal(t, mem.ID, result.TargetID)
	assert.Equal(t, "memory", result.TargetType)
	assert.Equal(t, shared.PurgeReasonSecretLeak, result.Reason)
	assert.Equal(t, shared.PurgeStatusPending, result.Status)
	assert.Equal(t, "user:russell", result.RequestedBy)
	assert.Equal(t, mem.Scope.ProjectID, result.Scope.ProjectID)
	assert.Len(t, deps.purgeR.saved, 1)
	assert.Len(t, deps.eventPub.events, 1)
	assert.Equal(t, shared.EventTypePurgeRequested, deps.eventPub.events[0].Type)
}

func TestPurgeService_Request_TargetNotFound(t *testing.T) {
	deps := newTestService()

	cmd := inbound.RequestPurgeCmd{
		TargetID:    shared.NewRecordID(),
		Reason:      shared.PurgeReasonSecretLeak,
		RequestedBy: "user:russell",
		AuditNote:   "leaked secret",
	}

	result, err := deps.svc.Request(context.Background(), cmd)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

// ---------------------------------------------------------------------------
// Tests: Execute
// ---------------------------------------------------------------------------

func TestPurgeService_Execute_Success(t *testing.T) {
	deps := newTestService()

	// Create a pending purge record.
	record, err := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonSecretLeak, "user:russell", "leaked key",
		validScope(t), deps.clock,
	)
	require.NoError(t, err)

	deps.purgeR.findByIDFunc = func(_ context.Context, id shared.RecordID) (*purge.PurgeRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	var wipeCalled bool
	deps.memR.wipeFunc = func(_ context.Context, _ shared.RecordID) error {
		wipeCalled = true
		return nil
	}

	var removeCalled bool
	deps.searchI.removeFunc = func(_ context.Context, _ shared.RecordID) error {
		removeCalled = true
		return nil
	}

	deps.relR.deleteByTargetFunc = func(_ context.Context, _ shared.RecordID) (int, error) {
		return 3, nil
	}

	var updatedStatus shared.PurgeStatus
	var updatedArtifacts *purge.PurgedArtifacts
	deps.purgeR.updateStatusFunc = func(_ context.Context, _ shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error {
		updatedStatus = status
		updatedArtifacts = artifacts
		return nil
	}

	cmd := inbound.ExecutePurgeCmd{PurgeID: record.ID}
	result, err := deps.svc.Execute(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, wipeCalled, "WipeContent should be called")
	assert.True(t, removeCalled, "SearchIndex.Remove should be called")
	assert.Equal(t, shared.PurgeStatusExecuted, updatedStatus)
	require.NotNil(t, updatedArtifacts)
	assert.True(t, updatedArtifacts.FTSInvalidated)
	assert.Equal(t, 3, updatedArtifacts.RelationsRemoved)
	assert.Len(t, deps.eventPub.events, 1)
	assert.Equal(t, shared.EventTypePurgeExecuted, deps.eventPub.events[0].Type)
}

func TestPurgeService_Execute_NotPending(t *testing.T) {
	deps := newTestService()

	record, err := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonSecretLeak, "user:russell", "leaked key",
		validScope(t), deps.clock,
	)
	require.NoError(t, err)

	// Transition to failed so it's no longer pending.
	record.MarkFailed("previous failure")

	deps.purgeR.findByIDFunc = func(_ context.Context, id shared.RecordID) (*purge.PurgeRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	cmd := inbound.ExecutePurgeCmd{PurgeID: record.ID}
	result, err := deps.svc.Execute(context.Background(), cmd)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, shared.ErrNotActive))
}

func TestPurgeService_Execute_AlreadyExecuted(t *testing.T) {
	deps := newTestService()

	record, err := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonSecretLeak, "user:russell", "leaked key",
		validScope(t), deps.clock,
	)
	require.NoError(t, err)

	// Transition to executed.
	require.NoError(t, record.MarkExecuting())
	require.NoError(t, record.MarkExecuted(purge.PurgedArtifacts{}, deps.clock))

	deps.purgeR.findByIDFunc = func(_ context.Context, id shared.RecordID) (*purge.PurgeRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	cmd := inbound.ExecutePurgeCmd{PurgeID: record.ID}
	result, err := deps.svc.Execute(context.Background(), cmd)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, shared.ErrNotActive))
}

func TestPurgeService_Execute_TxFailure_MarksFailed(t *testing.T) {
	deps := newTestService()

	record, err := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonSecretLeak, "user:russell", "leaked key",
		validScope(t), deps.clock,
	)
	require.NoError(t, err)

	deps.purgeR.findByIDFunc = func(_ context.Context, id shared.RecordID) (*purge.PurgeRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	txError := errors.New("database connection lost")
	deps.txMgr.withTxFunc = func(_ context.Context, _ func(ctx context.Context) error) error {
		return txError
	}

	var failedStatus shared.PurgeStatus
	deps.purgeR.updateStatusFunc = func(_ context.Context, _ shared.RecordID, status shared.PurgeStatus, _ *purge.PurgedArtifacts) error {
		failedStatus = status
		return nil
	}

	cmd := inbound.ExecutePurgeCmd{PurgeID: record.ID}
	result, err := deps.svc.Execute(context.Background(), cmd)

	require.Error(t, err)
	assert.Equal(t, txError, err)
	require.NotNil(t, result)
	assert.Equal(t, shared.PurgeStatusFailed, result.Status)
	assert.Equal(t, shared.PurgeStatusFailed, failedStatus)
	require.NotNil(t, result.ErrorDetail)
	assert.Equal(t, "database connection lost", *result.ErrorDetail)
}
