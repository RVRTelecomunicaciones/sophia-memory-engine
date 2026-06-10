package consolidation_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoLLMImportsInConsolidation verifies that D-M2-12 is enforced:
// no LLM client packages (openai, anthropic, llm-related) appear in the
// consolidation package's dependency tree.
//
// This uses `go list -deps` to enumerate all transitive dependencies of the
// consolidation package and fails if any LLM-related import path is found.
// golangci-lint's `forbidigo` rule provides a second layer of enforcement.
func TestNoLLMImportsInConsolidation(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps",
		"github.com/sophia-engine/memory-engine/internal/application/consolidation",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list -deps failed (may be a CI environment without full module cache): %v", err)
	}

	deps := string(out)
	banned := []string{
		"openai",
		"anthropic",
		"llm",
		"gpt",
		"claude",
	}

	for _, b := range banned {
		if strings.Contains(strings.ToLower(deps), b) {
			t.Errorf("D-M2-12 VIOLATION: LLM import path containing %q found in consolidation dependencies:\n%s",
				b, filterDeps(deps, b))
		}
	}
}

// filterDeps returns only the dependency lines that contain the substring.
func filterDeps(deps, substring string) string {
	var matched []string
	for _, line := range strings.Split(deps, "\n") {
		if strings.Contains(strings.ToLower(line), substring) {
			matched = append(matched, line)
		}
	}
	return strings.Join(matched, "\n")
}
