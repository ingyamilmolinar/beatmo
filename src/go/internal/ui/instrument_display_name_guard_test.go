//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestInstrumentNameInlineTitleCaseGuard prevents the regression where
// drumview code derives a row's display Name from an instrument ID via
// strings.ToUpper(id[:1]) + id[1:]. That pattern leaks separators —
// "hi-hat" becomes "Hi-hat" instead of "Hi Hat". The unified path is
// dv.computeInstLabel(id) (or audio.PrettyName for one-shot use); see
// drumview_instrument_helpers.go.
//
// The guard scans drumview_*.go (excluding tests) for the canonical
// leak shape — strings.ToUpper(<ident>[:1]) + <ident>[1:] where the
// same identifier appears on both sides — and fails if any are found.
// Unrelated patterns (a one-off capitalize() helper in drumview_fx_panel.go
// for effect parameter labels) use a local helper, not the inline form,
// and are not flagged.
func TestInstrumentNameInlineTitleCaseGuard(t *testing.T) {
	pattern := regexp.MustCompile(`strings\.ToUpper\([a-zA-Z_][a-zA-Z0-9_.]*\[:1\]\)\s*\+\s*[a-zA-Z_][a-zA-Z0-9_.]*\[1:\]`)

	matches, err := filepath.Glob("drumview_*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one drumview_*.go file in cwd")
	}

	var hits []string
	for _, p := range matches {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if pattern.MatchString(line) {
				hits = append(hits, p+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
	}
	if len(hits) > 0 {
		t.Fatalf("inline title-case pattern leaked back into drumview_*.go:\n%s\n\n"+
			"Use dv.computeInstLabel(id) (or audio.PrettyName(id) outside DrumView) instead.",
			strings.Join(hits, "\n"))
	}
}

