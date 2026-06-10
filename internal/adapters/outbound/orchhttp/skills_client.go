// Package orchhttp provides the HTTP adapter for the SkillsClient outbound port.
// It communicates with sophia-orchestrator's skills write API.
//
// Configuration via environment variables:
//   - ORCH_BASE_URL: base URL of the orch instance (e.g. http://orch:8080)
//   - ORCH_API_KEY:  API key for X-API-Key header authentication
//
// Retry policy: up to 3 attempts on HTTP 5xx with exponential backoff
// 100ms → 500ms → 2.5s (configurable via Options for testing).
package orchhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Options configures retry behaviour. Use NewSkillsClientWithOptions to override
// defaults; the default constructor NewSkillsClient uses production defaults.
type Options struct {
	// MaxRetries is the number of attempts before giving up on a 5xx error.
	// Minimum useful value is 1 (no retry).
	MaxRetries int
	// BaseBackoff is the sleep duration before the second attempt.
	BaseBackoff time.Duration
	// BackoffFactor multiplies BaseBackoff on each subsequent attempt.
	BackoffFactor float64
}

var defaultOptions = Options{
	MaxRetries:    3,
	BaseBackoff:   100 * time.Millisecond,
	BackoffFactor: 5.0, // 100ms → 500ms → 2500ms
}

// SkillsClientHTTP is the HTTP adapter for outbound.SkillsClient.
type SkillsClientHTTP struct {
	baseURL string
	apiKey  string
	opts    Options
	http    *http.Client
}

// NewSkillsClient constructs a SkillsClientHTTP with production retry defaults.
// Returns an error if apiKey is empty.
func NewSkillsClient(baseURL, apiKey string) (*SkillsClientHTTP, error) {
	return NewSkillsClientWithOptions(baseURL, apiKey, defaultOptions)
}

// NewSkillsClientWithOptions constructs a SkillsClientHTTP with custom options.
// Returns an error if apiKey is empty.
func NewSkillsClientWithOptions(baseURL, apiKey string, opts Options) (*SkillsClientHTTP, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("orchhttp: api key must not be empty")
	}
	if opts.MaxRetries < 1 {
		opts.MaxRetries = 1
	}
	return &SkillsClientHTTP{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		opts:    opts,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// patchMetricsBody mirrors the JSON schema for PATCH /api/v1/skills/{id}/metrics.
// Field names MUST match orch's patchMetricsReq exactly.
type patchMetricsBody struct {
	SuccessDelta           int     `json:"success_delta"`
	FailureDelta           int     `json:"failure_delta"`
	TestsPassedDelta       int     `json:"tests_passed_delta"`
	RollbackDelta          int     `json:"rollback_delta"`
	DeprecatedAPIHitsDelta int     `json:"deprecated_api_hits_delta"`
	UsageDelta             int     `json:"usage_delta"`
	AvgRetryReduction      float64 `json:"avg_retry_reduction"`
}

// patchStatusBody mirrors the JSON schema for PATCH /api/v1/skills/{id}/status.
type patchStatusBody struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// usageResponse mirrors GET /api/v1/skills/usage response envelope.
type usageResponse struct {
	Items []outbound.SkillUsageRow `json:"items"`
}

// PatchMetrics implements outbound.SkillsClient.
func (c *SkillsClientHTTP) PatchMetrics(ctx context.Context, skillID string, delta outbound.MetricsDelta) error {
	body := patchMetricsBody{
		SuccessDelta:           delta.SuccessDelta,
		FailureDelta:           delta.FailureDelta,
		TestsPassedDelta:       delta.TestsPassedDelta,
		RollbackDelta:          delta.RollbackDelta,
		DeprecatedAPIHitsDelta: delta.DeprecatedAPIHitsDelta,
		UsageDelta:             delta.UsageDelta,
		AvgRetryReduction:      delta.AvgRetryReduction,
	}
	url := fmt.Sprintf("%s/api/v1/skills/%s/metrics", c.baseURL, skillID)
	return c.doWithRetry(ctx, http.MethodPatch, url, body)
}

// PatchStatus implements outbound.SkillsClient.
func (c *SkillsClientHTTP) PatchStatus(ctx context.Context, skillID string, status string, reason string) error {
	body := patchStatusBody{Status: status, Reason: reason}
	url := fmt.Sprintf("%s/api/v1/skills/%s/status", c.baseURL, skillID)
	return c.doWithRetry(ctx, http.MethodPatch, url, body)
}

// GetSkill implements outbound.SkillsClient.
// Calls GET /api/v1/skills/{id} on orch — requires orch to expose that endpoint.
func (c *SkillsClientHTTP) GetSkill(ctx context.Context, skillID string) (*outbound.SkillSnapshot, error) {
	url := fmt.Sprintf("%s/api/v1/skills/%s", c.baseURL, skillID)
	req, err := c.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("orchhttp.GetSkill: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := c.checkStatus(resp); err != nil {
		return nil, err
	}

	var snap outbound.SkillSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("orchhttp.GetSkill: decode response: %w", err)
	}
	return &snap, nil
}

// GetUsage implements outbound.SkillsClient.
// Calls GET /api/v1/skills/usage?change_id=... on orch.
func (c *SkillsClientHTTP) GetUsage(ctx context.Context, changeID string) ([]outbound.SkillUsageRow, error) {
	url := fmt.Sprintf("%s/api/v1/skills/usage?change_id=%s", c.baseURL, changeID)
	req, err := c.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("orchhttp.GetUsage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := c.checkStatus(resp); err != nil {
		return nil, err
	}

	var envelope usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("orchhttp.GetUsage: decode response: %w", err)
	}
	return envelope.Items, nil
}

// doWithRetry sends a JSON request and retries up to opts.MaxRetries times on 5xx.
// 4xx errors are returned immediately without retry as typed *outbound.HTTPStatusError.
func (c *SkillsClientHTTP) doWithRetry(ctx context.Context, method, url string, body any) error {
	backoff := c.opts.BaseBackoff
	var lastErr error
	for attempt := 0; attempt < c.opts.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(float64(backoff) * c.opts.BackoffFactor)
		}

		req, err := c.newRequest(ctx, method, url, body)
		if err != nil {
			return err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("orchhttp: request failed: %w", err)
			continue
		}
		_ = resp.Body.Close()

		// 4xx: do not retry — return typed error immediately.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return &outbound.HTTPStatusError{
				StatusCode: resp.StatusCode,
				Message:    http.StatusText(resp.StatusCode),
			}
		}
		// 2xx: success.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		// 5xx: retry.
		lastErr = fmt.Errorf("orchhttp: server error %d", resp.StatusCode)
	}
	return fmt.Errorf("orchhttp: all %d attempts failed: %w", c.opts.MaxRetries, lastErr)
}

// newRequest builds an authenticated HTTP request. body may be nil for GET requests.
func (c *SkillsClientHTTP) newRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("orchhttp: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("orchhttp: build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// checkStatus returns an *outbound.HTTPStatusError for non-2xx responses.
func (c *SkillsClientHTTP) checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return &outbound.HTTPStatusError{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(b)),
	}
}

// compile-time guard: *SkillsClientHTTP must satisfy outbound.SkillsClient.
var _ outbound.SkillsClient = (*SkillsClientHTTP)(nil)
