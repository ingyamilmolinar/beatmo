package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every colour any token may take. Adding one is a deliberate palette decision; never widen it to pass a rogue
// token.
var allowedPaletteHex = map[string]bool{
	// sunset-gold
	"FFE98A": true, "FFD319": true, "FFB30A": true, "FF9E1F": true, "F58A12": true,
	// tangerine
	"FFC299": true, "FF9E54": true, "FF7A3D": true, "FF5C2A": true, "E84A1F": true,
	// hot-pink
	"FFA8E0": true, "FF6AD5": true, "FF2D9E": true, "EE00DD": true, "C400B5": true,
	// orchid
	"E0A8FF": true, "C24FFF": true, "A030FF": true, "8C1EFF": true, "6E12E0": true,
	// electric-blue
	"A8C8FF": true, "5E8AFF": true, "3A6AF0": true, "2A5FD6": true, "1E47B5": true,
	// cyan
	"9AF0FF": true, "3FE0E8": true, "00C8E0": true, "00A8C9": true, "0E7391": true,
	// mint
	"B3F5C2": true, "6BE89A": true, "3FD67A": true, "2BC85C": true, "1FA84A": true,
	// neon-night neutrals
	"120A1C": true, "1C1230": true, "281A40": true, "382354": true, "1F1438": true,
	"FDF2FF": true, "C9B8D6": true, "8A7A9C": true,
	// achromatic anchors (alpha-only)
	"FFFFFF": true, "000000": true,
}

var colorsLineRe = regexp.MustCompile(`^\s*([a-z][a-z0-9-]*):\s*"#([0-9A-Fa-f]{6})([0-9A-Fa-f]{2})?"`)

// designMDPathForPalette walks up from the test's cwd to the repo root DESIGN.md.
func designMDPathForPalette(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "DESIGN.md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("DESIGN.md not found walking up from cwd")
	return ""
}

func TestPaletteMembership(t *testing.T) {
	raw, err := os.ReadFile(designMDPathForPalette(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	// From `colors:` to the next top-level key (column 0, not a comment).
	inColors := false
	checked := 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "colors:") {
			inColors = true
			continue
		}
		if inColors && len(ln) > 0 && ln[0] != ' ' && ln[0] != '#' && ln[0] != '\t' {
			break // left the colors: block
		}
		if !inColors {
			continue
		}
		m := colorsLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		name, hex := m[1], strings.ToUpper(m[2])
		checked++
		if !allowedPaletteHex[hex] {
			t.Errorf("color token %q = #%s is OFF-PALETTE. Every colors: token must be a Vice City palette member (see allowedPaletteHex / the design spec). Repoint it to the nearest legal member.", name, hex)
		}
	}
	if checked < 100 {
		t.Fatalf("only scanned %d color tokens; expected the full colors: block (~111). Parser/anchor drift?", checked)
	}
}

var instrumentHexLineRe = regexp.MustCompile(`^\s*(?:- )?(?:color|hex):\s*"#([0-9A-Fa-f]{6})([0-9A-Fa-f]{2})?"`)

// The membership lock scans only `colors:`, and the compliance tests check circuits AGAINST the swatches, which
// is circular: an off-palette hex in an instrument block would pass every other test.
func TestInstrumentColorBlocksOnPalette(t *testing.T) {
	raw, err := os.ReadFile(designMDPathForPalette(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	blocks := []string{"instrumentSwatches:", "instrumentDefaults:", "instrumentFallbackPalette:"}
	for _, header := range blocks {
		inBlock := false
		checked := 0
		for _, ln := range lines {
			if strings.HasPrefix(ln, header) {
				inBlock = true
				continue
			}
			if inBlock {
				if len(ln) > 0 && ln[0] != ' ' && ln[0] != '#' && ln[0] != '\t' {
					break
				}
				m := instrumentHexLineRe.FindStringSubmatch(ln)
				if m == nil {
					continue
				}
				hex := strings.ToUpper(m[1])
				checked++
				if !allowedPaletteHex[hex] {
					t.Errorf("%s entry color #%s is OFF-PALETTE. Every instrument color must be a Vice City palette member (see allowedPaletteHex). Repoint it to the nearest legal member.", header, hex)
				}
			}
		}
		if checked == 0 {
			t.Errorf("scanned 0 color entries in %s — block missing or parser drift?", header)
		}
	}
}
