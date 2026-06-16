package consolidation_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/stretchr/testify/assert"
)

// Group K — filterDigestSkills drops ONLY Outcome=="unknown" entries.
//
// Operator decision (D-LH-4): "unknown" outcomes are GetSkill-failed/corrupt
// rows and MUST be dropped before serialization. Never-applied / low-but-real
// signal entries (any non-"unknown" outcome) are RETAINED, since availability
// is matcher signal. Order MUST be preserved (BuildDigest sorts later).
func TestFilterDigestSkills(t *testing.T) {
	tests := []struct {
		name string
		in   []consolidation.DigestSkill
		want []consolidation.DigestSkill
	}{
		{
			name: "drops only unknown, retains success and failure",
			in: []consolidation.DigestSkill{
				{SkillID: "aaa", Outcome: "success"},
				{SkillID: "bbb", Outcome: "unknown"},
				{SkillID: "ccc", Outcome: "failure"},
			},
			want: []consolidation.DigestSkill{
				{SkillID: "aaa", Outcome: "success"},
				{SkillID: "ccc", Outcome: "failure"},
			},
		},
		{
			name: "retains never-applied (non-unknown low-signal) outcome",
			in: []consolidation.DigestSkill{
				{SkillID: "aaa", Outcome: "not_applied"},
				{SkillID: "bbb", Outcome: "unknown"},
			},
			want: []consolidation.DigestSkill{
				{SkillID: "aaa", Outcome: "not_applied"},
			},
		},
		{
			name: "preserves input order among retained entries",
			in: []consolidation.DigestSkill{
				{SkillID: "zzz", Outcome: "success"},
				{SkillID: "unk", Outcome: "unknown"},
				{SkillID: "aaa", Outcome: "failure"},
				{SkillID: "mmm", Outcome: "success"},
			},
			want: []consolidation.DigestSkill{
				{SkillID: "zzz", Outcome: "success"},
				{SkillID: "aaa", Outcome: "failure"},
				{SkillID: "mmm", Outcome: "success"},
			},
		},
		{
			name: "empty input yields empty output",
			in:   []consolidation.DigestSkill{},
			want: []consolidation.DigestSkill{},
		},
		{
			name: "nil input yields empty output",
			in:   nil,
			want: []consolidation.DigestSkill{},
		},
		{
			name: "all-unknown input yields empty output",
			in: []consolidation.DigestSkill{
				{SkillID: "aaa", Outcome: "unknown"},
				{SkillID: "bbb", Outcome: "unknown"},
			},
			want: []consolidation.DigestSkill{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidation.FilterDigestSkills(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// The filter must not mutate the caller's slice (pure function).
func TestFilterDigestSkills_DoesNotMutateInput(t *testing.T) {
	in := []consolidation.DigestSkill{
		{SkillID: "aaa", Outcome: "success"},
		{SkillID: "bbb", Outcome: "unknown"},
	}
	_ = consolidation.FilterDigestSkills(in)
	assert.Equal(t, []consolidation.DigestSkill{
		{SkillID: "aaa", Outcome: "success"},
		{SkillID: "bbb", Outcome: "unknown"},
	}, in, "input slice must be unchanged")
}
