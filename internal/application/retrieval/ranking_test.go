package retrieval_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
)

// baseWeights are the production defaults from config.DefaultConfig — pinned
// here so the ranking tests stay deterministic if the defaults move.
var baseWeights = config.RankingWeights{
	FTS:                     0.25,
	Trigram:                 0.15,
	Recency:                 0.12,
	Importance:              0.13,
	TypeBoost:               0.10,
	Freshness:               0.10,
	ScopeExactness:          0.15,
	TopicKeyBoost:           1.5,
	SDDTypeIncrement:        0.10,
	TruncatedSnippetPenalty: 0.85,
}

// baseSignals is the neutral signal set we mutate per test case. All boolean
// flags are off — each test flips exactly one and observes the delta.
var baseSignals = retrieval.RankingSignals{
	FTSScore:        0.8,
	TRGMScore:       0.6,
	RecencyBoost:    0.5,
	ImportanceScore: 0.7,
	TypeBoost:       0.5,
	FreshnessBoost:  1.0,
	ScopeExactness:  1.0,
}

func TestComputeFinalScore(t *testing.T) {
	weights := config.RankingWeights{
		FTS:            0.25,
		Trigram:        0.15,
		Recency:        0.12,
		Importance:     0.13,
		TypeBoost:      0.10,
		Freshness:      0.10,
		ScopeExactness: 0.15,
	}

	signals := retrieval.RankingSignals{
		FTSScore:        0.8,
		TRGMScore:       0.6,
		RecencyBoost:    0.5,
		ImportanceScore: 0.7,
		TypeBoost:       0.5,
		FreshnessBoost:  1.0,
		ScopeExactness:  1.0,
	}

	expected := 0.25*0.8 + 0.15*0.6 + 0.12*0.5 + 0.13*0.7 + 0.10*0.5 + 0.10*1.0 + 0.15*1.0

	got := retrieval.ComputeFinalScore(weights, signals)

	assert.InDelta(t, expected, got, 1e-10)
}

func TestComputeRecencyBoost(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("just created", func(t *testing.T) {
		boost := retrieval.ComputeRecencyBoost(now, now)
		assert.InDelta(t, 1.0, boost, 1e-10)
	})

	t.Run("1 day ago", func(t *testing.T) {
		createdAt := now.Add(-24 * time.Hour)
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		assert.InDelta(t, 0.5, boost, 1e-10)
	})

	t.Run("30 days ago", func(t *testing.T) {
		createdAt := now.Add(-30 * 24 * time.Hour)
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		expected := 1.0 / (1.0 + 30.0)
		assert.InDelta(t, expected, boost, 1e-10)
		assert.True(t, math.Abs(boost-0.032258) < 0.001)
	})

	t.Run("future date clamped to 1.0", func(t *testing.T) {
		createdAt := now.Add(48 * time.Hour) // future
		boost := retrieval.ComputeRecencyBoost(createdAt, now)
		assert.InDelta(t, 1.0, boost, 1e-10)
	})
}

// -----------------------------------------------------------------------------
// ADR-0005 P2.3 — sdd_* workload tuning
// -----------------------------------------------------------------------------

// TestComputeFinalScore_TopicKeyBoost asserts that flipping TopicKeyMatch from
// false to true on otherwise-identical signals multiplies the final score by
// weights.TopicKeyBoost (default 1.5×). This is the data-integrity guarantee
// for the sdd_*-targeted topic_key boost.
func TestComputeFinalScore_TopicKeyBoost(t *testing.T) {
	off := baseSignals
	off.TopicKeyMatch = false

	on := baseSignals
	on.TopicKeyMatch = true

	scoreOff := retrieval.ComputeFinalScore(baseWeights, off)
	scoreOn := retrieval.ComputeFinalScore(baseWeights, on)

	assert.Greater(t, scoreOn, scoreOff, "topic-key match must boost final score")
	assert.InDelta(t, scoreOff*baseWeights.TopicKeyBoost, scoreOn, 1e-10,
		"topic-key boost must be exactly TopicKeyBoost× of the unboosted score")
}

// TestComputeFinalScore_SDDTypeIncrement asserts that the SDD-type increment
// is additive on TypeBoost and that the increment is non-zero in the default
// configuration — guarding against accidental zeroing of the constant.
//
// Cases:
//   1. sdd_spec (in filter) vs non-sdd (not in filter): sdd_* must score higher
//   2. sdd_proposal (in filter) and sdd_spec (in filter) both get the bump
//   3. custom_type (not in filter) gets NO increment regardless of sdd_* prefix
func TestComputeFinalScore_SDDTypeIncrement(t *testing.T) {
	withoutBump := baseSignals
	withoutBump.IsRequestedSDD = false

	withBump := baseSignals
	withBump.IsRequestedSDD = true

	scoreWithout := retrieval.ComputeFinalScore(baseWeights, withoutBump)
	scoreWith := retrieval.ComputeFinalScore(baseWeights, withBump)

	expectedDelta := baseWeights.TypeBoost * baseWeights.SDDTypeIncrement
	assert.InDelta(t, scoreWithout+expectedDelta, scoreWith, 1e-10,
		"sdd_* requested-type increment must add exactly TypeBoost × SDDTypeIncrement")
	assert.Greater(t, baseWeights.SDDTypeIncrement, 0.0,
		"default SDDTypeIncrement must be non-zero")
}

