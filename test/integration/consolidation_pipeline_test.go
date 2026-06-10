//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/orchhttp"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/search"
	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/application/ingest"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/events"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOrchServer builds a httptest server that handles the orch skills API.
// patchMetricsCalls tracks how many times PATCH /metrics is called per skill.
// patchStatusCalls tracks PATCH /status calls.
// getUsageRows is returned by GET /usage.
type fakeOrchServer struct {
	patchMetricsCalls int32 // atomic counter
	patchStatusCalls  int32
	getUsageRows      []outbound.SkillUsageRow
	// skill snapshot returned by GET /api/v1/skills/{id}
	skillSnap map[string]outbound.SkillSnapshot
	// simulate errors: key=skillID, value=statusCode to return for PatchMetrics
	patchMetricsErrors map[string]int
}

func (f *fakeOrchServer) handler() http.Handler {
	mux := http.NewServeMux()

	// GET /api/v1/skills/usage?change_id=...
	mux.HandleFunc("/api/v1/skills/usage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"items": f.getUsageRows}) //nolint:errcheck
	})

	// PATCH /api/v1/skills/{id}/metrics and /status
	mux.HandleFunc("/api/v1/skills/", func(w http.ResponseWriter, r *http.Request) {
		// Route based on path suffix.
		switch {
		case len(r.URL.Path) > 0 && r.URL.Path[len(r.URL.Path)-len("/metrics"):] == "/metrics":
			if r.Method != http.MethodPatch {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Extract skill_id from path: /api/v1/skills/{id}/metrics
			parts := splitSkillPath(r.URL.Path)
			if len(parts) >= 1 {
				skillID := parts[0]
				if code, ok := f.patchMetricsErrors[skillID]; ok {
					w.WriteHeader(code)
					return
				}
			}
			atomic.AddInt32(&f.patchMetricsCalls, 1)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck

		case len(r.URL.Path) > 0 && r.URL.Path[len(r.URL.Path)-len("/status"):] == "/status":
			if r.Method != http.MethodPatch {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			atomic.AddInt32(&f.patchStatusCalls, 1)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck

		default:
			// GET /api/v1/skills/{id}
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Extract skill_id from path: /api/v1/skills/{id}
			parts := splitSkillPath(r.URL.Path)
			snap := outbound.SkillSnapshot{
				SkillID:   "unknown",
				Status:    "candidate",
				RiskLevel: "low",
				Version:   "v1",
			}
			if len(parts) >= 1 {
				if s, ok := f.skillSnap[parts[0]]; ok {
					snap = s
				} else {
					snap.SkillID = parts[0]
				}
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(snap) //nolint:errcheck
		}
	})

	return mux
}

// splitSkillPath extracts the skill_id from paths like /api/v1/skills/{id} or /api/v1/skills/{id}/metrics.
func splitSkillPath(path string) []string {
	// path = /api/v1/skills/<id>[/<action>]
	prefix := "/api/v1/skills/"
	if len(path) <= len(prefix) {
		return nil
	}
	rest := path[len(prefix):]
	for i, ch := range rest {
		if ch == '/' {
			return []string{rest[:i], rest[i+1:]}
		}
	}
	return []string{rest}
}

// pipelineEnv bundles what integration tests need to inspect after Handle().
type pipelineEnv struct {
	handler *consolidation.HandlerV2
	memRepo *persistence.MemoryPgRepository
	scope   shared.Scope
	cleanup func()
}

// buildPipeline creates a HandlerV2 with real memory client + fake orch httptest server.
func buildPipeline(t *testing.T, fakeOrch *fakeOrchServer) pipelineEnv {
	t.Helper()

	pool := testhelper.SetupTestDB(t)
	truncateMemoriesAfter(t, pool)

	// Memory service stack.
	memRepo := persistence.NewMemoryPgRepository(pool)
	searchIdx := search.NewPostgresFTSIndex(pool)
	eventPub := events.NewInProcessEventPublisher()
	clock := shared.RealClock{}

	memorySvc := ingest.NewService(memRepo, searchIdx, eventPub, clock)
	memClient := consolidation.NewMemoryServiceClient(memorySvc, "test-project", clock)

	scope, _ := shared.NewScope("test-project")

	// Skills HTTP client.
	srv := httptest.NewServer(fakeOrch.handler())
	skillsClient, err := orchhttp.NewSkillsClientWithOptions(srv.URL, "test-key", orchhttp.Options{
		MaxRetries:    1,
		BaseBackoff:   1 * time.Millisecond,
		BackoffFactor: 1.0,
	})
	require.NoError(t, err)

	h := consolidation.NewHandlerV2(slog.Default(), clock, memClient, skillsClient)
	return pipelineEnv{
		handler: h,
		memRepo: memRepo,
		scope:   scope,
		cleanup: func() { srv.Close() },
	}
}

// ---------------------------------------------------------------------------
// N.1: full happy path — Handle 1 skill, no existing digest;
//      assert GetUsage called → PatchMetrics called → digest persisted
// ---------------------------------------------------------------------------

func TestConsolidationPipeline_HappyPath(t *testing.T) {
	fake := &fakeOrchServer{
		getUsageRows: []outbound.SkillUsageRow{
			{
				SkillUsageID:  "row-1",
				ChangeID:      "change-happy",
				PhaseType:    "apply",
				SkillID:       "skill-happy",
				SkillVersion:  "v1",
				Outcome:       "success",
				ApplyAttempts: 1,
			},
		},
		skillSnap: map[string]outbound.SkillSnapshot{
			"skill-happy": {
				SkillID:   "skill-happy",
				Status:    "candidate",
				RiskLevel: "low",
				Version:   "v1",
				Metrics:   outbound.SkillMetrics{UsageCount: 1, SuccessCount: 1},
			},
		},
	}

	env := buildPipeline(t, fake)
	defer env.cleanup()

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-happy",
		ChangeName: "happy-feature",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}

	err := env.handler.Handle(context.Background(), payload)
	require.NoError(t, err)

	// PatchMetrics must have been called once (one skill).
	assert.Equal(t, int32(1), atomic.LoadInt32(&fake.patchMetricsCalls),
		"PatchMetrics must be called for each skill")

	// Digest must be persisted in memory-engine.
	rec, err := env.memRepo.FindLatestActiveByTopicKey(context.Background(), env.scope, "digest/change-happy")
	require.NoError(t, err, "digest must exist in memory after pipeline")
	assert.NotNil(t, rec)
}

// ---------------------------------------------------------------------------
// N.2: PatchMetrics fails for skill A (orch returns 500); pipeline continues
//      for skill B; digest still persisted
// ---------------------------------------------------------------------------

func TestConsolidationPipeline_PartialFailure_ContinuesAndPersistsDigest(t *testing.T) {
	fake := &fakeOrchServer{
		getUsageRows: []outbound.SkillUsageRow{
			{SkillUsageID: "row-a", ChangeID: "change-partial", SkillID: "skill-A", Outcome: "success", ApplyAttempts: 1},
			{SkillUsageID: "row-b", ChangeID: "change-partial", SkillID: "skill-B", Outcome: "success", ApplyAttempts: 1},
		},
		skillSnap:          map[string]outbound.SkillSnapshot{},
		patchMetricsErrors: map[string]int{"skill-A": http.StatusInternalServerError},
	}

	env := buildPipeline(t, fake)
	defer env.cleanup()

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-partial",
		ChangeName: "partial-feature",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}

	err := env.handler.Handle(context.Background(), payload)
	require.NoError(t, err, "pipeline must not fail when one skill fails")

	// skill-B must be processed (1 successful PatchMetrics call for skill-B);
	// skill-A exhausted its retries (1 attempt with MaxRetries=1 means 1 attempt total).
	calls := atomic.LoadInt32(&fake.patchMetricsCalls)
	assert.Equal(t, int32(1), calls,
		"only skill-B PatchMetrics should succeed; skill-A returns 500 and is skipped")

	// Digest must still be persisted despite skill-A failure.
	rec, err := env.memRepo.FindLatestActiveByTopicKey(context.Background(), env.scope, "digest/change-partial")
	require.NoError(t, err, "digest must persist even when a skill fails")
	assert.NotNil(t, rec)
}

// ---------------------------------------------------------------------------
// N.3: re-receive same change_id → second Handle is no-op; no metrics calls
// ---------------------------------------------------------------------------

func TestConsolidationPipeline_ReReceive_NoOp(t *testing.T) {
	fake := &fakeOrchServer{
		getUsageRows: []outbound.SkillUsageRow{
			{SkillUsageID: "row-1", ChangeID: "change-idem", SkillID: "skill-X", Outcome: "success", ApplyAttempts: 1},
		},
		skillSnap: map[string]outbound.SkillSnapshot{},
	}

	env := buildPipeline(t, fake)
	defer env.cleanup()

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-idem",
		ChangeName: "idempotent-feature",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}

	// First invocation — must process and write digest.
	require.NoError(t, env.handler.Handle(context.Background(), payload))
	firstCount := atomic.LoadInt32(&fake.patchMetricsCalls)

	// Second invocation — idempotency guard must skip; no additional PatchMetrics calls.
	require.NoError(t, env.handler.Handle(context.Background(), payload))
	secondCount := atomic.LoadInt32(&fake.patchMetricsCalls)

	assert.Equal(t, firstCount, secondCount,
		"second invocation must not call PatchMetrics (idempotency guard)")
}
