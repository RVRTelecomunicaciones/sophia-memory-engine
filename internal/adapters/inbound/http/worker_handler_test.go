package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	inboundhttp "github.com/sophia-engine/memory-engine/internal/adapters/inbound/http"
	"github.com/sophia-engine/memory-engine/internal/adapters/inbound/http/middleware"
	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// captureHandler records every Handle call for async assertion.
type captureHandler struct {
	mu      sync.Mutex
	calls   []consolidation.PhaseArchivedReceived
	errToReturn error
}

func (c *captureHandler) Handle(_ context.Context, p consolidation.PhaseArchivedReceived) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, p)
	return c.errToReturn
}

func (c *captureHandler) CallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// mockAuthSvcWorker always approves the API key "valid-key".
type mockAuthSvcWorker struct{}

func (m *mockAuthSvcWorker) Authenticate(_ context.Context, key string) (auth.AuthContext, error) {
	if key == "valid-key" {
		return auth.AuthContext{TenantID: "test-tenant", ProjectID: "test-project", KeyID: "key-1"}, nil
	}
	return auth.AuthContext{}, auth.ErrUnauthorized
}

func buildWorkerRouter(h *captureHandler) chi.Router {
	authSvc := &mockAuthSvcWorker{}
	workerH := inboundhttp.NewWorkerHandler(h)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.APIKey(authSvc))
		r.Post("/worker/phase-archived", workerH.PhaseArchived)
	})
	return r
}

func validPayload() consolidation.PhaseArchivedReceived {
	return consolidation.PhaseArchivedReceived{
		ChangeID:   "change-abc",
		ChangeName: "my-feature",
		PhaseType:  "archive",
		ArchivedAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
	}
}

func postWorker(t *testing.T, router http.Handler, payload any, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worker/phase-archived", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// I.1: valid payload + correct API key → 202; Handle called asynchronously
// ---------------------------------------------------------------------------

func TestWorkerHandler_PhaseArchived_ValidPayload_Returns202(t *testing.T) {
	h := &captureHandler{}
	router := buildWorkerRouter(h)

	payload := validPayload()
	rr := postWorker(t, router, payload, "valid-key")

	assert.Equal(t, http.StatusAccepted, rr.Code, "must return 202 Accepted immediately")

	// Give the goroutine time to complete.
	require.Eventually(t, func() bool { return h.CallCount() == 1 }, time.Second, 5*time.Millisecond,
		"Handle must be called exactly once asynchronously")

	h.mu.Lock()
	got := h.calls[0]
	h.mu.Unlock()
	assert.Equal(t, "change-abc", got.ChangeID)
	assert.Equal(t, "my-feature", got.ChangeName)
}

// ---------------------------------------------------------------------------
// I.2: malformed JSON body → 400; no pipeline triggered
// ---------------------------------------------------------------------------

func TestWorkerHandler_PhaseArchived_MalformedJSON_Returns400(t *testing.T) {
	h := &captureHandler{}
	router := buildWorkerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/worker/phase-archived",
		strings.NewReader("not-valid-json{{"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "valid-key")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "malformed JSON must return 400")
	// No goroutine spawned — HandleCount stays 0.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, h.CallCount(), "no pipeline must be triggered for invalid JSON")
}

// ---------------------------------------------------------------------------
// I.3: missing API-key header → 401
// ---------------------------------------------------------------------------

func TestWorkerHandler_PhaseArchived_MissingAPIKey_Returns401(t *testing.T) {
	h := &captureHandler{}
	router := buildWorkerRouter(h)

	rr := postWorker(t, router, validPayload(), "" /* no key */)

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "missing API key must return 401")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, h.CallCount(), "no processing must occur when auth fails")
}

// ---------------------------------------------------------------------------
// I.4: wrong API-key value → 401
// ---------------------------------------------------------------------------

func TestWorkerHandler_PhaseArchived_WrongAPIKey_Returns401(t *testing.T) {
	h := &captureHandler{}
	router := buildWorkerRouter(h)

	rr := postWorker(t, router, validPayload(), "wrong-key")

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "wrong API key must return 401")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, h.CallCount(), "no processing must occur when auth fails")
}
