package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/adapters/inbound/http/middleware"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock AuthService -------------------------------------------------------

type mockAuthSvc struct {
	result auth.AuthContext
	err    error
}

func (m *mockAuthSvc) Authenticate(_ context.Context, _ string) (auth.AuthContext, error) {
	return m.result, m.err
}

// --- helpers ----------------------------------------------------------------

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	return body
}

// instrumentedHandler is a downstream handler that captures the AuthContext
// injected by the middleware so tests can assert context propagation.
type instrumentedHandler struct {
	called    bool
	authFound bool
	ac        auth.AuthContext
}

func (h *instrumentedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	ac, ok := auth.FromContext(r.Context())
	h.authFound = ok
	h.ac = ac
	w.WriteHeader(http.StatusOK)
}

// --- tests ------------------------------------------------------------------

func TestAPIKeyMiddleware_MissingHeader_Returns401(t *testing.T) {
	svc := &mockAuthSvc{}
	mw := middleware.APIKey(svc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	body := decodeBody(t, rr)
	assert.NotEmpty(t, body["error"])
}

func TestAPIKeyMiddleware_ValidKey_Returns200_AndInjectsContext(t *testing.T) {
	expected := auth.AuthContext{
		TenantID:  "sophia-dev",
		ProjectID: "sophia-dev",
		KeyID:     "01JTEST00000000000000000AA",
	}
	svc := &mockAuthSvc{result: expected}
	downstream := &instrumentedHandler{}
	mw := middleware.APIKey(svc)
	handler := mw(downstream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	req.Header.Set("X-API-Key", "memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, downstream.called, "downstream handler must be called")
	assert.True(t, downstream.authFound, "auth context must be present in downstream")
	assert.Equal(t, expected.TenantID, downstream.ac.TenantID)
	assert.Equal(t, expected.ProjectID, downstream.ac.ProjectID)
	assert.Equal(t, expected.KeyID, downstream.ac.KeyID)
}

func TestAPIKeyMiddleware_InvalidKey_Returns401_GenericMessage(t *testing.T) {
	svc := &mockAuthSvc{err: auth.ErrUnauthorized}
	mw := middleware.APIKey(svc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	req.Header.Set("X-API-Key", "wrong-key-that-is-long-enough-to-pass-length-check")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	body := decodeBody(t, rr)
	// Must be the generic message — no differentiation between missing/wrong/revoked.
	assert.Equal(t, "unauthorized", body["error"])
}

func TestAPIKeyMiddleware_RevokedKey_Returns401_GenericMessage(t *testing.T) {
	svc := &mockAuthSvc{err: auth.ErrUnauthorized}
	mw := middleware.APIKey(svc)
	downstream := &instrumentedHandler{}
	handler := mw(downstream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	req.Header.Set("X-API-Key", "memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, downstream.called)
}

func TestAPIKeyMiddleware_ShortKey_Returns401(t *testing.T) {
	// Even if the service returns ErrInvalidKey, the middleware must still 401.
	svc := &mockAuthSvc{err: auth.ErrInvalidKey}
	mw := middleware.APIKey(svc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not be called on short key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	req.Header.Set("X-API-Key", "tooshort")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