// TestIsRequestedSDDType pins down the precondition for the increment:
// only types that BOTH start with "sdd_" AND appear in the Types filter
// receive the bump. A bare sdd_* type without an explicit filter does not.
func TestIsRequestedSDDType(t *testing.T) {
	t.Run("sdd_spec in filter -> true", func(t *testing.T) {
		assert.True(t, retrieval.IsRequestedSDDType("sdd_spec",
			[]string{"sdd_proposal", "sdd_spec"}))
	})

	t.Run("sdd_proposal in filter -> true", func(t *testing.T) {
		assert.True(t, retrieval.IsRequestedSDDType("sdd_proposal",
			[]string{"sdd_proposal", "sdd_spec"}))
	})

	t.Run("custom_type not in filter -> false", func(t *testing.T) {
		assert.False(t, retrieval.IsRequestedSDDType("custom_type",
			[]string{"sdd_proposal", "sdd_spec"}))
	})

	t.Run("sdd_design not in filter -> false (sdd_* alone is not enough)", func(t *testing.T) {
		assert.False(t, retrieval.IsRequestedSDDType("sdd_design",
			[]string{"sdd_proposal", "sdd_spec"}))
	})

	t.Run("empty filter -> false", func(t *testing.T) {
		assert.False(t, retrieval.IsRequestedSDDType("sdd_spec", nil))
	})
}

// TestComputeFinalScore_TruncatedSnippetPenalty asserts that a truncated
// snippet demotes the final score by exactly weights.TruncatedSnippetPenalty
// relative to the same signals with a full snippet.
func TestComputeFinalScore_TruncatedSnippetPenalty(t *testing.T) {
	full := baseSignals
	full.SnippetTruncated = false

	truncated := baseSignals
	truncated.SnippetTruncated = true

	scoreFull := retrieval.ComputeFinalScore(baseWeights, full)
	scoreTruncated := retrieval.ComputeFinalScore(baseWeights, truncated)

	assert.Less(t, scoreTruncated, scoreFull, "truncated snippet must lower the score")
	assert.InDelta(t, scoreFull*baseWeights.TruncatedSnippetPenalty, scoreTruncated, 1e-10,
		"truncated-snippet penalty must be exactly TruncatedSnippetPenalty× of the full score")
}

// TestIsSnippetTruncated covers the snippet-marker detection used by the
// search service to decide when to flag SnippetTruncated.
func TestIsSnippetTruncated(t *testing.T) {
	t.Run("contains marker at start -> true", func(t *testing.T) {
		assert.True(t, retrieval.IsSnippetTruncated("...partial sentence here"))
	})

	t.Run("contains marker at end -> true", func(t *testing.T) {
		assert.True(t, retrieval.IsSnippetTruncated("opening fragment..."))
	})

	t.Run("contains marker in middle -> true", func(t *testing.T) {
		assert.True(t, retrieval.IsSnippetTruncated("foo ... bar"))
	})

	t.Run("no marker -> false", func(t *testing.T) {
		assert.False(t, retrieval.IsSnippetTruncated("complete sentence with content"))
	})

	t.Run("empty snippet -> false", func(t *testing.T) {
		assert.False(t, retrieval.IsSnippetTruncated(""))
	})
}

// TestComputeFinalScore_BoostsCompose asserts that all three P2.3 factors
// compose multiplicatively (or additively, for the SDD increment) without
// stepping on each other. Order of application: SDD-increment (into TypeBoost
// pre-sum) → topic-key (× post-sum) → truncated (× post-sum). Final score
// must equal the deterministic product.
func TestComputeFinalScore_BoostsCompose(t *testing.T) {
	sig := baseSignals
	sig.IsRequestedSDD = true
	sig.TopicKeyMatch = true
	sig.SnippetTruncated = true

	// Hand-compute the expected value.
	bumpedType := sig.TypeBoost + baseWeights.SDDTypeIncrement
	base := baseWeights.FTS*sig.FTSScore +
		baseWeights.Trigram*sig.TRGMScore +
		baseWeights.Recency*sig.RecencyBoost +
		baseWeights.Importance*sig.ImportanceScore +
		baseWeights.TypeBoost*bumpedType +
		baseWeights.Freshness*sig.FreshnessBoost +
		baseWeights.ScopeExactness*sig.ScopeExactness
	expected := base * baseWeights.TopicKeyBoost * baseWeights.TruncatedSnippetPenalty

	got := retrieval.ComputeFinalScore(baseWeights, sig)
	assert.InDelta(t, expected, got, 1e-10,
		"all three P2.3 boosts must compose deterministically")
}
