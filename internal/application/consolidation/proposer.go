package consolidation

import (
	"context"
	"fmt"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"gopkg.in/yaml.v3"
)

// ProposerSkillView is a type alias re-exported from outbound for use within
// this package. The canonical definition lives in ports/outbound/skills_client.go
// (D-M3-2). Using the outbound type directly keeps the port contract central.
type ProposerSkillView = outbound.ProposerSkillView

// ProposerSkillsClient is re-exported from outbound for use within this package.
// The canonical definition lives in ports/outbound/skills_client.go (D-M3-2).
type ProposerSkillsClient = outbound.ProposerSkillsClient

// SkillActivationProposal is the governance pending record emitted when a
// validated skill reaches usage_count >= 5. Per V4.1 §9 verbatim.
// The proposal is stored as YAML in memory-engine.
// M3 adds skill_name, scope, applies_when, risk_level (D-M3-2).
type SkillActivationProposal struct {
	SkillID         string                `yaml:"skill_id"`
	SkillName       string                `yaml:"skill_name"`       // NEW (V4.1 §9 / M3)
	Scope           map[string]any        `yaml:"scope,omitempty"`  // NEW (V4.1 §9 / M3)
	AppliesWhen     map[string]any        `yaml:"applies_when,omitempty"` // NEW (V4.1 §9 / M3)
	RiskLevel       string                `yaml:"risk_level"`       // NEW (V4.1 §9 / M3)
	Version         string                `yaml:"version"`
	ProposedBy      string                `yaml:"proposed_by"`
	ProposedAt      time.Time             `yaml:"proposed_at"`
	EvidenceChanges []string              `yaml:"evidence_changes"`
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
//
// When skillsCli is non-nil (M3 path), the proposer fetches the wider view
// (skill_name, scope, applies_when) from orch to populate the full V4.1 §9
// shape. When nil, the narrow snapshot fields are used (legacy/test path).
type Proposer struct {
	memory    MemoryClient
	clock     shared.Clock
	skillsCli ProposerSkillsClient // optional; non-nil enables full §9 shape (M3)
}

// NewProposer constructs a Proposer without the wide-fetch path (M2 compat).
// Use NewProposerWithSkills to enable the full V4.1 §9 shape (M3).
func NewProposer(memory MemoryClient, clock shared.Clock) *Proposer {
	return &Proposer{memory: memory, clock: clock}
}

// NewProposerWithSkills constructs a Proposer that fetches the richer skill view
// from orch for the full V4.1 §9 proposal shape (M3 / D-M3-2).
func NewProposerWithSkills(memory MemoryClient, clock shared.Clock, skillsCli ProposerSkillsClient) *Proposer {
	return &Proposer{memory: memory, clock: clock, skillsCli: skillsCli}
}

// Emit evaluates snap and, if conditions are met, writes a SkillActivationProposal
// to memory-engine at governance/skill-proposal/{skill_id}.
// Returns nil without emitting when:
//   - snap.Status != "validated"
//   - snap.Metrics.UsageCount < proposalUsageThreshold
//
// When a ProposerSkillsClient is configured (M3 path), Emit fetches the wider
// view to populate the full V4.1 §9 shape (skill_name, scope, applies_when,
// risk_level). On wide-fetch error the proposal falls back to the narrow shape
// so the governance record is never silently dropped.
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

	// Resolve wide fields: use SkillsClient when configured (M3), else narrow only.
	var skillName, riskLevel string
	var scope, appliesWhen map[string]any

	riskLevel = snap.RiskLevel // always available from the narrow snapshot

	if p.skillsCli != nil {
		if wide, wideErr := p.skillsCli.GetSkillWide(ctx, snap.SkillID); wideErr == nil {
			skillName = wide.SkillName
			scope = wide.Scope
			appliesWhen = wide.AppliesWhen
		}
		// On wide-fetch error: log omitted (no logger in proposer); proceed with
		// narrow fields. The governance record is still written; operator can
		// retry via re-emit on the next archive event.
	}

	proposal := SkillActivationProposal{
		SkillID:         snap.SkillID,
		SkillName:       skillName,
		Scope:           scope,
		AppliesWhen:     appliesWhen,
		RiskLevel:       riskLevel,
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
