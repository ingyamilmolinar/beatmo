package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestInstrumentSwatchesParity asserts the SuggestedInstrumentSwatches slice
// (re-exported from the generated genInstrumentSwatches) matches the
// `instrumentSwatches:` block in DESIGN.md byte-for-byte. Catches the case
// where someone edits DESIGN.md without re-running `make gen-design-tokens`,
// or where the generator silently changes ordering.
func TestInstrumentSwatchesParity(t *testing.T) {
	path := findDesignMD(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	wantNames, wantHexes := parseInstrumentSwatchBlock(t, string(body))
	if len(wantNames) == 0 {
		t.Fatal("DESIGN.md has no instrumentSwatches: block — runtime expects at least one entry")
	}

	if got, want := len(SuggestedInstrumentSwatches), len(wantNames); got != want {
		t.Fatalf("SuggestedInstrumentSwatches has %d entries; DESIGN.md has %d", got, want)
	}
	for i, sw := range SuggestedInstrumentSwatches {
		if sw.Name != wantNames[i] {
			t.Errorf("entry %d: name = %q, want %q (DESIGN.md ordering must be preserved)",
				i, sw.Name, wantNames[i])
		}
		if !strings.EqualFold(sw.Hex, wantHexes[i]) {
			t.Errorf("entry %d (%s): hex = %q, want %q", i, sw.Name, sw.Hex, wantHexes[i])
		}
	}
}

// Matches YAML block-style entries as written in DESIGN.md:
//
//	- name: coral
//	  hex: "#FF8C5A"
//
// Both keys must be on consecutive lines; spaces only between tokens (no `\s`,
// which would let a stray newline collapse two entries into one).
var instrumentSwatchEntryRE = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+name:[ \t]*([a-z][a-z0-9-]*)[ \t]*\r?\n[ \t]+hex:[ \t]*"(#[0-9A-Fa-f]{6}(?:[0-9A-Fa-f]{2})?)"`)

// parseInstrumentSwatchBlock extracts the names + hexes from the
// `instrumentSwatches:` block. Uses a regex rather than a YAML round-trip
// because the test only needs the {name, hex} pairs and adding yaml.v3 here
// would pull a dep into the runtime test set.
func parseInstrumentSwatchBlock(t *testing.T, body string) ([]string, []string) {
	t.Helper()
	front := frontMatter(body)
	if front == "" {
		t.Fatal("no YAML front matter in DESIGN.md")
	}
	idx := strings.Index(front, "instrumentSwatches:")
	if idx < 0 {
		return nil, nil
	}
	rest := front[idx:]
	matches := instrumentSwatchEntryRE.FindAllStringSubmatch(rest, -1)
	names := make([]string, 0, len(matches))
	hexes := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
		hexes = append(hexes, m[2])
	}
	return names, hexes
}
