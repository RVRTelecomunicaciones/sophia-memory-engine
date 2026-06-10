package consolidation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Handler is the PRE-0 consolidation entrypoint (log-only stub, kept for PRE-0 tests).
// New code should use HandlerV2 which wires the real pipeline.
type Handler struct {
	log   *slog.Logger
	clock shared.Clock
}

// NewHandler creates a Handler with the given logger and clock.
func NewHandler(log *slog.Logger, clock shared.Clock) *Handler {
	return &Handler{log: log, clock: clock}
}

// Handle is the PRE-0 log-only stub. M2 pipeline is in HandlerV2.Handle.
func (h *Handler) Handle(ctx context.Context, payload PhaseArchivedReceived) error {
	h.log.InfoContext(ctx, "phase.archived received",
		slog.String("change_id", payload.ChangeID),
		slog.String("change_name", payload.ChangeName),
		slog.Time("archived_at", payload.ArchivedAt),
		slog.Time("received_at", h.clock.Now()),
	)
	return nil
}

// HandlerV2 is the M2 consolidation pipeline entrypoint.
// It replaces the PRE-0 log-only stub and wires the real pipeline:
//
//  1. Idempotency guard (digest/{change_id} existence check)
//  2. GetUsage from orch to discover which skills were used
//  3. computeDeltas per skill
//  4. PatchMetrics loop (continue on per-skill error)
//  5. Promoter evaluation (candidate → validated)
//  6. Demoter evaluation (active → blocked/deprecated)
//  7. Proposer emission (validated + usage >= 5 → governance pending)
//  8. BuildDigest + Ingest to memory-engine
//
// Panics in per-skill steps are recovered; the error is logged and the
// pipeline continues to the next skill (worker process does not exit).
type HandlerV2 struct {
	log      *slog.Logger
	clock    shared.Clock
	memory   MemoryClient
	skills   outbound.SkillsClient
	promoter SkillPromoter
	demoter  SkillDemoter
	proposer SkillProposer
}

// SkillPromoter evaluates whether a skill qualifies for promotion.
// Returns the new status and true when a transition should be applied.
type SkillPromoter interface {
	Evaluate(ctx context.Context, snap *outbound.SkillSnapshot) (newStatus string, ok bool)
}

// SkillDemoter evaluates whether a skill should be demoted.
// Returns the new status and true when a transition should be applied.
type SkillDemoter interface {
	Evaluate(ctx context.Context, snap *outbound.SkillSnapshot) (newStatus string, ok bool)
}

// SkillProposer emits a governance activation proposal for a validated skill.
type SkillProposer interface {
	Emit(ctx context.Context, snap *outbound.SkillSnapshot, changeID string) error
}

// noopPromoter satisfies SkillPromoter with no-op behaviour (used until promoter.go is wired).
type noopPromoter struct{}

func (noopPromoter) Evaluate(_ context.Context, _ *outbound.SkillSnapshot) (string, bool) {
	return "", false
}

// noopDemoter satisfies SkillDemoter with no-op behaviour.
type noopDemoter struct{}

func (noopDemoter) Evaluate(_ context.Context, _ *outbound.SkillSnapshot) (string, bool) {
	return "", false
}

// noopProposer satisfies SkillProposer with no-op behaviour.
type noopProposer struct{}

func (noopProposer) Emit(_ context.Context, _ *outbound.SkillSnapshot, _ string) error {
	return nil
}

// NewHandlerV2 constructs a HandlerV2 with noop promoter/demoter/proposer.
// Groups K, L, M replace these with real implementations via the With* methods
// or by injecting them at wire time.
func NewHandlerV2(log *slog.Logger, clock shared.Clock, memory MemoryClient, skills outbound.SkillsClient) *HandlerV2 {
	return &HandlerV2{
		log:      log,
		clock:    clock,
		memory:   memory,
		skills:   skills,
		promoter: noopPromoter{},
		demoter:  noopDemoter{},
		proposer: noopProposer{},
	}
}

