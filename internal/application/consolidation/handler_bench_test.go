package consolidation_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// benchMemoryClient is an in-memory MemoryClient that never reports an existing
// digest (so the pipeline always runs the full per-skill loop) and discards
// ingested content. No Postgres, no Docker — pure in-memory fakes (D-LH-5).
type benchMemoryClient struct{}

func (benchMemoryClient) HasTopic(context.Context, string) (bool, error)  { return false, nil }
func (benchMemoryClient) ReadContent(context.Context, string) (string, error) { return "", nil }
func (benchMemoryClient) Ingest(context.Context, consolidation.IngestRequest) error {
	return nil
}

// benchSkillsClient returns a configurable number of distinct skill_usage rows
// so the benchmark exposes the per-skill cost of the consolidation loop. All
// calls are O(1) in-memory so the measurement reflects the pipeline loop, not
// network or DB latency.
type benchSkillsClient struct {
	rows []outbound.SkillUsageRow
}

func newBenchSkillsClient(changeID string, n int) *benchSkillsClient {
	rows := make([]outbound.SkillUsageRow, n)
	for i := 0; i < n; i++ {
		rows[i] = outbound.SkillUsageRow{
			SkillUsageID:  fmt.Sprintf("usage-%06d", i),
			ChangeID:      changeID,
			PhaseType:     "apply",
			SkillID:       fmt.Sprintf("skill-%06d", i),
			SkillVersion:  "1.0.0",
			Outcome:       "success",
			ApplyAttempts: 1,
		}
	}
	return &benchSkillsClient{rows: rows}
}

func (c *benchSkillsClient) GetUsage(context.Context, string) ([]outbound.SkillUsageRow, error) {
	return c.rows, nil
}

func (c *benchSkillsClient) PatchMetrics(context.Context, string, outbound.MetricsDelta) error {
	return nil
}

func (c *benchSkillsClient) PatchStatus(context.Context, string, string, string) error {
	return nil
}

func (c *benchSkillsClient) GetSkill(_ context.Context, skillID string) (*outbound.SkillSnapshot, error) {
	return &outbound.SkillSnapshot{SkillID: skillID, Status: "candidate", RiskLevel: "low"}, nil
}

// BenchmarkHandlerV2_Handle measures ns/op and allocs/op of the full
// 9-step HandlerV2.Handle pipeline across increasing skill_usage row counts.
// It runs entirely on in-memory fakes (no integration build tag, no Docker),
// so the per-skill loop cost is attributable from the row-count progression.
func BenchmarkHandlerV2_Handle(b *testing.B) {
	// Discard logs so logging I/O doesn't pollute the measurement.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := shared.NewFixedClock(time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC))
	mem := benchMemoryClient{}
	ctx := context.Background()

	for _, rows := range []int{1, 10, 100, 1000} {
		rows := rows
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			changeID := fmt.Sprintf("bench-change-%d", rows)
			skills := newBenchSkillsClient(changeID, rows)
			h := consolidation.NewHandlerV2(log, clock, mem, skills)
			payload := consolidation.PhaseArchivedReceived{
				ChangeID:   changeID,
				ChangeName: "bench",
				PhaseType:  "archive",
				ArchivedAt: clock.Now(),
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := h.Handle(ctx, payload); err != nil {
					b.Fatalf("Handle: %v", err)
				}
			}
		})
	}
}
