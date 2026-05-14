package retrieval_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// --- Mocks ---

type mockSearchIndex struct {
	results []outbound.FTSResult
	err     error
	indexed []outbound.SearchEntry
}

func (m *mockSearchIndex) Index(_ context.Context, entry outbound.SearchEntry) error {
	m.indexed = append(m.indexed, entry)
	return nil
}

func (m *mockSearchIndex) Remove(_ context.Context, _ shared.RecordID) error {
	return nil
}

func (m *mockSearchIndex) Search(_ context.Context, _ outbound.FTSQuery) ([]outbound.FTSResult, error) {
	return m.results, m.err
}

type mockMemoryRepo struct {
	findResult *memory.MemoryRecord
	findErr    error
}

func (m *mockMemoryRepo) Save(_ context.Context, _ *memory.MemoryRecord) error { return nil }
func (m *mockMemoryRepo) FindByID(_ context.Context, _ shared.RecordID) (*memory.MemoryRecord, error) {
	return m.findResult, m.findErr
}
func (m *mockMemoryRepo) FindLatestActiveByTopicKey(_ context.Context, _ shared.Scope, _ string) (*memory.MemoryRecord, error) {
	return nil, shared.ErrNotFound
}
func (m *mockMemoryRepo) UpdateStatus(_ context.Context, _ shared.RecordID, _ shared.MemoryStatus) error {
	return nil
}
func (m *mockMemoryRepo) WipeContent(_ context.Context, _ shared.RecordID) error { return nil }

type mockCtxDecisionRepo struct {
	activeByScopeResult []decision.Decision
	activeByScopeErr    error
}

func (m *mockCtxDecisionRepo) Save(_ context.Context, _ *decision.Decision) error { return nil }
func (m *mockCtxDecisionRepo) FindByID(_ context.Context, _ shared.RecordID) (*decision.Decision, error) {
	return nil, shared.ErrNotFound
}
func (m *mockCtxDecisionRepo) FindActiveByKey(_ context.Context, _ string, _ shared.Scope) (*decision.Decision, error) {
	return nil, shared.ErrNotFound
}
func (m *mockCtxDecisionRepo) FindByKey(_ context.Context, _ string, _ shared.Scope) ([]decision.Decision, error) {
	return nil, nil
}
func (m *mockCtxDecisionRepo) FindActiveByScope(_ context.Context, _ shared.Scope) ([]decision.Decision, error) {
	return m.activeByScopeResult, m.activeByScopeErr
}
func (m *mockCtxDecisionRepo) UpdateStatus(_ context.Context, _ shared.RecordID, _ shared.DecisionStatus, _ *shared.RecordID) error {
	return nil
}

type mockCtxHeuristicRepo struct {
	findByScopeResult []heuristic.HeuristicRule
	findByScopeErr    error
}

func (m *mockCtxHeuristicRepo) Save(_ context.Context, _ *heuristic.HeuristicRule) error {
	return nil
}
func (m *mockCtxHeuristicRepo) FindByID(_ context.Context, _ shared.RecordID) (*heuristic.HeuristicRule, error) {
	return nil, shared.ErrNotFound
}
func (m *mockCtxHeuristicRepo) FindActiveByKey(_ context.Context, _ string, _ shared.Scope) (*heuristic.HeuristicRule, error) {
	return nil, shared.ErrNotFound
}
func (m *mockCtxHeuristicRepo) FindByScope(_ context.Context, _ shared.Scope, _ *bool) ([]heuristic.HeuristicRule, error) {
	return m.findByScopeResult, m.findByScopeErr
}
func (m *mockCtxHeuristicRepo) UpdateEnabled(_ context.Context, _ shared.RecordID, _ bool) error {
	return nil
}

type mockCtxRelationRepo struct {
	traverseResults []outbound.TraverseResult
	traverseErr     error
	traverseQueries []outbound.TraverseQuery
}

func (m *mockCtxRelationRepo) Save(_ context.Context, _ *relation.Relation) error { return nil }
func (m *mockCtxRelationRepo) FindFromSource(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}
func (m *mockCtxRelationRepo) FindToTarget(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, nil
}
func (m *mockCtxRelationRepo) Traverse(_ context.Context, q outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	m.traverseQueries = append(m.traverseQueries, q)
	return m.traverseResults, m.traverseErr
}
func (m *mockCtxRelationRepo) DeleteByTarget(_ context.Context, _ shared.RecordID) (int, error) {
	return 0, nil
}

