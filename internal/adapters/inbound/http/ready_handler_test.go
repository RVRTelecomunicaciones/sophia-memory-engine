package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPinger lets a test control Ping's outcome (and optionally simulate a
// slow pinger that respects ctx.Done so we can assert timeout behavior).
type mockPinger struct {
	err     error
	delay   time.Duration
	calls   int
	lastCtx context.Context
}

func (m *mockPinger) Ping(ctx context.Context) error {
	m.calls++
	m.lastCtx = ctx
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.err
}

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	return body
}

func TestReadyHandler_OK(t *testing.T) {
	pinger := &mockPinger{}
	h := NewReadyHandler(pinger)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	start := time.Now()
	h.Ready(rr, req)
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, 1, pinger.calls)
	assert.Less(t, elapsed, 100*time.Millisecond, "happy path must respond in <100ms")

	body := decodeBody(t, rr)
	assert.Equal(t, "ready", body["status"])

	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "checks must be an object")
	assert.Equal(t, "ok", checks["db"])
}

func TestReadyHandler_DBFailure(t *testing.T) {
	pinger := &mockPinger{err: errors.New("connection refused")}
	h := NewReadyHandler(pinger)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	h.Ready(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	body := decodeBody(t, rr)
	assert.Equal(t, "degraded", body["status"])

	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "checks must be an object")
	assert.Equal(t, "connection refused", checks["db"])
}

func TestReadyHandler_Timeout(t *testing.T) {
	// Pinger blocks longer than the handler's internal 2s timeout. We shrink
	// the wait via the request context so the test stays fast: the handler's
	// timeout chains off r.Context(), so a 50ms request context wins.
	pinger := &mockPinger{delay: 5 * time.Second}
	h := NewReadyHandler(pinger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	h.Ready(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	body := decodeBody(t, rr)
	assert.Equal(t, "degraded", body["status"])

	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "checks must be an object")
	// On timeout the body surfaces the context error verbatim (e.g.
	// "context deadline exceeded") so operators can distinguish timeout
	// from a connection-refused style failure.
	assert.Contains(t, checks["db"], "context")
}
