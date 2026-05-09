package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock: inbound.MemoryService
// ---------------------------------------------------------------------------

type mockMemoryService struct {
	ingestFunc        func(ctx context.Context, cmd inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error)
	getFunc           func(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	getByTopicKeyFunc func(ctx context.Context, query inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error)
	archiveFunc       func(ctx context.Context, cmd inbound.ArchiveMemoryCmd) error

	lastTopicKeyQuery inbound.GetByTopicKeyQuery
}

func (m *mockMemoryService) Ingest(ctx context.Context, cmd inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error) {
	if m.ingestFunc != nil {
		return m.ingestFunc(ctx, cmd)
	}
	return nil, errors.New("not implemented")
}

func (m *mockMemoryService) Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, shared.ErrNotFound
}

func (m *mockMemoryService) GetByTopicKey(ctx context.Context, query inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
	m.lastTopicKeyQuery = query
	if m.getByTopicKeyFunc != nil {
		return m.getByTopicKeyFunc(ctx, query)
	}
	return nil, shared.ErrNotFound
}

func (m *mockMemoryService) Archive(ctx context.Context, cmd inbound.ArchiveMemoryCmd) error {
	if m.archiveFunc != nil {
		return m.archiveFunc(ctx, cmd)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestRecord(t *testing.T, topicKey string) *memory.MemoryRecord {
	t.Helper()
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, err := shared.NewScope("proj-1")
	require.NoError(t, err)
	prov, err := shared.NewProvenance("test-source", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	rec, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"task list payload",
		scope,
		prov,
		clock,
		memory.WithTopicKey(topicKey),
	)
	require.NoError(t, err)
	return rec
}

func newRequest(t *testing.T, method, target string, query url.Values) *http.Request {
	t.Helper()
	if query != nil {
		target = target + "?" + query.Encode()
	}
	req := httptest.NewRequest(method, target, nil)
	return req
}

// invokeGetByTopicKey calls the handler directly without router-level
// middleware so each test stays focused on handler behavior.
func invokeGetByTopicKey(handler *MemoryHandler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.GetByTopicKey(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Tests: GetByTopicKey
// ---------------------------------------------------------------------------

func TestMemoryHandler_GetByTopicKey_Success(t *testing.T) {
	svc := &mockMemoryService{}
	rec := newTestRecord(t, "sdd/example/tasks")
	svc.getByTopicKeyFunc = func(_ context.Context, _ inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
		return rec, nil
	}

	q := url.Values{
		"project_id": []string{"proj-1"},
		"topic_key":  []string{"sdd/example/tasks"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body memoryResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, rec.ID.String(), body.ID)
	require.NotNil(t, body.TopicKey)
	assert.Equal(t, "sdd/example/tasks", *body.TopicKey)
	assert.Equal(t, "proj-1", body.Scope.ProjectID)
	assert.Equal(t, "proj-1", svc.lastTopicKeyQuery.ProjectID)
	assert.Equal(t, "sdd/example/tasks", svc.lastTopicKeyQuery.TopicKey)
}

func TestMemoryHandler_GetByTopicKey_PropagatesOptionalScope(t *testing.T) {
	svc := &mockMemoryService{}
	rec := newTestRecord(t, "sdd/example/tasks")
	svc.getByTopicKeyFunc = func(_ context.Context, _ inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
		return rec, nil
	}

	q := url.Values{
		"project_id":  []string{"proj-1"},
		"topic_key":   []string{"sdd/example/tasks"},
		"tenant_id":   []string{"tenant-1"},
		"repo_id":     []string{"repo-1"},
		"agent_id":    []string{"agent-1"},
		"session_id":  []string{"sess-1"},
		"environment": []string{"production"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	require.Equal(t, http.StatusOK, rr.Code)

	got := svc.lastTopicKeyQuery
	require.NotNil(t, got.TenantID)
	assert.Equal(t, "tenant-1", *got.TenantID)
	require.NotNil(t, got.RepoID)
	assert.Equal(t, "repo-1", *got.RepoID)
	require.NotNil(t, got.AgentID)
	assert.Equal(t, "agent-1", *got.AgentID)
	require.NotNil(t, got.SessionID)
	assert.Equal(t, "sess-1", *got.SessionID)
	require.NotNil(t, got.Environment)
	assert.Equal(t, "production", *got.Environment)
}

func TestMemoryHandler_GetByTopicKey_MissingProjectID(t *testing.T) {
	svc := &mockMemoryService{}
	q := url.Values{
		"topic_key": []string{"sdd/example/tasks"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestMemoryHandler_GetByTopicKey_MissingTopicKey(t *testing.T) {
	svc := &mockMemoryService{}
	q := url.Values{
		"project_id": []string{"proj-1"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestMemoryHandler_GetByTopicKey_NotFound(t *testing.T) {
	svc := &mockMemoryService{
		getByTopicKeyFunc: func(_ context.Context, _ inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
			return nil, shared.ErrNotFound
		},
	}
	q := url.Values{
		"project_id": []string{"proj-1"},
		"topic_key":  []string{"missing"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestMemoryHandler_GetByTopicKey_GenericError(t *testing.T) {
	svc := &mockMemoryService{
		getByTopicKeyFunc: func(_ context.Context, _ inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
			return nil, errors.New("db down")
		},
	}
	q := url.Values{
		"project_id": []string{"proj-1"},
		"topic_key":  []string{"sdd/example/tasks"},
	}
	req := newRequest(t, http.MethodGet, "/api/v1/memories/by-topic-key", q)

	rr := invokeGetByTopicKey(NewMemoryHandler(svc), req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestMemoryHandler_GetByTopicKey_RoutingPrecedence ensures the explicit
// /memories/by-topic-key route resolves before /memories/{id} so chi does
// NOT treat "by-topic-key" as a literal record ID.
func TestMemoryHandler_GetByTopicKey_RoutingPrecedence(t *testing.T) {
	svc := &mockMemoryService{
		getByTopicKeyFunc: func(_ context.Context, _ inbound.GetByTopicKeyQuery) (*memory.MemoryRecord, error) {
			return newTestRecord(t, "sdd/example/tasks"), nil
		},
		getFunc: func(_ context.Context, _ shared.RecordID) (*memory.MemoryRecord, error) {
			t.Fatal("Get should not be called for /memories/by-topic-key")
			return nil, nil
		},
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		h := NewMemoryHandler(svc)
		r.Get("/memories/by-topic-key", h.GetByTopicKey)
		r.Get("/memories/{id}", h.Get)
	})

	q := url.Values{
		"project_id": []string{"proj-1"},
		"topic_key":  []string{"sdd/example/tasks"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/by-topic-key?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
