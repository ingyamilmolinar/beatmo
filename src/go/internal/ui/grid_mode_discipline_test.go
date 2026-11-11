package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// gridModeManager fields may only be written inside grid_mode.go — every
// other file must go through Enter/Cancel/Commit/Tick/Register. This keeps
// the mode domain centralized (see the design spec § Centralized mode domain).
func TestGridModeManagerFieldWritesOnlyInOwnFile(t *testing.T) {
	// Known gap: best-effort grep — an aliased local (`m := &g.modes; m.kind = x`)
	// or other non-dot-qualified access would slip past this regex.
	writeRe := regexp.MustCompile(`\.modes\.(kind|subject|phase|observers)\s*(=|\+\+|--|\+=|-=)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "grid_mode.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for ln, line := range strings.Split(string(data), "\n") {
			if writeRe.MatchString(line) {
				t.Errorf("%s:%d writes gridModeManager state directly: %s", f, ln+1, strings.TrimSpace(line))
			}
		}
	}
}
