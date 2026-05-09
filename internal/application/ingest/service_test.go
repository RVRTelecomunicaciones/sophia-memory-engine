package ingest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sophia-engine/memory-engine/internal/application/ingest"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// ---------------------------------------------------------------------------
// Mock: MemoryRepository
// ---------------------------------------------------------------------------

type mockMemoryRepo struct {
	saveFunc         func(ctx context.Context, record *memory.MemoryRecord) error
	findByIDFunc     func(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	findByTopicFunc  func(ctx context.Context, scope shared.Scope, topicKey string) (*memory.MemoryRecord, error)
	updateStatusFunc func(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error
	saved            []*memory.MemoryRecord
	lastTopicKey     string
	lastScope        shared.Scope
}

func (m *mockMemoryRepo) Save(ctx context.Context, record *memory.MemoryRecord) error {
	m.saved = append(m.saved, record)
	if m.saveFunc != nil {
		return m.saveFunc(ctx, record)
	}
	return nil
}

func (m *mockMemoryRepo) FindByID(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, shared.ErrNotFound
}

func (m *mockMemoryRepo) FindLatestActiveByTopicKey(ctx context.Context, scope shared.Scope, topicKey string) (*memory.MemoryRecord, error) {
	m.lastScope = scope
	m.lastTopicKey = topicKey
	if m.findByTopicFunc != nil {
		return m.findByTopicFunc(ctx, scope, topicKey)
	}
	return nil, shared.ErrNotFound
}

func (m *mockMemoryRepo) UpdateStatus(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status)
	}
	return nil
}

func (m *mockMemoryRepo) WipeContent(ctx context.Context, id shared.RecordID) error {
	return nil
}

// ---------------------------------------------------------------------------
// Mock: SearchIndex
// ---------------------------------------------------------------------------

type mockSearchIndex struct{}

func (m *mockSearchIndex) Index(_ context.Context, _ outbound.SearchEntry) error    { return nil }
func (m *mockSearchIndex) Remove(_ context.Context, _ shared.RecordID) error        { return nil }
func (m *mockSearchIndex) Search(_ context.Context, _ outbound.FTSQuery) ([]outbound.FTSResult, error) {
	return nil, nil
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

func newTestService() (*ingest.Service, *mockMemoryRepo, *mockEventPublisher) {
	repo := &mockMemoryRepo{}
	idx := &mockSearchIndex{}
	pub := &mockEventPublisher{}
	clock := newFixedClock()
	svc := ingest.NewService(repo, idx, pub, clock)
	return svc, repo, pub
}

// ---------------------------------------------------------------------------
// Tests: Ingest
// ---------------------------------------------------------------------------

func TestIngestService_Ingest_Episodic_Success(t *testing.T) {
	svc, repo, pub := newTestService()

	validFrom := fixedTime.Add(-24 * time.Hour)
	cmd := inbound.IngestMemoryCmd{
		Type:       shared.MemoryTypeEpisodic,
		Content:    "Discussed migration strategy with team",
		Scope:      validScope(t),
		Provenance: validProvenance(t),
		ValidFrom:  &validFrom,
	}

	result, err := svc.Ingest(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.ID.IsValid())
	assert.Equal(t, fixedTime, result.CreatedAt)
	assert.Len(t, repo.saved, 1)
	assert.Equal(t, cmd.Content, repo.saved[0].Content)
	assert.Len(t, pub.events, 1)
	assert.Equal(t, shared.EventTypeMemoryIngested, pub.events[0].Type)
}

func TestIngestService_Ingest_Episodic_MissingValidFrom(t *testing.T) {
	svc, _, _ := newTestService()

	cmd := inbound.IngestMemoryCmd{
		Type:       shared.MemoryTypeEpisodic,
		Content:    "Some episodic content",
		Scope:      validScope(t),
		Provenance: validProvenance(t),
		// ValidFrom intentionally nil
	}

	result, err := svc.Ingest(context.Background(), cmd)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestIngestService_Ingest_Semantic_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	cmd := inbound.IngestMemoryCmd{
		Type:       shared.MemoryTypeSemantic,
		Content:    "Go interfaces should be small and focused",
		Scope:      validScope(t),
		Provenance: validProvenance(t),
		// No ValidFrom needed for semantic
	}

	result, err := svc.Ingest(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.ID.IsValid())
	assert.Len(t, repo.saved, 1)
	assert.Equal(t, shared.MemoryTypeSemantic, repo.saved[0].Type)
}

func TestIngestService_Ingest_EmptyContent(t *testing.T) {
	svc, _, _ := newTestService()

	cmd := inbound.IngestMemoryCmd{
		Type:       shared.MemoryTypeSemantic,
		Content:    "",
		Scope:      validScope(t),
		Provenance: validProvenance(t),
	}

	result, err := svc.Ingest(context.Background(), cmd)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestIngestService_Ingest_WithAllOptions(t *testing.T) {
	svc, repo, _ := newTestService()

	validFrom := fixedTime.Add(-1 * time.Hour)
	summary := "A brief summary"
	topicKey := "arch/testing"
	ftsLang := "english"

	cmd := inbound.IngestMemoryCmd{
		Type:        shared.MemoryTypeEpisodic,
		Content:     "Full content with all options set",
		Summary:     &summary,
		Tags:        []string{"architecture", "testing"},
		TopicKey:    &topicKey,
		FTSLanguage: &ftsLang,
		Scope:       validScope(t),
		Provenance:  validProvenance(t),
		ValidFrom:   &validFrom,
	}

	result, err := svc.Ingest(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)

	saved := repo.saved[0]
	assert.Equal(t, &summary, saved.Summary)
	assert.Equal(t, []string{"architecture", "testing"}, saved.Tags)
	assert.Equal(t, &topicKey, saved.TopicKey)
	assert.Equal(t, "english", saved.FTSLanguage)
}

// ---------------------------------------------------------------------------
// Tests: Get
// ---------------------------------------------------------------------------

func TestIngestService_Get_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	record, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"test content",
		validScope(t),
		validProvenance(t),
		clock,
	)
	require.NoError(t, err)

	repo.findByIDFunc = func(_ context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	got, err := svc.Get(context.Background(), record.ID)

	require.NoError(t, err)
	assert.Equal(t, record.ID, got.ID)
	assert.Equal(t, record.Content, got.Content)
}

func TestIngestService_Get_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	got, err := svc.Get(context.Background(), shared.NewRecordID())

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestIngestService_Get_Purged(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.findByIDFunc = func(_ context.Context, _ shared.RecordID) (*memory.MemoryRecord, error) {
		return nil, shared.ErrPurged
	}

	got, err := svc.Get(context.Background(), shared.NewRecordID())

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrPurged))
}

