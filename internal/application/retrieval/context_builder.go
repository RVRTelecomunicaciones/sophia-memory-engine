package retrieval

import (
	"context"
	"sort"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// ContextBuilder assembles structured context bundles for agents within a token budget.
// It fetches decisions and heuristics by scope (always included), memories by FTS query,
// and optionally expands the graph to include related records.
type ContextBuilder struct {
	searchIdx outbound.SearchIndex
	memRepo   outbound.MemoryRepository
	decRepo   outbound.DecisionRepository
	heurRepo  outbound.HeuristicRepository
	relRepo   outbound.RelationRepository
	config    config.RetrievalConfig
	clock     shared.Clock
}

// NewContextBuilder creates a new ContextBuilder with the given dependencies.
func NewContextBuilder(
	searchIdx outbound.SearchIndex,
	memRepo outbound.MemoryRepository,
	decRepo outbound.DecisionRepository,
	heurRepo outbound.HeuristicRepository,
	relRepo outbound.RelationRepository,
	cfg config.RetrievalConfig,
	clock shared.Clock,
) *ContextBuilder {
	return &ContextBuilder{
		searchIdx: searchIdx,
		memRepo:   memRepo,
		decRepo:   decRepo,
		heurRepo:  heurRepo,
		relRepo:   relRepo,
		config:    cfg,
		clock:     clock,
	}
}

// sectionBudgets holds the token allocation per context section.
type sectionBudgets struct {
	decisions int
	heuristics int
	episodic   int
	semantic   int
	related    int
}

// BuildContext assembles a structured context package for an agent within a token budget.
func (b *ContextBuilder) BuildContext(ctx context.Context, req inbound.ContextRequest) (*inbound.ContextBundle, error) {
	start := time.Now()

	maxTokens := b.config.ContextBudget.DefaultMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	expandGraph := true
	if req.ExpandGraph != nil {
		expandGraph = *req.ExpandGraph
	}

	budget := b.allocateBudget(maxTokens)

	var (
		scanned   int
		included  int
		truncated bool
	)

	// 1. Fetch active decisions (always, regardless of query).
	decSection, decScanned, decTrunc := b.fetchDecisions(ctx, req.Scope, budget.decisions)
	scanned += decScanned
	included += len(decSection.Records)
	truncated = truncated || decTrunc

	// 2. Fetch enabled heuristics (always, regardless of query).
	heurSection, heurScanned, heurTrunc := b.fetchHeuristics(ctx, req.Scope, budget.heuristics)
	scanned += heurScanned
	included += len(heurSection.Records)
	truncated = truncated || heurTrunc

	// 3. Fetch memories via FTS (only if query provided).
	var episodicSection inbound.ContextSection
	if req.Query != nil && *req.Query != "" {
		// Redistribute unused budget from decisions and heuristics.
		unusedDecBudget := budget.decisions - decSection.TokenCount
		unusedHeurBudget := budget.heuristics - heurSection.TokenCount
		extraBudget := 0
		if unusedDecBudget > 0 {
			extraBudget += unusedDecBudget
		}
		if unusedHeurBudget > 0 {
			extraBudget += unusedHeurBudget
		}

		episodicBudget := budget.episodic + budget.semantic + extraBudget
		var memScanned int
		var memTrunc bool
		episodicSection, memScanned, memTrunc = b.fetchMemories(ctx, req.Scope, *req.Query, episodicBudget)
		scanned += memScanned
		included += len(episodicSection.Records)
		truncated = truncated || memTrunc
	}

	// 4. Graph expansion (optional).
	var relatedSection inbound.ContextSection
	if expandGraph {
		topIDs := b.collectTopRecordIDs(decSection, heurSection, episodicSection)

		// Redistribute unused budget if no query was provided.
		relatedBudget := budget.related
		if req.Query == nil || *req.Query == "" {
			unusedEpisodic := budget.episodic + budget.semantic
			unusedDecBudget := budget.decisions - decSection.TokenCount
			unusedHeurBudget := budget.heuristics - heurSection.TokenCount
			extra := unusedEpisodic
			if unusedDecBudget > 0 {
				extra += unusedDecBudget
			}
			if unusedHeurBudget > 0 {
				extra += unusedHeurBudget
			}
			relatedBudget += extra
		}

		var relScanned int
		relatedSection, relScanned = b.expandGraph(ctx, topIDs, relatedBudget)
		scanned += relScanned
		included += len(relatedSection.Records)
	}

	// 5. Assemble sections.
	sections := make([]inbound.ContextSection, 0, 4)
	if len(decSection.Records) > 0 {
		sections = append(sections, decSection)
	}
	if len(heurSection.Records) > 0 {
		sections = append(sections, heurSection)
	}
	if len(episodicSection.Records) > 0 {
		sections = append(sections, episodicSection)
	}
	if len(relatedSection.Records) > 0 {
		sections = append(sections, relatedSection)
	}

	totalTokens := 0
	for _, s := range sections {
		totalTokens += s.TokenCount
	}

	return &inbound.ContextBundle{
		Sections:    sections,
		TotalTokens: totalTokens,
		Truncated:   truncated,
		GeneratedAt: b.clock.Now(),
		ScopeUsed:   req.Scope,
		DebugInfo: &inbound.ContextDebugInfo{
			Strategy:        "fts+scope+graph",
			RecordsScanned:  scanned,
			RecordsIncluded: included,
			GraphExpanded:   expandGraph,
			GraphDepth:      b.config.ContextBudget.MaxGraphExpandDepth,
			TokenBudgetUsed: totalTokens,
			DurationMs:      time.Since(start).Milliseconds(),
		},
	}, nil
}

// allocateBudget divides the total token budget across sections using config percentages.
func (b *ContextBuilder) allocateBudget(maxTokens int) sectionBudgets {
	cb := b.config.ContextBudget
	return sectionBudgets{
		decisions:  int(float64(maxTokens) * cb.DecisionsPct),
		heuristics: int(float64(maxTokens) * cb.HeuristicsPct),
		episodic:   int(float64(maxTokens) * cb.RecentEpisodicPct),
		semantic:   int(float64(maxTokens) * cb.SemanticPct),
		related:    int(float64(maxTokens) * cb.RelatedPct),
	}
}

// estimateTokens returns an approximate token count for a string (~4 chars per token).
func estimateTokens(content string) int {
	if len(content) == 0 {
		return 0
	}
	tokens := len(content) / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// compactContent returns a budget-fitting string: prefers Summary, truncates Content if needed.
func compactContent(content string, summary *string, tokenBudget int) string {
	if summary != nil && *summary != "" {
		s := *summary
		if estimateTokens(s) <= tokenBudget {
			return s
		}
		// Truncate summary to fit.
		maxChars := tokenBudget * 4
		if maxChars < len(s) {
			return s[:maxChars]
		}
		return s
	}

	if estimateTokens(content) <= tokenBudget {
		return content
	}

	maxChars := tokenBudget * 4
	if maxChars < len(content) {
		return content[:maxChars]
	}
	return content
}

// fetchDecisions retrieves all active decisions by scope and compacts them into a section.
func (b *ContextBuilder) fetchDecisions(ctx context.Context, scope shared.Scope, tokenBudget int) (inbound.ContextSection, int, bool) {
	section := inbound.ContextSection{Type: "decisions"}

	decisions, err := b.decRepo.FindActiveByScope(ctx, scope)
	if err != nil {
		return section, 0, false
	}

	scanned := len(decisions)
	usedTokens := 0
	wasTruncated := false

	for _, d := range decisions {
		content := d.Title + ": " + d.Rationale
		available := tokenBudget - usedTokens
		if available <= 0 {
			wasTruncated = true
			break
		}

		compacted := compactContent(content, nil, available)
		tokens := estimateTokens(compacted)

		section.Records = append(section.Records, inbound.ContextRecord{
			ID:      d.ID,
			Type:    "decision",
			Content: compacted,
			Score:   1.0, // Decisions always included, max priority.
		})
		usedTokens += tokens
	}

	section.TokenCount = usedTokens
	return section, scanned, wasTruncated
}

// fetchHeuristics retrieves all enabled heuristics by scope and compacts them into a section.
func (b *ContextBuilder) fetchHeuristics(ctx context.Context, scope shared.Scope, tokenBudget int) (inbound.ContextSection, int, bool) {
	section := inbound.ContextSection{Type: "heuristics"}

	enabled := true
	rules, err := b.heurRepo.FindByScope(ctx, scope, &enabled)
	if err != nil {
		return section, 0, false
	}

	scanned := len(rules)
	usedTokens := 0
	wasTruncated := false

	for _, h := range rules {
		content := h.Condition + " → " + h.Action
		available := tokenBudget - usedTokens
		if available <= 0 {
			wasTruncated = true
			break
		}

		compacted := compactContent(content, nil, available)
		tokens := estimateTokens(compacted)

		section.Records = append(section.Records, inbound.ContextRecord{
			ID:      h.ID,
			Type:    "heuristic",
			Content: compacted,
			Score:   1.0, // Heuristics always included, max priority.
		})
		usedTokens += tokens
	}

	section.TokenCount = usedTokens
	return section, scanned, wasTruncated
}

// fetchMemories searches via FTS and returns all results in a "recent_episodic" section.
// Phase 2 can split into episodic vs semantic by memory type.
func (b *ContextBuilder) fetchMemories(ctx context.Context, scope shared.Scope, query string, tokenBudget int) (inbound.ContextSection, int, bool) {
	section := inbound.ContextSection{Type: "recent_episodic"}

	ftsQuery := outbound.FTSQuery{
		Text:  query,
		Scope: scope,
		Limit: b.config.Thresholds.MaxResults,
	}

	ftsResults, err := b.searchIdx.Search(ctx, ftsQuery)
	if err != nil {
		return section, 0, false
	}

	scanned := len(ftsResults)
	now := b.clock.Now()

	// Score and sort results.
	type scoredFTS struct {
		result outbound.FTSResult
		score  float64
	}

	scored := make([]scoredFTS, 0, len(ftsResults))
	for _, r := range ftsResults {
		recency := ComputeRecencyBoost(r.ID.Time(), now)
		score := ComputeFinalScore(b.config.Weights, RankingSignals{
			FTSScore:        r.Rank,
			TRGMScore:       r.TrigramScore,
			RecencyBoost:    recency,
			ImportanceScore: 0.5,
			TypeBoost:       0.5,
			FreshnessBoost:  1.0,
			ScopeExactness:  1.0,
		})
		scored = append(scored, scoredFTS{result: r, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	usedTokens := 0
	wasTruncated := false
	for _, sr := range scored {
		available := tokenBudget - usedTokens
		if available <= 0 {
			wasTruncated = true
			break
		}

		content := sr.result.Snippet
		compacted := compactContent(content, nil, available)
		tokens := estimateTokens(compacted)

		section.Records = append(section.Records, inbound.ContextRecord{
			ID:      sr.result.ID,
			Type:    sr.result.RecordType,
			Content: compacted,
			Score:   sr.score,
		})
		usedTokens += tokens
	}

	section.TokenCount = usedTokens
	return section, scanned, wasTruncated
}

// collectTopRecordIDs gathers the top N record IDs from all sections for graph expansion.
func (b *ContextBuilder) collectTopRecordIDs(sections ...inbound.ContextSection) []shared.RecordID {
	maxCount := b.config.ContextBudget.MaxGraphExpandCount

	type scoredID struct {
		id    shared.RecordID
		score float64
	}

	var all []scoredID
	for _, s := range sections {
		for _, r := range s.Records {
			all = append(all, scoredID{id: r.ID, score: r.Score})
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].score > all[j].score
	})

	if len(all) > maxCount {
		all = all[:maxCount]
	}

	ids := make([]shared.RecordID, len(all))
	for i, s := range all {
		ids[i] = s.id
	}
	return ids
}

// expandGraph traverses depth-1 outbound relations from the given record IDs.
func (b *ContextBuilder) expandGraph(ctx context.Context, startIDs []shared.RecordID, tokenBudget int) (inbound.ContextSection, int) {
	section := inbound.ContextSection{Type: "related"}

	if len(startIDs) == 0 {
		return section, 0
	}

	scanned := 0
	usedTokens := 0

	// Track seen IDs to avoid duplicates.
	seen := make(map[string]bool, len(startIDs))
	for _, id := range startIDs {
		seen[id.String()] = true
	}

	for _, startID := range startIDs {
		if usedTokens >= tokenBudget {
			break
		}

		tq := outbound.TraverseQuery{
			StartID:   startID,
			Direction: outbound.TraverseOutbound,
			MaxDepth:  b.config.ContextBudget.MaxGraphExpandDepth,
		}

		results, err := b.relRepo.Traverse(ctx, tq)
		if err != nil {
			continue
		}

		scanned += len(results)

		for _, tr := range results {
			targetStr := tr.Relation.TargetID.String()
			if seen[targetStr] {
				continue
			}
			seen[targetStr] = true

			available := tokenBudget - usedTokens
			if available <= 0 {
				break
			}

			relType := string(tr.Relation.Type)
			content := "related via " + relType
			tokens := estimateTokens(content)

			section.Records = append(section.Records, inbound.ContextRecord{
				ID:       tr.Relation.TargetID,
				Type:     "relation",
				Content:  content,
				Score:    0.5,
				Relation: &relType,
			})
			usedTokens += tokens
		}
	}

	section.TokenCount = usedTokens
	return section, scanned
}
