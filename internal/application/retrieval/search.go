package retrieval

import (
	"context"
	"sort"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// SearchService implements the Search method of inbound.RetrievalService.
// It does not yet implement BuildContext (that requires a separate vertical slice).
type SearchService struct {
	searchIdx outbound.SearchIndex
	memRepo   outbound.MemoryRepository
	config    config.RetrievalConfig
	clock     shared.Clock
}

// NewSearchService creates a new SearchService with the given dependencies.
func NewSearchService(
	searchIdx outbound.SearchIndex,
	memRepo outbound.MemoryRepository,
	cfg config.RetrievalConfig,
	clock shared.Clock,
) *SearchService {
	return &SearchService{
		searchIdx: searchIdx,
		memRepo:   memRepo,
		config:    cfg,
		clock:     clock,
	}
}

// Search performs a hybrid search: FTS retrieval + composite ranking.
func (s *SearchService) Search(ctx context.Context, query inbound.SearchQuery) (*inbound.SearchResults, error) {
	limit := s.config.Thresholds.DefaultResults
	if query.Limit != nil {
		limit = *query.Limit
	}
	if limit > s.config.Thresholds.MaxResults {
		limit = s.config.Thresholds.MaxResults
	}

	offset := 0
	if query.Offset != nil {
		offset = *query.Offset
	}

	ftsQuery := outbound.FTSQuery{
		Text:      query.Query,
		Scope:     query.Scope,
		Types:     query.Types,
		TimeRange: query.TimeRange,
		Limit:     limit,
		Offset:    offset,
	}

	ftsResults, err := s.searchIdx.Search(ctx, ftsQuery)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()

	type scoredResult struct {
		fts     outbound.FTSResult
		signals RankingSignals
		final   float64
	}

	scored := make([]scoredResult, 0, len(ftsResults))
	for _, r := range ftsResults {
		// For the vertical slice, we use placeholder values for signals
		// that require additional data (trigram, importance, type boost, freshness, scope).
		// CreatedAt is approximated from the ULID timestamp embedded in the RecordID.
		recencyBoost := ComputeRecencyBoost(r.ID.Time(), now)

		signals := RankingSignals{
			FTSScore:        r.Rank,
			TRGMScore:       0.0, // placeholder — will come from PG similarity()
			RecencyBoost:    recencyBoost,
			ImportanceScore: 0.5, // default — will load from record later
			TypeBoost:       0.5, // memories default (decisions=0.8, heuristics=0.7)
			FreshnessBoost:  1.0, // fresh default
			ScopeExactness:  1.0, // exact project match assumed
		}

		final := ComputeFinalScore(s.config.Weights, signals)

		scored = append(scored, scoredResult{
			fts:     r,
			signals: signals,
			final:   final,
		})
	}

	// Sort by final score descending.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].final > scored[j].final
	})

	results := make([]inbound.SearchResult, 0, len(scored))
	for _, sr := range scored {
		results = append(results, inbound.SearchResult{
			ID:         sr.fts.ID,
			RecordType: sr.fts.RecordType,
			Snippet:    sr.fts.Snippet,
			Score:      sr.final,
			Ranking: inbound.RankingExplanation{
				FTSScore:        sr.signals.FTSScore,
				TRGMScore:       sr.signals.TRGMScore,
				RecencyBoost:    sr.signals.RecencyBoost,
				ImportanceScore: sr.signals.ImportanceScore,
				TypeBoost:       sr.signals.TypeBoost,
				FreshnessBoost:  sr.signals.FreshnessBoost,
				ScopeExactness:  sr.signals.ScopeExactness,
				FinalScore:      sr.final,
			},
			Scope:     query.Scope,
			Freshness: shared.FreshnessLevelFresh,
			CreatedAt: sr.fts.ID.Time(),
		})
	}

	return &inbound.SearchResults{
		Results:    results,
		TotalCount: len(results),
		Query:      query.Query,
		Scope:      query.Scope,
	}, nil
}
