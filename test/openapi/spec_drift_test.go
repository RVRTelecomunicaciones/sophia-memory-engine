// Package openapi contains drift-detection tests that assert the hand-written
// OpenAPI spec stays in sync with the live chi router.
//
// Run with:
//
//	go test ./test/openapi/... -count=1
//	make openapi-validate
package openapi

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/sophia-engine/memory-engine/internal/adapters/inbound/http"
	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// specPath resolves the OpenAPI file relative to this test file so the test
// works regardless of the working directory it is invoked from.
func specPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// test/openapi/ → ../../api/openapi/memory-engine.yaml
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "api", "openapi", "memory-engine.yaml")
}

// TestOpenAPISpec_Loads verifies that the YAML file parses and passes
// kin-openapi's built-in OpenAPI 3 validation.
func TestOpenAPISpec_Loads(t *testing.T) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile(specPath(t))
	require.NoError(t, err, "failed to parse api/openapi/memory-engine.yaml")

	err = spec.Validate(context.Background())
	require.NoError(t, err, "OpenAPI spec validation failed")
}

// --- Minimal stubs so NewRouter can be instantiated without real backends ---
// These types only need to satisfy the interface; their methods are never called.

type noopMemorySvc struct{}

func (noopMemorySvc) Ingest(_ context.Context, _ inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error) {
	return nil, nil
}
func (noopMemorySvc) Get(_ context.Context, _ shared.RecordID) (*memory.MemoryRecord, error) {
	return nil, nil
}
func (noopMemorySvc) GetByTopicKey(_ context.Context, _, _ string) (*memory.MemoryRecord, error) {
	return nil, nil
}
func (noopMemorySvc) Archive(_ context.Context, _ inbound.ArchiveMemoryCmd) error { return nil }

type noopDecisionSvc struct{}

func (noopDecisionSvc) Record(_ context.Context, _ inbound.RecordDecisionCmd) (*inbound.RecordDecisionResult, error) {
	return nil, nil
}
func (noopDecisionSvc) Get(_ context.Context, _ shared.RecordID) (*decision.Decision, error) {
	return nil, nil
}
func (noopDecisionSvc) GetHistory(_ context.Context, _ inbound.DecisionHistoryQuery) ([]decision.Decision, error) {
	return nil, nil
}
func (noopDecisionSvc) Contradict(_ context.Context, _ inbound.ContradictDecisionCmd) error {
	return nil
}

type noopHeuristicSvc struct{}

func (noopHeuristicSvc) Create(_ context.Context, _ inbound.CreateHeuristicCmd) (*inbound.CreateHeuristicResult, error) {
	return nil, nil
}
func (noopHeuristicSvc) GetActive(_ context.Context, _ inbound.GetActiveHeuristicQuery) (*heuristic.HeuristicRule, error) {
	return nil, nil
}
func (noopHeuristicSvc) ListByScope(_ context.Context, _ inbound.ListHeuristicsQuery) ([]heuristic.HeuristicRule, error) {
	return nil, nil
}
func (noopHeuristicSvc) Toggle(_ context.Context, _ inbound.ToggleHeuristicCmd) error { return nil }

type noopRelationSvc struct{}

func (noopRelationSvc) Create(_ context.Context, _ inbound.CreateRelationCmd) (*inbound.CreateRelationResult, error) {
	return nil, nil
}
func (noopRelationSvc) GetFrom(_ context.Context, _ inbound.RelationQuery) ([]inbound.RelationResult, error) {
	return nil, nil
}
func (noopRelationSvc) GetTo(_ context.Context, _ inbound.RelationQuery) ([]inbound.RelationResult, error) {
	return nil, nil
}

type noopPurgeSvc struct{}

func (noopPurgeSvc) Request(_ context.Context, _ inbound.RequestPurgeCmd) (*purge.PurgeRecord, error) {
	return nil, nil
}
func (noopPurgeSvc) Execute(_ context.Context, _ inbound.ExecutePurgeCmd) (*purge.PurgeRecord, error) {
	return nil, nil
}

type noopFeedbackSvc struct{}

func (noopFeedbackSvc) Submit(_ context.Context, _ inbound.SubmitFeedbackCmd) (*shared.RetrievalFeedback, error) {
	return nil, nil
}

type noopAuthSvc struct{}

func (noopAuthSvc) Authenticate(_ context.Context, _ string) (auth.AuthContext, error) {
	return auth.AuthContext{}, nil
}

type noopDBPinger struct{}

func (noopDBPinger) Ping(_ context.Context) error { return nil }

