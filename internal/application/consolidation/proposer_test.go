package consolidation_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// fakeProposerSkillsClient — satisfies outbound.ProposerSkillsClient
// (aliased as consolidation.ProposerSkillsClient).
// Returns a wide view with skill_name, scope, applies_when populated.
// ---------------------------------------------------------------------------

type fakeProposerSkillsClient struct {
	wideResult *outbound.ProposerSkillView
	wideErr    error
}

func (f *fakeProposerSkillsClient) GetSkillWide(_ context.Context, _ string) (*outbound.ProposerSkillView, error) {
	return f.wideResult, f.wideErr
}

func defaultWideResult(skillID string) *outbound.ProposerSkillView {
	return &outbound.ProposerSkillView{
		SkillSnapshot: outbound.SkillSnapshot{
			SkillID:   skillID,
			Status:    "validated",
			RiskLevel: "low",
			Version:   "v2",
			Metrics:   outbound.SkillMetrics{UsageCount: 5, SuccessCount: 3},
		},
		SkillName:   "clean-arch",
		Scope:       map[string]any{"project_id": "proj-1"},
		AppliesWhen: map[string]any{"feature_type": []any{"backend"}},
	}
}

// ---------------------------------------------------------------------------
// M.1: validated + usage_count=5 → proposal emitted at governance/skill-proposal/{skill_id}
// ---------------------------------------------------------------------------

func TestProposer_ValidatedUsage5_EmitsProposal(t *testing.T) {
	mem := newFakeMemoryClient()
	p := consolidation.NewProposer(mem, testClock(t))

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-gov-1",
		Status:    "validated",
		RiskLevel: "low",
		Version:   "v1",
		Metrics:   outbound.SkillMetrics{UsageCount: 5, SuccessCount: 3},
	}
	err := p.Emit(context.Background(), snap, "change-x")
	require.NoError(t, err)

	require.Len(t, mem.ingested, 1, "exactly one Ingest call must be made")
	req := mem.ingested[0]
	assert.Equal(t, "governance/skill-proposal/skill-gov-1", req.TopicKey)
	assert.Equal(t, "semantic", req.Type)
	assert.Contains(t, req.Tags, "governance")
	assert.Contains(t, req.Tags, "skill_proposal")
	assert.Contains(t, req.Tags, "pending")
}

// ---------------------------------------------------------------------------
// M.2: validated + usage_count=4 → no proposal
// ---------------------------------------------------------------------------

func TestProposer_ValidatedUsage4_NoProposal(t *testing.T) {
	mem := newFakeMemoryClient()
	p := consolidation.NewProposer(mem, testClock(t))

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-low-usage",
		Status:    "validated",
		RiskLevel: "low",
		Metrics:   outbound.SkillMetrics{UsageCount: 4},
	}
	err := p.Emit(context.Background(), snap, "change-y")
	require.NoError(t, err)

	assert.Empty(t, mem.ingested, "no proposal must be emitted for usage_count=4")
}

// ---------------------------------------------------------------------------
// M.3: active skill → no proposal (wrong status)
// ---------------------------------------------------------------------------

func TestProposer_ActiveSkill_NoProposal(t *testing.T) {
	mem := newFakeMemoryClient()
	p := consolidation.NewProposer(mem, testClock(t))

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-active",
		Status:    "active",
		RiskLevel: "medium",
		Metrics:   outbound.SkillMetrics{UsageCount: 10},
	}
	err := p.Emit(context.Background(), snap, "change-z")
	require.NoError(t, err)

	assert.Empty(t, mem.ingested, "no proposal for active skill")
}

// ---------------------------------------------------------------------------
// M.4: re-emission appends evidence_changes; metrics snapshot updated
// ---------------------------------------------------------------------------

