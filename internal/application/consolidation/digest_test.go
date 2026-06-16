package consolidation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Fake implementations for testing the idempotency guard + digest
// ---------------------------------------------------------------------------

type fakeMemoryClient struct {
	existing  map[string]bool  // topic_key → exists
	topicErr  map[string]error // topic_key → error from HasTopic
	ingested  []consolidation.IngestRequest
	ingestErr error
}

func newFakeMemoryClient() *fakeMemoryClient {
	return &fakeMemoryClient{
		existing: make(map[string]bool),
		topicErr: make(map[string]error),
	}
}

func (f *fakeMemoryClient) HasTopic(_ context.Context, topicKey string) (bool, error) {
	if err, ok := f.topicErr[topicKey]; ok {
		return false, err
	}
	return f.existing[topicKey], nil
}

func (f *fakeMemoryClient) ReadContent(_ context.Context, topicKey string) (string, error) {
	// Return the last ingested content for this topic_key, if any.
	for i := len(f.ingested) - 1; i >= 0; i-- {
		if f.ingested[i].TopicKey == topicKey {
			return f.ingested[i].Content, nil
		}
	}
	return "", nil
}

func (f *fakeMemoryClient) Ingest(_ context.Context, req consolidation.IngestRequest) error {
	if f.ingestErr != nil {
		return f.ingestErr
	}
	f.ingested = append(f.ingested, req)
	return nil
}

type fakeSkillsClient struct {
	patchMetricsCalls []string
	patchStatusCalls  []string
}

func (f *fakeSkillsClient) PatchMetrics(_ context.Context, skillID string, _ outbound.MetricsDelta) error {
	f.patchMetricsCalls = append(f.patchMetricsCalls, skillID)
	return nil
}

func (f *fakeSkillsClient) PatchStatus(_ context.Context, skillID string, _, _ string) error {
	f.patchStatusCalls = append(f.patchStatusCalls, skillID)
	return nil
}

func (f *fakeSkillsClient) GetSkill(_ context.Context, _ string) (*outbound.SkillSnapshot, error) {
	return &outbound.SkillSnapshot{Status: "candidate", RiskLevel: "low"}, nil
}

func (f *fakeSkillsClient) GetUsage(_ context.Context, _ string) ([]outbound.SkillUsageRow, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// J.1: digest/{change_id} exists → Handle returns nil immediately, no metrics
// ---------------------------------------------------------------------------

func TestHandler_Idempotency_ExistingDigest_SkipsProcessing(t *testing.T) {
	mem := newFakeMemoryClient()
	mem.existing["digest/change-already-done"] = true

	skills := &fakeSkillsClient{}

	h := consolidation.NewHandlerV2(testLogger(t), testClock(t), mem, skills)

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-already-done",
		ChangeName: "old-feature",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}
	err := h.Handle(context.Background(), payload)
	require.NoError(t, err, "idempotent skip must return nil")

	assert.Empty(t, skills.patchMetricsCalls, "no PatchMetrics call on duplicate change")
	assert.Empty(t, mem.ingested, "no Ingest call on duplicate change")
}

// ---------------------------------------------------------------------------
// J.2: digest/{change_id} absent → pipeline proceeds
// ---------------------------------------------------------------------------

func TestHandler_Idempotency_AbsentDigest_PipelineProceeds(t *testing.T) {
	mem := newFakeMemoryClient()
	// No entry for "digest/new-change" — pipeline should proceed.
	skills := &fakeSkillsClient{}

	h := consolidation.NewHandlerV2(testLogger(t), testClock(t), mem, skills)

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "new-change",
		ChangeName: "brand-new",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}
	_ = h.Handle(context.Background(), payload)

	// Ingest must have been called (digest written at end).
	require.NotEmpty(t, mem.ingested, "digest must be persisted when change is new")
	assert.Equal(t, "digest/new-change", mem.ingested[0].TopicKey)
}

// ---------------------------------------------------------------------------
// J.3: HasTopic returns error → pipeline aborts, no metrics call
// ---------------------------------------------------------------------------

func TestHandler_Idempotency_HasTopicError_AbortsWithError(t *testing.T) {
	mem := newFakeMemoryClient()
	topicErr := errors.New("memory backend unavailable")
	mem.topicErr["digest/broken-change"] = topicErr

	skills := &fakeSkillsClient{}

	h := consolidation.NewHandlerV2(testLogger(t), testClock(t), mem, skills)

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "broken-change",
		ChangeName: "bad",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}
	err := h.Handle(context.Background(), payload)
	require.Error(t, err, "HasTopic error must propagate")

	assert.Empty(t, skills.patchMetricsCalls, "no PatchMetrics call when guard fails")
}

// ---------------------------------------------------------------------------
// J.4: Golden test — digest YAML is deterministic (byte-identical on re-marshal)
// ---------------------------------------------------------------------------

