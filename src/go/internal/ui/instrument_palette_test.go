package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Catches a DESIGN.md edit without `make gen-design-tokens`, or a generator that reorders.
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

// Both keys on consecutive lines, spaces only between tokens: `\s` would let a stray newline merge two entries.
var instrumentSwatchEntryRE = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+name:[ \t]*([a-z][a-z0-9-]*)[ \t]*\r?\n[ \t]+hex:[ \t]*"(#[0-9A-Fa-f]{6}(?:[0-9A-Fa-f]{2})?)"`)

// A regex, not YAML: only {name, hex} pairs are needed, and yaml.v3 would add a dependency.
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