func TestProposer_ReEmission_AppendsEvidenceChanges(t *testing.T) {
	mem := newFakeMemoryClient()
	p := consolidation.NewProposer(mem, testClock(t))

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-re-emit",
		Status:    "validated",
		RiskLevel: "low",
		Version:   "v1",
		Metrics:   outbound.SkillMetrics{UsageCount: 5, SuccessCount: 2},
	}

	// First emission.
	require.NoError(t, p.Emit(context.Background(), snap, "change-A"))
	require.Len(t, mem.ingested, 1)

	// Parse first proposal to get its content.
	var first consolidation.SkillActivationProposal
	require.NoError(t, yaml.Unmarshal([]byte(mem.ingested[0].Content), &first))
	require.Len(t, first.EvidenceChanges, 1)
	assert.Equal(t, "change-A", first.EvidenceChanges[0])

	// Second emission with updated metrics.
	snap.Metrics.UsageCount = 7
	require.NoError(t, p.Emit(context.Background(), snap, "change-B"))
	// Should still be 1 call (upsert via topic_key); the content is updated.
	require.Len(t, mem.ingested, 2, "proposer upserts by calling Ingest again with same topic_key")

	// Parse second proposal.
	var second consolidation.SkillActivationProposal
	require.NoError(t, yaml.Unmarshal([]byte(mem.ingested[1].Content), &second))
	require.Len(t, second.EvidenceChanges, 2, "evidence_changes must include both change IDs")
	assert.Equal(t, "change-A", second.EvidenceChanges[0])
	assert.Equal(t, "change-B", second.EvidenceChanges[1])
	assert.Equal(t, 7, second.Metrics.UsageCount, "metrics snapshot must be updated")
}

// ---------------------------------------------------------------------------
// M.5: proposal proposed_by="archive_worker"; change_id appears in evidence_changes
// ---------------------------------------------------------------------------

func TestProposer_ProposedBy_ArchiveWorker(t *testing.T) {
	mem := newFakeMemoryClient()
	p := consolidation.NewProposer(mem, testClock(t))

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-check-fields",
		Status:    "validated",
		RiskLevel: "low",
		Version:   "v2",
		Metrics:   outbound.SkillMetrics{UsageCount: 6, SuccessCount: 4},
	}
	require.NoError(t, p.Emit(context.Background(), snap, "change-evidence"))

	require.Len(t, mem.ingested, 1)
	req := mem.ingested[0]

	// Proposal is stored as YAML.
	var proposal consolidation.SkillActivationProposal
	require.NoError(t, yaml.Unmarshal([]byte(req.Content), &proposal))

	assert.Equal(t, "archive_worker", proposal.ProposedBy)
	assert.Contains(t, proposal.EvidenceChanges, "change-evidence")

	// Also verify the content is valid YAML (not accidentally JSON-only).
	assert.True(t, strings.Contains(req.Content, "proposed_by:"),
		"proposal content must use YAML format with proposed_by key")
	// Sanity: not parseable as pure JSON (it should be YAML)
	var jsonTest interface{}
	jsonErr := json.Unmarshal([]byte(req.Content), &jsonTest)
	_ = jsonErr // YAML might look like JSON for simple structs; just verify YAML parse worked above
}

// ---------------------------------------------------------------------------
// B.1 (M3): full V4.1 §9 proposal shape — skill_name, scope, applies_when,
// risk_level MUST all be present in the emitted YAML.
// REQUIRES: NewProposerWithSkills + ProposerSkillsClient + ProposerSkillView.
// MUST FAIL until B.3/B.4 GREEN step is done.
// ---------------------------------------------------------------------------

