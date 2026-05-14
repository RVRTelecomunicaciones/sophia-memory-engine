package retrieval

import (
	"strings"
	"time"

	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
)

// snippetTruncationMarker is the substring ts_headline emits at the boundary
// of a truncated fragment. We match the literal "..." rather than try to
// re-derive headline semantics — this is the only marker the snippet template
// uses (see postgres_fts.go ts_headline call).
const snippetTruncationMarker = "..."

// sddTypePrefix is the record-type prefix used by sophia-orchestator's SDD
// artifacts (sdd_proposal, sdd_spec, sdd_design, sdd_tasks, …). Records whose
// type begins with this prefix AND whose type was explicitly requested in the
// search Types filter receive SDDTypeIncrement on top of the base TypeBoost.
const sddTypePrefix = "sdd_"

// RankingSignals holds the individual scoring signals for hybrid retrieval ranking.
//
// The first 7 fields (FTSScore..ScopeExactness) feed the linear base score.
// The trailing booleans (P2.3) are flags consumed by ComputeFinalScore to
// apply multiplicative boosts / penalties AFTER the base composition. They
// are not weighted into the linear combination — that is intentional: each
// is a categorical signal, not a sliding scale.
type RankingSignals struct {
	FTSScore        float64
	TRGMScore       float64
	RecencyBoost    float64
	ImportanceScore float64
	TypeBoost       float64
	FreshnessBoost  float64
	ScopeExactness  float64

	// P2.3 flags — apply post-composition.
	TopicKeyMatch    bool // record's topic_key matches the caller's filter / query
	IsRequestedSDD   bool // record_type starts with sdd_ AND appears in the request's Types filter
	SnippetTruncated bool // ts_headline emitted a "..." marker — snippet is fragmented
}

// ComputeFinalScore computes the final ranking score from individual signals.
//
// The base score is a linear combination of the 7 weighted signals. After the
// linear step, we apply (in this order, each independent of the others):
//
//   1. SDD-type increment — when IsRequestedSDD is true, the TypeBoost
//      component is bumped by SDDTypeIncrement BEFORE the linear sum. This
//      keeps the type bump observable in the TypeBoost field of the ranking
//      DTO (no new DTO field — public API shape is preserved).
//   2. Topic-key boost — when TopicKeyMatch is true, the final score is
//      multiplied by TopicKeyBoost. This dominates FTS/TRGM-only matches.
//   3. Truncated-snippet penalty — when SnippetTruncated is true, the final
//      score is multiplied by TruncatedSnippetPenalty. This demotes fragmented
//      hits in favour of full-content matches.
//
// Determinism: pure function of (weights, signals). No clock, no I/O. Safe to
// reuse across goroutines.
func ComputeFinalScore(weights config.RankingWeights, signals RankingSignals) float64 {
	// 1. Apply the SDD-type increment to the TypeBoost component. We do this
	// in a local copy so callers' RankingSignals are not mutated; the
	// returned float reflects the bumped value in TypeBoost only at compute
	// time, but the original signals.TypeBoost (already including the bump
	// if applied upstream) is what the caller stores in the DTO.
	typeBoost := signals.TypeBoost
	if signals.IsRequestedSDD {
		typeBoost += weights.SDDTypeIncrement
	}

	base := weights.FTS*signals.FTSScore +
		weights.Trigram*signals.TRGMScore +
		weights.Recency*signals.RecencyBoost +
		weights.Importance*signals.ImportanceScore +
		weights.TypeBoost*typeBoost +
		weights.Freshness*signals.FreshnessBoost +
		weights.ScopeExactness*signals.ScopeExactness

	// 2. Topic-key exact-match boost.
	if signals.TopicKeyMatch && weights.TopicKeyBoost > 0 {
		base *= weights.TopicKeyBoost
	}

	// 3. Truncated-snippet penalty.
	if signals.SnippetTruncated && weights.TruncatedSnippetPenalty > 0 {
		base *= weights.TruncatedSnippetPenalty
	}

	return base
}

// ComputeRecencyBoost returns a score between 0 and 1 based on age.
// Just created → ~1.0, 1 day ago → ~0.5, older → approaching 0.
func ComputeRecencyBoost(createdAt time.Time, now time.Time) float64 {
	days := now.Sub(createdAt).Hours() / 24.0
	if days < 0 {
		days = 0
	}
	return 1.0 / (1.0 + days)
}

// IsSnippetTruncated reports whether a ts_headline-produced snippet was
// fragmented. The Postgres ts_headline template configured in postgres_fts.go
// uses "..." as the only marker; we match on the literal substring.
//
// Edge case: a search hit on a record that contains "..." in its own content
// would be wrongly flagged. We accept that: in the orchestator's audit
// workload the content of SDD artifacts is structured Markdown without bare
// ellipses, and the impact (× 0.85 on score) is small enough to not flip
// ordering for true high-quality hits.
func IsSnippetTruncated(snippet string) bool {
	return strings.Contains(snippet, snippetTruncationMarker)
}

// IsSDDType reports whether a record type belongs to the SDD family
// (sdd_proposal, sdd_spec, sdd_design, sdd_tasks, sdd_apply, sdd_verify, …).
func IsSDDType(recordType string) bool {
	return strings.HasPrefix(recordType, sddTypePrefix)
}

// IsRequestedSDDType reports whether `recordType` is an SDD-family type AND
// the caller explicitly requested it via the search Types filter. We require
// both — a plain unsolicited sdd_* match (e.g., when the caller filters by
// no type at all) does NOT get the increment, because the increment exists
// to disambiguate WITHIN an SDD-targeted query, not to advantage SDD records
// across every search.
func IsRequestedSDDType(recordType string, requestedTypes []string) bool {
	if !IsSDDType(recordType) {
		return false
	}
	for _, t := range requestedTypes {
		if t == recordType {
			return true
		}
	}
	return false
}
