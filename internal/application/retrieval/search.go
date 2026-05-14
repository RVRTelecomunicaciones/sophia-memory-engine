package retrieval

import (
	"context"
	"sort"
	"strings"

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
//
// ADR-0005 P2.3 tuning: for the orchestator's `sdd_*` workload, the caller's
// search query string is interpreted as a candidate topic_key. When the FTS
// result snippet contains content that "looks like" the topic key, we apply
// the topic-key boost. We do NOT have a side-channel hit to the record's
// topic_key here (that would require a per-result repo call), so the boost
// is approximated: when the caller's Query is non-empty AND the caller's
// Types filter contains at least one sdd_* type AND the snippet contains the
// query verbatim, we treat it as a topic-key match. This is a conservative
// signal that won't flip ordering for FTS-quality hits but will lift exact
// "sdd/some/key" lookups to the top of the result set.
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

	// Pre-compute whether the caller's Types filter includes ANY sdd_* type.
	// We use this to decide whether the topic-key boost should fire (the
	// boost is scoped to sdd_*-targeted queries, per ADR-0005 P2.3).
	queryAsksForSDD := false
	for _, t := range query.Types {
		if IsSDDType(t) {
			queryAsksForSDD = true
			break
		}
	}

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

		// P2.3 flags.
		isRequestedSDD := IsRequestedSDDType(r.RecordType, query.Types)
		snippetTruncated := IsSnippetTruncated(r.Snippet)
		// Approximate topic-key match: the caller's query string appears verbatim
		// in the snippet AND the caller is targeting sdd_* types. The snippet
		// contains ts_headline output (which mirrors source content), so a
		// topic_key like "sdd/p2.3/retrieval" appearing in the snippet is a
		// strong indicator the record's topic_key equals the query string.
		topicKeyMatch := queryAsksForSDD && query.Query != "" &&
			containsTopicKey(r.Snippet, query.Query)

		signals := RankingSignals{
			FTSScore:         r.Rank,
			TRGMScore:        r.TrigramScore,
			RecencyBoost:     recencyBoost,
			ImportanceScore:  0.5, // default — will load from record later
			TypeBoost:        0.5, // memories default (decisions=0.8, heuristics=0.7)
			FreshnessBoost:   1.0, // fresh default
			ScopeExactness:   1.0, // exact project match assumed
			TopicKeyMatch:    topicKeyMatch,
			IsRequestedSDD:   isRequestedSDD,
			SnippetTruncated: snippetTruncated,
		}

		final := ComputeFinalScore(s.config.Weights, signals)

		// Persist the bumped TypeBoost in the DTO so the SDD-type increment
		// is observable to clients without changing the response shape.
		dtoTypeBoost := signals.TypeBoost
		if isRequestedSDD {
			dtoTypeBoost += s.config.Weights.SDDTypeIncrement
		}
		signals.TypeBoost = dtoTypeBoost

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

// containsTopicKey reports whether `query` appears as a substring of `snippet`.
// Cheap by design; the topic-key boost is a soft signal, not a strict equality
// check. Both strings are taken as-is (case-sensitive) because topic_keys are
// case-sensitive in the engine. An empty query returns false.
func containsTopicKey(snippet, query string) bool {
	if query == "" {
		return false
	}
	return strings.Contains(snippet, query)
}