// buildLiveRouter constructs the real chi router with all-noop dependencies.
// No HTTP traffic is served; we only interrogate the route tree.
func buildLiveRouter() chi.Router {
	return httpAdapter.NewRouter(
		noopMemorySvc{},
		noopDecisionSvc{},
		noopHeuristicSvc{},
		noopRelationSvc{},
		// SearchService and ContextBuilder are concrete structs. We pass nil
		// pointers so the router wires up without panic; no requests are served.
		(*retrieval.SearchService)(nil),
		(*retrieval.ContextBuilder)(nil),
		noopPurgeSvc{},
		noopFeedbackSvc{},
		noopAuthSvc{},
		noopDBPinger{},
	)
}

// routeKey is a normalised "METHOD /path/pattern" pair for set operations.
type routeKey struct {
	method  string
	pattern string
}

func (k routeKey) String() string { return k.method + " " + k.pattern }

// collectRouterRoutes walks the chi router and returns every registered route.
// chi patterns already use {param} syntax which matches OpenAPI.
func collectRouterRoutes(t *testing.T, r chi.Router) map[routeKey]struct{} {
	t.Helper()
	routes := make(map[routeKey]struct{})
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// chi sometimes appends a wildcard suffix for mounts; strip it.
		route = strings.TrimSuffix(route, "/*")
		routes[routeKey{method: method, pattern: route}] = struct{}{}
		return nil
	})
	require.NoError(t, err, "chi.Walk failed")
	return routes
}

// collectSpecRoutes returns all (METHOD, path) pairs defined in the OpenAPI spec.
func collectSpecRoutes(t *testing.T, spec *openapi3.T) map[routeKey]struct{} {
	t.Helper()
	routes := make(map[routeKey]struct{})
	for path, item := range spec.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			routes[routeKey{method: strings.ToUpper(method), pattern: path}] = struct{}{}
		}
	}
	return routes
}

// sortedKeys returns the string representation of all keys, sorted.
func sortedKeys(m map[routeKey]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k.String())
	}
	sort.Strings(out)
	return out
}

// TestOpenAPISpec_MatchesChiRouter asserts that every route in the chi router
// has a corresponding entry in the OpenAPI spec and vice versa.
//
// When the test fails it prints:
//   - Routes in the router that are missing from the spec
//   - Paths in the spec that are missing from the router
//
// Fix the spec (api/openapi/memory-engine.yaml) — do not silence the test.
func TestOpenAPISpec_MatchesChiRouter(t *testing.T) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile(specPath(t))
	require.NoError(t, err, "failed to load spec")

	router := buildLiveRouter()
	routerRoutes := collectRouterRoutes(t, router)
	specRoutes := collectSpecRoutes(t, spec)

	var missingInSpec []string
	for k := range routerRoutes {
		if _, ok := specRoutes[k]; !ok {
			missingInSpec = append(missingInSpec, k.String())
		}
	}

	var missingInRouter []string
	for k := range specRoutes {
		if _, ok := routerRoutes[k]; !ok {
			missingInRouter = append(missingInRouter, k.String())
		}
	}

	sort.Strings(missingInSpec)
	sort.Strings(missingInRouter)

	if len(missingInSpec) == 0 && len(missingInRouter) == 0 {
		t.Logf("OK: router (%d routes) and spec (%d paths) are in sync", len(routerRoutes), len(specRoutes))
		return
	}

	var sb strings.Builder
	if len(missingInSpec) > 0 {
		fmt.Fprintf(&sb, "\nRoutes registered in chi router but MISSING from OpenAPI spec (%d):\n", len(missingInSpec))
		for _, r := range missingInSpec {
			fmt.Fprintf(&sb, "  - %s\n", r)
		}
	}
	if len(missingInRouter) > 0 {
		fmt.Fprintf(&sb, "\nPaths in OpenAPI spec but NOT registered in chi router (%d):\n", len(missingInRouter))
		for _, r := range missingInRouter {
			fmt.Fprintf(&sb, "  - %s\n", r)
		}
	}
	fmt.Fprintf(&sb, "\nAll router routes (%d):\n", len(routerRoutes))
	for _, r := range sortedKeys(routerRoutes) {
		fmt.Fprintf(&sb, "  %s\n", r)
	}
	fmt.Fprintf(&sb, "\nAll spec paths (%d):\n", len(specRoutes))
	for _, r := range sortedKeys(specRoutes) {
		fmt.Fprintf(&sb, "  %s\n", r)
	}
	t.Fatal(sb.String())
}
