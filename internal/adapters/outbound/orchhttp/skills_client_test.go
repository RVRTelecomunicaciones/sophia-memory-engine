package orchhttp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/orchhttp"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// H.1: PatchMetrics marshals correct delta JSON + sends ORCH_API_KEY header
// ---------------------------------------------------------------------------

func TestSkillsClient_PatchMetrics_MarshalsDeltaAndSendsAPIKey(t *testing.T) {
	var capturedBody map[string]interface{}
	var capturedKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("X-API-Key")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	}))
	defer srv.Close()

	client, err := orchhttp.NewSkillsClient(srv.URL, "test-api-key")
	require.NoError(t, err)

	delta := outbound.MetricsDelta{
		SuccessDelta:           1,
		FailureDelta:           0,
		TestsPassedDelta:       1,
		RollbackDelta:          0,
		DeprecatedAPIHitsDelta: 0,
		AvgRetryReduction:      0.25,
	}

	err = client.PatchMetrics(context.Background(), "skill-001", delta)
	require.NoError(t, err)

	assert.Equal(t, "test-api-key", capturedKey, "X-API-Key header must match the configured key")
	assert.Equal(t, float64(1), capturedBody["success_delta"], "success_delta must be 1")
	assert.Equal(t, float64(0), capturedBody["failure_delta"], "failure_delta must be 0")
	assert.Equal(t, float64(1), capturedBody["tests_passed_delta"], "tests_passed_delta must be 1")
	assert.InDelta(t, 0.25, capturedBody["avg_retry_reduction"], 1e-9, "avg_retry_reduction must be 0.25")
}

// ---------------------------------------------------------------------------
// H.2: Retry — 3 attempts with backoff on 5xx; 4th call not made
// ---------------------------------------------------------------------------

func TestSkillsClient_PatchMetrics_RetryOn5xx(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Use very short backoff for test speed.
	client, err := orchhttp.NewSkillsClientWithOptions(srv.URL, "test-api-key", orchhttp.Options{
		MaxRetries:    3,
		BaseBackoff:   5 * time.Millisecond,
		BackoffFactor: 2.0,
	})
	require.NoError(t, err)

	delta := outbound.MetricsDelta{SuccessDelta: 1}
	err = client.PatchMetrics(context.Background(), "skill-001", delta)
	require.Error(t, err, "should return error after all retries exhausted")

	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "must attempt exactly 3 times; 4th call must not be made")
}

// ---------------------------------------------------------------------------
// H.3: 404 returns typed error containing status code
// ---------------------------------------------------------------------------

func TestSkillsClient_PatchMetrics_404ReturnsTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "skill not found"}) //nolint:errcheck
	}))
	defer srv.Close()

	client, err := orchhttp.NewSkillsClient(srv.URL, "test-api-key")
	require.NoError(t, err)

	delta := outbound.MetricsDelta{SuccessDelta: 1}
	err = client.PatchMetrics(context.Background(), "skill-404", delta)
	require.Error(t, err, "must return non-nil error for 404")

	var httpErr *outbound.HTTPStatusError
	require.ErrorAs(t, err, &httpErr, "error must be a typed HTTPStatusError")
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode, "error must contain the HTTP status code 404")
}

// ---------------------------------------------------------------------------
// H.4: GetSkill round-trip — fake orch returns valid JSON → populated *SkillSnapshot
// ---------------------------------------------------------------------------

func TestSkillsClient_GetSkill_RoundTrip(t *testing.T) {
	expected := &outbound.SkillSnapshot{
		SkillID:   "skill-123",
		Status:    "candidate",
		RiskLevel: "low",
		Version:   "v1",
		Metrics: outbound.SkillMetrics{
			UsageCount:        3,
			SuccessCount:      1,
			FailureCount:      0,
			TestsPassedCount:  1,
			AvgRetryReduction: 0.25,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/api/v1/skills/%s", expected.SkillID), r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expected) //nolint:errcheck
	}))
	defer srv.Close()

	client, err := orchhttp.NewSkillsClient(srv.URL, "test-api-key")
	require.NoError(t, err)

	got, err := client.GetSkill(context.Background(), "skill-123")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, expected.SkillID, got.SkillID)
	assert.Equal(t, expected.Status, got.Status)
	assert.Equal(t, expected.RiskLevel, got.RiskLevel)
	assert.Equal(t, expected.Metrics.UsageCount, got.Metrics.UsageCount)
	assert.Equal(t, expected.Metrics.SuccessCount, got.Metrics.SuccessCount)
}

// ---------------------------------------------------------------------------
// H.5: Constructor with empty API key returns error
// ---------------------------------------------------------------------------

func TestSkillsClient_EmptyAPIKey_ReturnsError(t *testing.T) {
	_, err := orchhttp.NewSkillsClient("http://example.com", "")
	require.Error(t, err, "constructor must reject empty API key")
	assert.Contains(t, err.Error(), "api key", "error message must mention api key")
}