// ---------------------------------------------------------------------------
// Tests: Archive
// ---------------------------------------------------------------------------

func TestIngestService_Archive_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	clock := newFixedClock()
	record, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"archivable content",
		validScope(t),
		validProvenance(t),
		clock,
	)
	require.NoError(t, err)

	repo.findByIDFunc = func(_ context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
		if id.Equal(record.ID) {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	var updatedStatus shared.MemoryStatus
	repo.updateStatusFunc = func(_ context.Context, _ shared.RecordID, status shared.MemoryStatus) error {
		updatedStatus = status
		return nil
	}

	cmd := inbound.ArchiveMemoryCmd{
		ID:          record.ID,
		Reason:      "no longer relevant",
		RequestedBy: "test-user",
	}

	err = svc.Archive(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, shared.MemoryStatusArchived, updatedStatus)
}

func TestIngestService_Archive_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	cmd := inbound.ArchiveMemoryCmd{
		ID:          shared.NewRecordID(),
		Reason:      "cleanup",
		RequestedBy: "test-user",
	}

	err := svc.Archive(context.Background(), cmd)

	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

// ---------------------------------------------------------------------------
// Tests: GetByTopicKey
// ---------------------------------------------------------------------------

func TestIngestService_GetByTopicKey_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	topicKey := "sdd/example/tasks"
	clock := newFixedClock()
	record, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"task list payload",
		validScope(t),
		validProvenance(t),
		clock,
		memory.WithTopicKey(topicKey),
	)
	require.NoError(t, err)

	repo.findByTopicFunc = func(_ context.Context, _ shared.Scope, key string) (*memory.MemoryRecord, error) {
		if key == topicKey {
			return record, nil
		}
		return nil, shared.ErrNotFound
	}

	got, err := svc.GetByTopicKey(context.Background(), inbound.GetByTopicKeyQuery{
		ProjectID: "test-project",
		TopicKey:  topicKey,
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, record.ID, got.ID)
	assert.Equal(t, topicKey, repo.lastTopicKey)
	assert.Equal(t, "test-project", repo.lastScope.ProjectID)
}

func TestIngestService_GetByTopicKey_PropagatesScopeFilters(t *testing.T) {
	svc, repo, _ := newTestService()

	repoID := "repo-x"
	agentID := "agent-y"
	envID := "production"

	repo.findByTopicFunc = func(_ context.Context, _ shared.Scope, _ string) (*memory.MemoryRecord, error) {
		return nil, shared.ErrNotFound
	}

	_, err := svc.GetByTopicKey(context.Background(), inbound.GetByTopicKeyQuery{
		ProjectID:   "test-project",
		TopicKey:    "sdd/example/tasks",
		RepoID:      &repoID,
		AgentID:     &agentID,
		Environment: &envID,
	})

	require.Error(t, err)
	require.NotNil(t, repo.lastScope.RepoID)
	assert.Equal(t, repoID, *repo.lastScope.RepoID)
	require.NotNil(t, repo.lastScope.AgentID)
	assert.Equal(t, agentID, *repo.lastScope.AgentID)
	require.NotNil(t, repo.lastScope.Environment)
	assert.Equal(t, envID, *repo.lastScope.Environment)
}

func TestIngestService_GetByTopicKey_MissingProjectID(t *testing.T) {
	svc, _, _ := newTestService()

	got, err := svc.GetByTopicKey(context.Background(), inbound.GetByTopicKeyQuery{
		TopicKey: "sdd/example/tasks",
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestIngestService_GetByTopicKey_MissingTopicKey(t *testing.T) {
	svc, _, _ := newTestService()

	got, err := svc.GetByTopicKey(context.Background(), inbound.GetByTopicKeyQuery{
		ProjectID: "test-project",
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestIngestService_GetByTopicKey_NotFound(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.findByTopicFunc = func(_ context.Context, _ shared.Scope, _ string) (*memory.MemoryRecord, error) {
		return nil, shared.ErrNotFound
	}

	got, err := svc.GetByTopicKey(context.Background(), inbound.GetByTopicKeyQuery{
		ProjectID: "test-project",
		TopicKey:  "missing",
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}
