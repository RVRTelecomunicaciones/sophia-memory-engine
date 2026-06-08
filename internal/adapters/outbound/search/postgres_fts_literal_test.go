package search_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPostgresFTSIndex_SQLLiteralsUseSimple verifies that the three
// plainto_tsquery / ts_headline call sites in postgres_fts.go use the
// 'simple' dictionary, not 'spanish'.
//
// This is a white-box regression guard: if someone accidentally re-introduces
// a 'spanish' literal in the FTS SQL, this test catches it immediately on
// make test-unit without needing a live database.
//
// The test reads the source file directly because the SQL is assembled inline
// inside Search() and is not exported. This is the standard approach for
// detecting hardcoded string regressions in SQL-heavy Go code.
func TestPostgresFTSIndex_SQLLiteralsUseSimple(t *testing.T) {
	// Locate postgres_fts.go relative to this test file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate source file")
	}
	dir := filepath.Dir(thisFile)
	srcPath := filepath.Join(dir, "postgres_fts.go")

	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read %s: %v", srcPath, err)
	}
	src := string(data)

	cases := []struct {
		name    string
		present string
		absent  string
	}{
		{
			name:    "FTS language is simple, not spanish",
			present: "'simple'",
			absent:  "'spanish'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(src, tc.present) {
				t.Errorf("expected %s to contain SQL literal %q", srcPath, tc.present)
			}
			if strings.Contains(src, tc.absent) {
				t.Errorf("expected %s to NOT contain SQL literal %q (found 'spanish' — run task A.5 to replace with 'simple')", srcPath, tc.absent)
			}
		})
	}
}
