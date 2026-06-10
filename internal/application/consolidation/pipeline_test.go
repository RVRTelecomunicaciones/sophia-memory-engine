package consolidation_test

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panicPromoter panics when Evaluate is called — used to test panic recovery.
type panicPromoter struct{}

func (p panicPromoter) Evaluate(_ context.Context, _ *outbound.SkillSnapshot) (string, bool) {
	panic("promoter panic: simulated crash")
}

// ---------------------------------------------------------------------------
// N.4: panic in promoter step is recovered; pipeline continues; process does not exit
// ---------------------------------------------------------------------------

func TestHandler_PanicInPromoter_Recovered(t *testing.T) {
	mem := newFakeMemoryClient()
	// Provide a skills client that returns a usage row so the promoter is exercised.
	skills := &fakeSkillsClientWithUsage{
		usageRows: []outbound.SkillUsageRow{
			{
				SkillUsageID:  "row-panic",
				ChangeID:      "change-panic",
				SkillID:       "skill-panic",
				SkillVersion:  "v1",
				Outcome:       "success",
				ApplyAttempts: 1,
			},
		},
		snapshots: map[string]*outbound.SkillSnapshot{
			"skill-panic": {
				SkillID:   "skill-panic",
				Status:    "candidate",
				RiskLevel: "low",
				Metrics:   outbound.SkillMetrics{SuccessCount: 1, TestsPassedCount: 1},
			},
		},
	}

	h := consolidation.NewHandlerV2(testLogger(t), testClock(t), mem, skills).
		WithPromoter(panicPromoter{})

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-panic",
		ChangeName: "panic-test",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}

	// The pipeline must NOT panic out — it must recover and return nil.
	require.NotPanics(t, func() {
		err := h.Handle(context.Background(), payload)
		// The pipeline continues after panic recovery; digest should still be persisted.
		require.NoError(t, err, "panic must be recovered; pipeline must return nil")
	})

	// Digest must still be written despite the panic.
	assert.NotEmpty(t, mem.ingested, "digest must be persisted even when promoter panics")
}

// fakeSkillsClientWithUsage is a skills client that returns configurable usage rows and snapshots.
type fakeSkillsClientWithUsage struct {
	usageRows []outbound.SkillUsageRow
	snapshots map[string]*outbound.SkillSnapshot
}

func (f *fakeSkillsClientWithUsage) PatchMetrics(_ context.Context, _ string, _ outbound.MetricsDelta) error {
	return nil
}

func (f *fakeSkillsClientWithUsage) PatchStatus(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeSkillsClientWithUsage) GetSkill(_ context.Context, skillID string) (*outbound.SkillSnapshot, error) {
	if snap, ok := f.snapshots[skillID]; ok {
		return snap, nil
	}
	return &outbound.SkillSnapshot{SkillID: skillID, Status: "candidate", RiskLevel: "low"}, nil
}

func (f *fakeSkillsClientWithUsage) GetUsage(_ context.Context, _ string) ([]outbound.SkillUsageRow, error) {
	return f.usageRows, nil
}
