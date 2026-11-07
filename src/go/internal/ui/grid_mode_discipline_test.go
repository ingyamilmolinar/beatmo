package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every other file goes through Enter/Cancel/Commit/Tick/Register, keeping the mode domain in one place.
func TestGridModeManagerFieldWritesOnlyInOwnFile(t *testing.T) {
	// Best effort: an aliased local (m := &g.modes; m.kind = x) slips past the regex.
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
