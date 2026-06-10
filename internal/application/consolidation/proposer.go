package consolidation

import (
	"context"
	"fmt"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"gopkg.in/yaml.v3"
)

// SkillActivationProposal is the governance pending record emitted when a
// validated skill reaches usage_count >= 5. Per V4.1 §9 verbatim.
// The proposal is stored as YAML in memory-engine.
type SkillActivationProposal struct {
	SkillID         string              `yaml:"skill_id"`
	Version         string              `yaml:"version"`
	ProposedBy      string              `yaml:"proposed_by"`
	ProposedAt      time.Time           `yaml:"proposed_at"`
	EvidenceChanges []string            `yaml:"evidence_changes"`
	Metrics         outbound.SkillMetrics `yaml:"metrics_snapshot"`
}

// proposalUsageThreshold is the minimum usage_count for a validated skill to
// trigger a governance proposal (Q1 operator-locked value per D-M2-8).
const proposalUsageThreshold = 5

// Proposer emits a SkillActivationProposal to memory-engine pending governance
// storage when a validated skill reaches the usage threshold (D-M2-8).
// It satisfies the SkillProposer interface.
//
// Idempotency: re-emission reads the existing proposal from memory-engine,
// appends the new change_id to EvidenceChanges, updates the metrics snapshot,
// and overwrites via topic_key upsert (no duplicate records).
type Proposer struct {
	memory MemoryClient
	clock  shared.Clock
}

// NewProposer constructs a Proposer.
func NewProposer(memory MemoryClient, clock shared.Clock) *Proposer {
	return &Proposer{memory: memory, clock: clock}
}

// Emit evaluates snap and, if conditions are met, writes a SkillActivationProposal
// to memory-engine at governance/skill-proposal/{skill_id}.
// Returns nil without emitting when:
//   - snap.Status != "validated"
//   - snap.Metrics.UsageCount < proposalUsageThreshold
func (p *Proposer) Emit(ctx context.Context, snap *outbound.SkillSnapshot, changeID string) error {
	if snap.Status != "validated" {
		return nil
	}
	if snap.Metrics.UsageCount < proposalUsageThreshold {
		return nil
	}

	topicKey := fmt.Sprintf("governance/skill-proposal/%s", snap.SkillID)

	// Idempotent re-emission: read existing proposal and append evidence.
	evidenceChanges := []string{changeID}

	existingContent, readErr := p.memory.ReadContent(ctx, topicKey)
	if readErr == nil && existingContent != "" {
		// Parse the existing proposal to extract and merge evidence_changes.
		var existing SkillActivationProposal
		if parseErr := yaml.Unmarshal([]byte(existingContent), &existing); parseErr == nil {
			// Append the new change_id to the existing evidence list (dedup not required per spec).
			merged := make([]string, len(existing.EvidenceChanges), len(existing.EvidenceChanges)+1)
			copy(merged, existing.EvidenceChanges)
			merged = append(merged, changeID)
			evidenceChanges = merged
		}
	}

	proposal := SkillActivationProposal{
		SkillID:         snap.SkillID,
		Version:         snap.Version,
		ProposedBy:      "archive_worker",
		ProposedAt:      p.clock.Now(),
		EvidenceChanges: evidenceChanges,
		Metrics:         snap.Metrics,
	}

	content, err := yaml.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("proposer.Emit: marshal proposal: %w", err)
	}

	return p.memory.Ingest(ctx, IngestRequest{
		TopicKey: topicKey,
		Type:     "semantic",
		Content:  string(content),
		Tags:     []string{"governance", "skill_proposal", "pending"},
	})
}