// --- Helpers ---

func ctxTestScope(t *testing.T) shared.Scope {
	t.Helper()
	s, err := shared.NewScope("test-project")
	require.NoError(t, err)
	return s
}

func ctxTestConfig() config.RetrievalConfig {
	cfg := config.DefaultConfig()
	return cfg.Retrieval
}

func makeDecision(t *testing.T, clock shared.Clock, scope shared.Scope, key, title, rationale string) decision.Decision {
	t.Helper()
	uri := "https://example.com/doc"
	evidence, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &uri, nil)
	require.NoError(t, err)
	conf, err := shared.NewConfidence(0.9, shared.ConfidenceSourceHuman)
	require.NoError(t, err)
	prov, err := shared.NewProvenance("test", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	d, err := decision.NewDecision(key, title, "desc", rationale, []shared.EvidenceRef{evidence}, scope, prov, conf, clock, 1)
	require.NoError(t, err)
	return *d
}

func makeHeuristic(t *testing.T, clock shared.Clock, scope shared.Scope, key, condition, action string) heuristic.HeuristicRule {
	t.Helper()
	conf, err := shared.NewConfidence(0.8, shared.ConfidenceSourceHuman)
	require.NoError(t, err)
	prov, err := shared.NewProvenance("test", shared.IngestMethodDirect, nil)
	require.NoError(t, err)
	h, err := heuristic.NewHeuristicRule(key, condition, action, "because", scope, prov, conf, clock, 1)
	require.NoError(t, err)
	return *h
}

func newBuilder(
	searchIdx outbound.SearchIndex,
	memRepo outbound.MemoryRepository,
	decRepo outbound.DecisionRepository,
	heurRepo outbound.HeuristicRepository,
	relRepo outbound.RelationRepository,
	clock shared.Clock,
) *retrieval.ContextBuilder {
	return retrieval.NewContextBuilder(searchIdx, memRepo, decRepo, heurRepo, relRepo, ctxTestConfig(), clock)
}

// --- Tests ---

func TestBuildContext_IncludesActiveDecisions_AlwaysRegardlessOfQuery(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Industry standard")
	d2 := makeDecision(t, clock, scope, "db/postgres", "Use PostgreSQL", "Battle tested")
	d3 := makeDecision(t, clock, scope, "api/rest", "Use REST", "Simplicity")

	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1, d2, d3}}
	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, decRepo, &mockCtxHeuristicRepo{}, &mockCtxRelationRepo{}, clock)

	// No query provided — decisions should still be included.
	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	var decSection *inbound.ContextSection
	for i, s := range bundle.Sections {
		if s.Type == "decisions" {
			decSection = &bundle.Sections[i]
			break
		}
	}

	require.NotNil(t, decSection, "decisions section should be present")
	assert.Len(t, decSection.Records, 3)
}

func TestBuildContext_IncludesEnabledHeuristics_AlwaysRegardlessOfQuery(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	h1 := makeHeuristic(t, clock, scope, "naming/camel", "Go code", "Use camelCase")
	h2 := makeHeuristic(t, clock, scope, "error/wrap", "Error handling", "Always wrap errors")

	heurRepo := &mockCtxHeuristicRepo{findByScopeResult: []heuristic.HeuristicRule{h1, h2}}
	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, &mockCtxDecisionRepo{}, heurRepo, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	var heurSection *inbound.ContextSection
	for i, s := range bundle.Sections {
		if s.Type == "heuristics" {
			heurSection = &bundle.Sections[i]
			break
		}
	}

	require.NotNil(t, heurSection, "heuristics section should be present")
	assert.Len(t, heurSection.Records, 2)
}