func TestBuildDigest_DeterministicYAML(t *testing.T) {
	input := consolidation.ChangeDigest{
		ChangeID:        "change-gold",
		ProjectID:       "proj-1",
		DurationSeconds: 120,
		Phases: []consolidation.DigestPhase{
			{Phase: "spec", Status: "done", Attempts: 1},
			{Phase: "apply", Status: "done", Attempts: 2, RetryReasons: []string{"env error"}},
			{Phase: "explore", Status: "done", Attempts: 1},
		},
		SkillsUsed: []consolidation.DigestSkill{
			{SkillID: "zzz-skill", Outcome: "success"},
			{SkillID: "aaa-skill", Outcome: "success"},
			{SkillID: "mmm-skill", Outcome: "failure"},
		},
	}

	yaml1, err := consolidation.BuildDigest(input)
	require.NoError(t, err)
	yaml2, err := consolidation.BuildDigest(input)
	require.NoError(t, err)

	assert.Equal(t, yaml1, yaml2, "YAML output must be byte-identical across two invocations")

	// Phases must be sorted by phase_type ascending.
	var parsed struct {
		Phases []struct {
			Phase string `yaml:"phase"`
		} `yaml:"phases"`
		SkillsUsed []struct {
			SkillID string `yaml:"skill_id"`
		} `yaml:"skills_used"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(yaml1), &parsed))

	require.Len(t, parsed.Phases, 3)
	assert.Equal(t, "apply", parsed.Phases[0].Phase, "first phase must be 'apply' (alphabetical)")
	assert.Equal(t, "explore", parsed.Phases[1].Phase, "second phase must be 'explore'")
	assert.Equal(t, "spec", parsed.Phases[2].Phase, "third phase must be 'spec'")

	require.Len(t, parsed.SkillsUsed, 3)
	assert.Equal(t, "aaa-skill", parsed.SkillsUsed[0].SkillID, "first skill must be 'aaa-skill' (alphabetical)")
	assert.Equal(t, "mmm-skill", parsed.SkillsUsed[1].SkillID)
	assert.Equal(t, "zzz-skill", parsed.SkillsUsed[2].SkillID)

	// Golden file test: write once, compare on subsequent runs.
	goldenPath := filepath.Join("testdata", "digest_golden.yaml")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		require.NoError(t, os.MkdirAll("testdata", 0750))
		require.NoError(t, os.WriteFile(goldenPath, []byte(yaml1), 0600))
		t.Log("golden file written; re-run test to compare")
		return
	}
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(golden), yaml1, "YAML must match golden file byte-for-byte")
}

// ---------------------------------------------------------------------------
// Group L: golden fixture reflects the unknown-outcome filter (D-LH-4).
// A known envelope including one `unknown` skill is filtered before BuildDigest;
// the resulting YAML must match the regenerated golden fixture byte-for-byte and
// must OMIT the unknown entry while sorting phases/skills ascending.
// ---------------------------------------------------------------------------

func TestBuildDigest_GoldenFixture_OmitsUnknown(t *testing.T) {
	// Same envelope as the golden fixture, plus one `unknown` skill that the
	// filter must drop. Skills/phases are intentionally unsorted on input.
	rawSkills := []consolidation.DigestSkill{
		{SkillID: "zzz-skill", Outcome: "success"},
		{SkillID: "ddd-skill", Outcome: "unknown"}, // dropped by filter
		{SkillID: "aaa-skill", Outcome: "success"},
		{SkillID: "mmm-skill", Outcome: "failure"},
	}

	input := consolidation.ChangeDigest{
		ChangeID:        "change-gold",
		ProjectID:       "proj-1",
		DurationSeconds: 120,
		Phases: []consolidation.DigestPhase{
			{Phase: "spec", Status: "done", Attempts: 1},
			{Phase: "apply", Status: "done", Attempts: 2, RetryReasons: []string{"env error"}},
			{Phase: "explore", Status: "done", Attempts: 1},
		},
		SkillsUsed: consolidation.FilterDigestSkills(rawSkills),
	}

	got, err := consolidation.BuildDigest(input)
	require.NoError(t, err)

	// The unknown entry must not survive serialization.
	assert.NotContains(t, got, "ddd-skill", "unknown-outcome skill must be filtered out")
	assert.NotContains(t, got, "unknown", "no unknown outcome should appear in the digest")

	goldenPath := filepath.Join("testdata", "digest_golden.yaml")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll("testdata", 0750))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0600))
		t.Log("golden fixture regenerated; re-run without UPDATE_GOLDEN to compare")
		return
	}
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(golden), got, "filtered digest YAML must match golden fixture byte-for-byte")
}

// ---------------------------------------------------------------------------
// J.5: digest persisted via Ingest with correct topic_key/type/tags
// ---------------------------------------------------------------------------

func TestHandler_Digest_PersistedWithCorrectMetadata(t *testing.T) {
	mem := newFakeMemoryClient()
	skills := &fakeSkillsClient{}

	h := consolidation.NewHandlerV2(testLogger(t), testClock(t), mem, skills)

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "meta-check",
		ChangeName: "check-meta",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}
	err := h.Handle(context.Background(), payload)
	require.NoError(t, err)

	require.NotEmpty(t, mem.ingested)
	req := mem.ingested[0]
	assert.Equal(t, "digest/meta-check", req.TopicKey, "topic_key must be digest/{change_id}")
	assert.Equal(t, "semantic", req.Type, "type must be semantic")
	assert.Contains(t, req.Tags, "change_digest", "tags must include change_digest")
}