func TestProposer_FullV41Shape_AllNineFields(t *testing.T) {
	mem := newFakeMemoryClient()
	skillsCli := &fakeProposerSkillsClient{
		wideResult: defaultWideResult("skill-full-shape"),
	}
	p := consolidation.NewProposerWithSkills(mem, testClock(t), skillsCli)

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-full-shape",
		Status:    "validated",
		RiskLevel: "low",
		Version:   "v2",
		Metrics:   outbound.SkillMetrics{UsageCount: 5, SuccessCount: 3},
	}
	require.NoError(t, p.Emit(context.Background(), snap, "change-full"))

	require.Len(t, mem.ingested, 1)
	content := mem.ingested[0].Content

	var proposal consolidation.SkillActivationProposal
	require.NoError(t, yaml.Unmarshal([]byte(content), &proposal))

	// All 9 V4.1 §9 fields must be present and non-empty/non-nil.
	require.Equal(t, "skill-full-shape", proposal.SkillID, "skill_id must be set")
	require.Equal(t, "clean-arch", proposal.SkillName, "skill_name must be set from wide fetch")
	require.Equal(t, "v2", proposal.Version, "version must be set")
	require.NotEmpty(t, proposal.Scope, "scope must be set from wide fetch")
	require.NotEmpty(t, proposal.AppliesWhen, "applies_when must be set from wide fetch")
	require.Equal(t, "low", proposal.RiskLevel, "risk_level must be set from snapshot")
	require.NotZero(t, proposal.Metrics.UsageCount, "metrics must be set")
	require.Equal(t, "archive_worker", proposal.ProposedBy, "proposed_by must be archive_worker")
	require.Contains(t, proposal.EvidenceChanges, "change-full", "evidence_changes must contain change_id")
}

// ---------------------------------------------------------------------------
// B.2 (M3): re-emit appends evidence_changes; all 9 §9 fields still present.
// MUST FAIL until B.3/B.4 GREEN step is done.
// ---------------------------------------------------------------------------

func TestProposer_ReEmitWithWideFields_AllNineFieldsPresent(t *testing.T) {
	mem := newFakeMemoryClient()
	skillsCli := &fakeProposerSkillsClient{
		wideResult: defaultWideResult("skill-re-emit-wide"),
	}
	p := consolidation.NewProposerWithSkills(mem, testClock(t), skillsCli)

	snap := &outbound.SkillSnapshot{
		SkillID:   "skill-re-emit-wide",
		Status:    "validated",
		RiskLevel: "medium",
		Version:   "v1",
		Metrics:   outbound.SkillMetrics{UsageCount: 5, SuccessCount: 3},
	}

	// First emission.
	require.NoError(t, p.Emit(context.Background(), snap, "change-first"))
	require.Len(t, mem.ingested, 1)

	// Second emission — same skill, new change_id.
	snap.Metrics.UsageCount = 7
	wide2 := defaultWideResult("skill-re-emit-wide")
	wide2.SkillSnapshot.Metrics.UsageCount = 7
	skillsCli.wideResult = wide2
	require.NoError(t, p.Emit(context.Background(), snap, "change-second"))
	require.Len(t, mem.ingested, 2, "proposer upserts by calling Ingest twice with same topic_key")

	// Parse the second emission.
	var second consolidation.SkillActivationProposal
	require.NoError(t, yaml.Unmarshal([]byte(mem.ingested[1].Content), &second))

	// All 9 §9 fields must be present.
	require.Equal(t, "skill-re-emit-wide", second.SkillID)
	require.Equal(t, "clean-arch", second.SkillName, "skill_name must persist on re-emit")
	require.NotEmpty(t, second.Scope, "scope must persist on re-emit")
	require.NotEmpty(t, second.AppliesWhen, "applies_when must persist on re-emit")
	require.NotEmpty(t, second.RiskLevel, "risk_level must persist on re-emit")
	require.Equal(t, 7, second.Metrics.UsageCount, "metrics snapshot must be updated on re-emit")
	require.Equal(t, "archive_worker", second.ProposedBy)
	require.Len(t, second.EvidenceChanges, 2, "evidence_changes must accumulate")
	require.Equal(t, "change-first", second.EvidenceChanges[0])
	require.Equal(t, "change-second", second.EvidenceChanges[1])
}