func TestBuildContext_RespectsTokenBudget(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	// Create decisions with large content to exceed budget.
	longRationale := string(make([]byte, 2000)) // 2000 chars ≈ 500 tokens
	d1 := makeDecision(t, clock, scope, "key1", "Title1", longRationale)
	d2 := makeDecision(t, clock, scope, "key2", "Title2", longRationale)
	d3 := makeDecision(t, clock, scope, "key3", "Title3", longRationale)

	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1, d2, d3}}

	expandGraph := false
	maxTokens := 100
	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, decRepo, &mockCtxHeuristicRepo{}, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope:       scope,
		MaxTokens:   &maxTokens,
		ExpandGraph: &expandGraph,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)
	assert.LessOrEqual(t, bundle.TotalTokens, maxTokens)
}

func TestBuildContext_UsesSummaryWhenAvailable(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	// FTS results include a snippet — we check the content is included.
	ftsResult := outbound.FTSResult{
		ID:         shared.NewRecordID(),
		RecordType: "memory",
		Rank:       0.9,
		Snippet:    "This is the snippet summary",
	}

	searchIdx := &mockSearchIndex{results: []outbound.FTSResult{ftsResult}}
	expandGraph := false
	query := "test query"
	builder := newBuilder(searchIdx, &mockMemoryRepo{}, &mockCtxDecisionRepo{}, &mockCtxHeuristicRepo{}, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope:       scope,
		Query:       &query,
		ExpandGraph: &expandGraph,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	var episodicSection *inbound.ContextSection
	for i, s := range bundle.Sections {
		if s.Type == "recent_episodic" {
			episodicSection = &bundle.Sections[i]
			break
		}
	}

	require.NotNil(t, episodicSection)
	assert.Len(t, episodicSection.Records, 1)
	assert.Equal(t, "This is the snippet summary", episodicSection.Records[0].Content)
}

func TestBuildContext_GraphExpansion(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Standard")

	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1}}

	targetID1 := shared.NewRecordID()
	targetID2 := shared.NewRecordID()

	relRepo := &mockCtxRelationRepo{
		traverseResults: []outbound.TraverseResult{
			{
				Relation: relation.Relation{
					ID:       shared.NewRecordID(),
					SourceID: d1.ID,
					TargetID: targetID1,
					Type:     shared.RelationTypeSupersedes,
					Scope:    scope,
				},
				Depth: 1,
				Path:  []shared.RecordID{d1.ID, targetID1},
			},
			{
				Relation: relation.Relation{
					ID:       shared.NewRecordID(),
					SourceID: d1.ID,
					TargetID: targetID2,
					Type:     shared.RelationTypeDependsOn,
					Scope:    scope,
				},
				Depth: 1,
				Path:  []shared.RecordID{d1.ID, targetID2},
			},
		},
	}

	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, decRepo, &mockCtxHeuristicRepo{}, relRepo, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	var relatedSection *inbound.ContextSection
	for i, s := range bundle.Sections {
		if s.Type == "related" {
			relatedSection = &bundle.Sections[i]
			break
		}
	}

	require.NotNil(t, relatedSection, "related section should be present")
	assert.Len(t, relatedSection.Records, 2)
	assert.NotNil(t, relatedSection.Records[0].Relation)
}

func TestBuildContext_NoQuery_StillIncludesDecisionsAndHeuristics(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Standard")
	h1 := makeHeuristic(t, clock, scope, "naming/camel", "Go code", "Use camelCase")

	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1}}
	heurRepo := &mockCtxHeuristicRepo{findByScopeResult: []heuristic.HeuristicRule{h1}}

	expandGraph := false
	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, decRepo, heurRepo, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope:       scope,
		ExpandGraph: &expandGraph,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	// No episodic section (no query).
	hasEpisodic := false
	hasDecisions := false
	hasHeuristics := false
	for _, s := range bundle.Sections {
		switch s.Type {
		case "recent_episodic":
			hasEpisodic = true
		case "decisions":
			hasDecisions = true
		case "heuristics":
			hasHeuristics = true
		}
	}

	assert.True(t, hasDecisions, "decisions should be present")
	assert.True(t, hasHeuristics, "heuristics should be present")
	assert.False(t, hasEpisodic, "episodic should not be present without query")
}