// WithPromoter replaces the promoter. Used by the pipeline wire step (Group N).
func (h *HandlerV2) WithPromoter(p SkillPromoter) *HandlerV2 { h.promoter = p; return h }

// WithDemoter replaces the demoter.
func (h *HandlerV2) WithDemoter(d SkillDemoter) *HandlerV2 { h.demoter = d; return h }

// WithProposer replaces the proposer.
func (h *HandlerV2) WithProposer(p SkillProposer) *HandlerV2 { h.proposer = p; return h }

// Handle executes the M2 consolidation pipeline for a received archive event.
func (h *HandlerV2) Handle(ctx context.Context, payload PhaseArchivedReceived) error {
	// Step 1: Idempotency guard.
	exists, err := h.memory.HasTopic(ctx, "digest/"+payload.ChangeID)
	if err != nil {
		h.log.ErrorContext(ctx, "consolidation: idempotency check failed",
			slog.String("change_id", payload.ChangeID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("consolidation.Handle: idempotency check: %w", err)
	}
	if exists {
		h.log.InfoContext(ctx, "consolidation: change already processed; skip",
			slog.String("change_id", payload.ChangeID),
		)
		return nil
	}

	// Step 2: Fetch skill_usage rows for this change.
	usageRows, err := h.skills.GetUsage(ctx, payload.ChangeID)
	if err != nil {
		h.log.ErrorContext(ctx, "consolidation: GetUsage failed",
			slog.String("change_id", payload.ChangeID),
			slog.String("error", err.Error()),
		)
		// Non-fatal: continue with empty usage (digest will still be written).
		usageRows = nil
	}

	// Step 3: Per-skill delta loop.
	deltas := computeDeltas(usageRows)
	digestSkills := make([]DigestSkill, 0, len(usageRows))

	for _, d := range deltas {
		// Recover panics per skill so the pipeline continues.
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.log.ErrorContext(ctx, "consolidation: panic in skill step; skip",
						slog.String("change_id", payload.ChangeID),
						slog.String("skill_id", d.SkillID),
						slog.Any("panic", r),
					)
				}
			}()

			// Step 4: PatchMetrics — continue on error.
			if err := h.skills.PatchMetrics(ctx, d.SkillID, d.Delta); err != nil {
				h.log.ErrorContext(ctx, "consolidation: PatchMetrics failed; continue",
					slog.String("skill_id", d.SkillID),
					slog.String("error", err.Error()),
				)
				digestSkills = append(digestSkills, DigestSkill{SkillID: d.SkillID, Outcome: "failure"})
				return
			}

			// Step 5: GetSkill to read post-patch state.
			snap, err := h.skills.GetSkill(ctx, d.SkillID)
			if err != nil {
				h.log.WarnContext(ctx, "consolidation: GetSkill failed; skip promoter/demoter",
					slog.String("skill_id", d.SkillID),
					slog.String("error", err.Error()),
				)
				digestSkills = append(digestSkills, DigestSkill{SkillID: d.SkillID, Outcome: "unknown"})
				return
			}

			// Step 6: Promoter (candidate → validated).
			if snap.Status == "candidate" {
				if newStatus, ok := h.promoter.Evaluate(ctx, snap); ok {
					if pErr := h.skills.PatchStatus(ctx, d.SkillID, newStatus, "promoter: thresholds met"); pErr != nil {
						h.log.WarnContext(ctx, "consolidation: PatchStatus (promote) failed",
							slog.String("skill_id", d.SkillID),
							slog.String("error", pErr.Error()),
						)
					} else {
						snap.Status = newStatus
					}
				}
			}

			// Step 7: Demoter (active → blocked/deprecated).
			if snap.Status == "active" {
				if newStatus, ok := h.demoter.Evaluate(ctx, snap); ok {
					if pErr := h.skills.PatchStatus(ctx, d.SkillID, newStatus, "demoter: threshold exceeded"); pErr != nil {
						h.log.WarnContext(ctx, "consolidation: PatchStatus (demote) failed",
							slog.String("skill_id", d.SkillID),
							slog.String("error", pErr.Error()),
						)
					} else {
						snap.Status = newStatus
					}
				}
			}

			// Step 8: Proposer (validated + usage >= 5).
			if snap.Status == "validated" && snap.Metrics.UsageCount >= 5 {
				if pErr := h.proposer.Emit(ctx, snap, payload.ChangeID); pErr != nil {
					h.log.WarnContext(ctx, "consolidation: proposer.Emit failed",
						slog.String("skill_id", d.SkillID),
						slog.String("error", pErr.Error()),
					)
				}
			}

			outcome := "success"
			digestSkills = append(digestSkills, DigestSkill{SkillID: d.SkillID, Outcome: outcome})
		}()
	}

	// Step 9: Build digest and persist.
	digest := ChangeDigest{
		ChangeID:        payload.ChangeID,
		ProjectID:       "", // M3+: wire from auth context
		DurationSeconds: 0,  // M3+: compute from phase timestamps
		Phases: []DigestPhase{
			{Phase: payload.PhaseType, Status: "done", Attempts: 1},
		},
		SkillsUsed: digestSkills,
	}

	yamlContent, err := BuildDigest(digest)
	if err != nil {
		return fmt.Errorf("consolidation.Handle: build digest: %w", err)
	}

	topicKey := "digest/" + payload.ChangeID
	tk := topicKey
	if err := h.memory.Ingest(ctx, IngestRequest{
		TopicKey: tk,
		Type:     "semantic",
		Content:  yamlContent,
		Tags:     []string{"change_digest"},
	}); err != nil {
		return fmt.Errorf("consolidation.Handle: ingest digest: %w", err)
	}

	h.log.InfoContext(ctx, "consolidation: pipeline complete",
		slog.String("change_id", payload.ChangeID),
		slog.Int("skills_processed", len(deltas)),
	)
	return nil
}

