package consolidation

// FilterDigestSkills returns a copy of in with all entries whose Outcome is
// "unknown" removed. "unknown" marks a GetSkill-failed / corrupt row
// (D-LH-4) and must never reach the persisted change_digest.
//
// Every other outcome is retained — including never-applied / low-but-real
// signal entries — because availability without use is matcher signal.
//
// The function is pure: the input slice is never mutated, input order is
// preserved among retained entries, and the result is always non-nil.
// BuildDigest performs the final sort, so this filter only decides membership.
func FilterDigestSkills(in []DigestSkill) []DigestSkill {
	out := make([]DigestSkill, 0, len(in))
	for _, s := range in {
		if s.Outcome == "unknown" {
			continue
		}
		out = append(out, s)
	}
	return out
}
