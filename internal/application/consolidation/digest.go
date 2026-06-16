package consolidation

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// ChangeDigest is the deterministic YAML record written at topic_key=digest/{change_id}
// per V4.1 §13.1. Phases are sorted ascending by PhaseType; SkillsUsed by SkillID.
// The LLM-assisted variant (V4.1 §13.2) is FORBIDDEN in M2 — no LLM imports here.
type ChangeDigest struct {
	ChangeID        string        `yaml:"change_id"`
	ProjectID       string        `yaml:"project_id"`
	DurationSeconds int64         `yaml:"duration_seconds"`
	Phases          []DigestPhase `yaml:"phases"`
	SkillsUsed      []DigestSkill `yaml:"skills_used"`
	ErrorsResolved  []DigestError `yaml:"errors_resolved,omitempty"`
}

// DigestPhase captures per-phase summary data.
type DigestPhase struct {
	Phase        string   `yaml:"phase"`
	Status       string   `yaml:"status"`
	Attempts     int      `yaml:"attempts"`
	RetryReasons []string `yaml:"retry_reasons,omitempty"`
}

// Digest outcome values for a per-skill entry in the change digest.
// Shared by the producer (handler step 4-9) and the FilterDigestSkills
// consumer so the literals cannot drift apart.
const (
	// OutcomeSuccess marks a skill whose post-patch state was read cleanly.
	OutcomeSuccess = "success"
	// OutcomeFailure marks a skill whose PatchMetrics step failed.
	OutcomeFailure = "failure"
	// OutcomeUnknown marks a GetSkill-failed / corrupt row. Filtered out of
	// the persisted digest (D-LH-4).
	OutcomeUnknown = "unknown"
)

// DigestSkill captures per-skill outcome in the change.
type DigestSkill struct {
	SkillID string `yaml:"skill_id"`
	Outcome string `yaml:"outcome"`
}

// DigestError captures a resolved error entry (M4+ will populate from phase envelopes).
type DigestError struct {
	Message string `yaml:"message"`
}

// IngestRequest is the payload the Handler passes to MemoryClient.Ingest.
// Field names mirror the inbound.IngestMemoryCmd vocabulary so the memory-engine
// HTTP adapter can translate them without a dedicated port type.
type IngestRequest struct {
	TopicKey string
	Type     string
	Content  string
	Tags     []string
}

// BuildDigest produces a deterministic YAML string from the given ChangeDigest.
// It sorts Phases by Phase (ascending) and SkillsUsed by SkillID (ascending)
// before encoding so output bytes are identical for identical inputs.
func BuildDigest(d ChangeDigest) (string, error) {
	// Deep-copy slices before sorting to avoid mutating the caller's data.
	phases := make([]DigestPhase, len(d.Phases))
	copy(phases, d.Phases)
	sort.Slice(phases, func(i, j int) bool { return phases[i].Phase < phases[j].Phase })

	skills := make([]DigestSkill, len(d.SkillsUsed))
	copy(skills, d.SkillsUsed)
	sort.Slice(skills, func(i, j int) bool { return skills[i].SkillID < skills[j].SkillID })

	sorted := ChangeDigest{
		ChangeID:        d.ChangeID,
		ProjectID:       d.ProjectID,
		DurationSeconds: d.DurationSeconds,
		Phases:          phases,
		SkillsUsed:      skills,
		ErrorsResolved:  d.ErrorsResolved,
	}

	b, err := yaml.Marshal(sorted)
	if err != nil {
		return "", fmt.Errorf("consolidation.BuildDigest: marshal: %w", err)
	}
	return string(b), nil
}