// skillDelta holds the computed delta for a single skill.
type skillDelta struct {
	SkillID string
	Delta   outbound.MetricsDelta
}

// computeDeltas calculates per-skill metric deltas from the usage rows.
// D-M2-11: avg_retry_reduction proxy = (1.5 - apply_attempts) / 1.5.
func computeDeltas(rows []outbound.SkillUsageRow) []skillDelta {
	// Group rows by skill_id; aggregate outcome counts.
	type counts struct {
		success      int
		failure      int
		applyAttempts int
	}
	aggregated := make(map[string]*counts)
	order := make([]string, 0) // preserve first-seen order for deterministic output

	for _, row := range rows {
		if _, seen := aggregated[row.SkillID]; !seen {
			aggregated[row.SkillID] = &counts{}
			order = append(order, row.SkillID)
		}
		c := aggregated[row.SkillID]
		switch row.Outcome {
		case "success":
			c.success++
		case "failure":
			c.failure++
		}
		if row.ApplyAttempts > c.applyAttempts {
			c.applyAttempts = row.ApplyAttempts
		}
	}

	deltas := make([]skillDelta, 0, len(order))
	for _, skillID := range order {
		c := aggregated[skillID]

		// D-M2-11: avg_retry_reduction proxy
		attempts := float64(c.applyAttempts)
		if attempts == 0 {
			attempts = 1 // default baseline
		}
		avgRetryReduction := (1.5 - attempts) / 1.5

		successDelta := c.success
		failureDelta := c.failure
		testsPassed := 0
		if c.success > 0 {
			testsPassed = 1
		}

		deltas = append(deltas, skillDelta{
			SkillID: skillID,
			Delta: outbound.MetricsDelta{
				SuccessDelta:     successDelta,
				FailureDelta:     failureDelta,
				TestsPassedDelta: testsPassed,
				UsageDelta:       successDelta + failureDelta,
				AvgRetryReduction: avgRetryReduction,
			},
		})
	}
	return deltas
}