func TestBuildContext_DebugInfo(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Standard")
	h1 := makeHeuristic(t, clock, scope, "naming/camel", "Go code", "Use camelCase")

	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1}}
	heurRepo := &mockCtxHeuristicRepo{findByScopeResult: []heuristic.HeuristicRule{h1}}

	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, decRepo, heurRepo, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.DebugInfo)

	assert.Equal(t, "fts+scope+graph", bundle.DebugInfo.Strategy)
	assert.Equal(t, 2, bundle.DebugInfo.RecordsScanned)  // 1 decision + 1 heuristic
	assert.Equal(t, 2, bundle.DebugInfo.RecordsIncluded) // Both included
	assert.True(t, bundle.DebugInfo.GraphExpanded)
	assert.Equal(t, 1, bundle.DebugInfo.GraphDepth)
	assert.GreaterOrEqual(t, bundle.DebugInfo.TokenBudgetUsed, 1)
	assert.GreaterOrEqual(t, bundle.DebugInfo.DurationMs, int64(0))
}

func TestBuildContext_EmptyScope_ReturnsEmptyBundle(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	expandGraph := false
	builder := newBuilder(&mockSearchIndex{}, &mockMemoryRepo{}, &mockCtxDecisionRepo{}, &mockCtxHeuristicRepo{}, &mockCtxRelationRepo{}, clock)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope:       scope,
		ExpandGraph: &expandGraph,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)
	assert.Empty(t, bundle.Sections)
	assert.Equal(t, 0, bundle.TotalTokens)
	assert.False(t, bundle.Truncated)
}

func TestBuildContext_GraphExpansion_RespectsFanoutConfig(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Standard")
	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1}}

	targetID := shared.NewRecordID()
	relRepo := &mockCtxRelationRepo{
		traverseResults: []outbound.TraverseResult{
			{
				Relation: relation.Relation{
					ID:       shared.NewRecordID(),
					SourceID: d1.ID,
					TargetID: targetID,
					Type:     shared.RelationTypeRelatesTo,
					Scope:    scope,
				},
				Depth: 1,
				Path:  []shared.RecordID{d1.ID, targetID},
			},
		},
	}

	// Use a custom config with specific fanout values.
	cfg := ctxTestConfig()
	cfg.ContextBudget.MaxFanoutPerNode = 5
	cfg.ContextBudget.MaxTotalExpanded = 25

	builder := retrieval.NewContextBuilder(
		&mockSearchIndex{}, &mockMemoryRepo{}, decRepo,
		&mockCtxHeuristicRepo{}, relRepo, cfg, clock,
	)

	bundle, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)

	// Verify that the TraverseQuery received the fanout config values.
	require.Len(t, relRepo.traverseQueries, 1, "expected exactly 1 traverse call")
	tq := relRepo.traverseQueries[0]
	assert.Equal(t, 5, tq.MaxFanoutPerNode, "MaxFanoutPerNode should come from config")
	assert.Equal(t, 25, tq.MaxTotalResults, "MaxTotalResults should come from config")
}

func TestBuildContext_GraphExpansion_ZeroFanoutMeansNoLimit(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC))
	scope := ctxTestScope(t)

	d1 := makeDecision(t, clock, scope, "auth/jwt", "Use JWT", "Standard")
	decRepo := &mockCtxDecisionRepo{activeByScopeResult: []decision.Decision{d1}}

	relRepo := &mockCtxRelationRepo{}

	// Explicitly set fanout to 0.
	cfg := ctxTestConfig()
	cfg.ContextBudget.MaxFanoutPerNode = 0
	cfg.ContextBudget.MaxTotalExpanded = 0

	builder := retrieval.NewContextBuilder(
		&mockSearchIndex{}, &mockMemoryRepo{}, decRepo,
		&mockCtxHeuristicRepo{}, relRepo, cfg, clock,
	)

	_, err := builder.BuildContext(context.Background(), inbound.ContextRequest{
		Scope: scope,
	})

	require.NoError(t, err)

	// Verify that zero values are passed through (repo layer handles defaults).
	require.Len(t, relRepo.traverseQueries, 1)
	tq := relRepo.traverseQueries[0]
	assert.Equal(t, 0, tq.MaxFanoutPerNode, "zero means no limit")
	assert.Equal(t, 0, tq.MaxTotalResults, "zero means use default")
}
