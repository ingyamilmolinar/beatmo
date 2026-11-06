package assets

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The shipped faces are subsets of stock Inter (see font_embed.go). Subsetting
// is the largest download saving available, but it fails SILENTLY: a dropped
// codepoint renders as .notdef -- a tofu box -- and no test that only exercises
// layout would notice. These tests pin the floor, so narrowing KEEP_RANGES in
// scripts/subset_fonts.mjs breaks the build instead of the UI.
//
// The cmap is decoded here with nothing but encoding/binary rather than through
// x/image/font/opentype, because the fast test path (-tags test -modfile
// go.test.mod) stubs Ebiten and does not carry x/image. A guard that only ran on
// the real-Ebiten path would miss most CI runs -- and this way the assertions
// read the shipped bytes directly instead of a library's interpretation of them.

// requiredRunes is every codepoint the UI is known to draw, grouped by why it
// must survive so a future edit to the subset ranges can see what it would cost.
var requiredRunes = []struct {
	why    string
	sample string
}{
	{"ASCII printable -- every label, number and keycap",
		" !\"#$%&'()*+,-./0123456789:;<=>?@" +
			"ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`" +
			"abcdefghijklmnopqrstuvwxyz{|}~"},
	{"Spanish locale (i18n/catalog_es.go) -- accents, tilde, diaeresis, inverted marks",
		"áéíóúÁÉÍÓÚñÑüÜ¿¡"},
	{"template display titles (templates.go displayTitles)",
		"ÁÓáäèéêíñóöúř—’"},
	{"units and math in DSP / analyzer labels",
		"°±²³µ·×÷¼⅓⅔−≈≠≤≥∞√Δ"},
	{"typographic punctuation in menu and stage labels",
		"–—’“”…‹›·"},
	// Only symbols Beatmo actually draws AS TEXT belong here. The chrome glyphs
	// DESIGN.md forbids (▶ ▼ ▲ ✕ ≡ ⏸ ■ and the arrows) go through IconID +
	// DrawIcon, and stock Inter has no glyph for several of them anyway
	// (U+2715 ✕, U+2261 ≡, U+2669 ♩ are absent upstream) -- listing one here
	// would assert coverage the full font never had.
	{"symbols drawn as text (favorite star, check mark)",
		"★✓"},
}

// --- minimal sfnt cmap reader -------------------------------------------------

func be16(b []byte, i int) int { return int(binary.BigEndian.Uint16(b[i:])) }
func be32(b []byte, i int) int { return int(binary.BigEndian.Uint32(b[i:])) }

// cmapCoverage returns the set of codepoints the font maps to a real glyph.
func cmapCoverage(t *testing.T, name string, ttf []byte) map[rune]bool {
	t.Helper()
	if len(ttf) < 12 {
		t.Fatalf("%s: embedded font is empty or truncated (%d bytes)", name, len(ttf))
	}
	numTables := be16(ttf, 4)
	var cmap []byte
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		if rec+16 > len(ttf) {
			t.Fatalf("%s: table directory truncated", name)
		}
		if string(ttf[rec:rec+4]) == "cmap" {
			off, length := be32(ttf, rec+8), be32(ttf, rec+12)
			if off+length > len(ttf) {
				t.Fatalf("%s: cmap table out of bounds", name)
			}
			cmap = ttf[off : off+length]
			break
		}
	}
	if cmap == nil {
		t.Fatalf("%s: no cmap table", name)
	}

	out := map[rune]bool{}
	for i := 0; i < be16(cmap, 2); i++ {
		rec := 4 + 8*i
		platform, encoding := be16(cmap, rec), be16(cmap, rec+2)
		// Unicode (0, *) and Windows Unicode BMP/full (3,1) and (3,10).
		if !(platform == 0 || (platform == 3 && (encoding == 1 || encoding == 10))) {
			continue
		}
		sub := cmap[be32(cmap, rec+4):]
		switch be16(sub, 0) {
		case 4:
			readCmap4(sub, out)
		case 12:
			readCmap12(sub, out)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: cmap decoded to zero codepoints", name)
	}
	return out
}

func readCmap4(sub []byte, out map[rune]bool) {
	segCount := be16(sub, 6) / 2
	endBase := 14
	startBase := endBase + segCount*2 + 2
	deltaBase := startBase + segCount*2
	rangeBase := deltaBase + segCount*2
	for s := 0; s < segCount; s++ {
		start, end := be16(sub, startBase+s*2), be16(sub, endBase+s*2)
		if start > end || end == 0xFFFF && start == 0xFFFF {
			continue
		}
		delta := int(int16(binary.BigEndian.Uint16(sub[deltaBase+s*2:])))
		ro := be16(sub, rangeBase+s*2)
		for c := start; c <= end; c++ {
			var gid int
			if ro == 0 {
				gid = (c + delta) & 0xFFFF
			} else {
				idx := rangeBase + s*2 + ro + (c-start)*2
				if idx+2 > len(sub) {
					continue
				}
				gid = be16(sub, idx)
				if gid != 0 {
					gid = (gid + delta) & 0xFFFF
				}
			}
			if gid != 0 {
				out[rune(c)] = true
			}
		}
	}
}

func readCmap12(sub []byte, out map[rune]bool) {
	for g := 0; g < be32(sub, 12); g++ {
		rec := 16 + g*12
		if rec+12 > len(sub) {
			return
		}
		start, end, gid := be32(sub, rec), be32(sub, rec+4), be32(sub, rec+8)
		if gid == 0 || end < start || end-start > 0x10FFFF {
			continue
		}
		for c := start; c <= end; c++ {
			out[rune(c)] = true
		}
	}
}

func embeddedFaces(t *testing.T) map[string]map[rune]bool {
	t.Helper()
	return map[string]map[rune]bool{
		"Inter-Regular.subset":  cmapCoverage(t, "Inter-Regular.subset", GoRegularTTF),
		"Inter-SemiBold.subset": cmapCoverage(t, "Inter-SemiBold.subset", GoBoldTTF),
	}
}

func sortedNames(m map[string]map[rune]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- the guards ---------------------------------------------------------------

// TestEmbeddedFontsCoverRequiredRunes fails if the subset dropped a codepoint the
// UI renders. A miss here ships a tofu box to users.
func TestEmbeddedFontsCoverRequiredRunes(t *testing.T) {
	faces := embeddedFaces(t)
	for _, name := range sortedNames(faces) {
		cov := faces[name]
		for _, group := range requiredRunes {
			var missing []string
			for _, r := range group.sample {
				if !cov[r] {
					missing = append(missing, fmt.Sprintf("%q(U+%04X)", r, r))
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s is missing %d rune(s) needed for %s: %s\n"+
					"\twiden KEEP_RANGES in scripts/subset_fonts.mjs, then `make gen-fonts`",
					name, len(missing), group.why, strings.Join(missing, " "))
			}
		}
	}
}

// TestEmbeddedFontsAreSubsets guards the other direction: re-pointing the embed
// at the full upstream TTFs silently regresses the download by ~160 KiB gzip.
// Stock Inter reaches 2,851 codepoints; a subset must be well under that while
// still being a usable face.
func TestEmbeddedFontsAreSubsets(t *testing.T) {
	const (
		minCodepoints = 600  // below this the subset has lost real coverage
		maxCodepoints = 1600 // near 2,851 the full upstream face slipped back in
	)
	faces := embeddedFaces(t)
	for _, name := range sortedNames(faces) {
		n := len(faces[name])
		switch {
		case n < minCodepoints:
			t.Errorf("%s: only %d reachable codepoints (< %d) -- subset is too narrow",
				name, n, minCodepoints)
		case n > maxCodepoints:
			t.Errorf("%s: %d reachable codepoints (> %d) -- this looks like the FULL "+
				"upstream Inter, not a subset; run `make gen-fonts` and keep the embed "+
				"pointed at fonts/*.subset.ttf", name, n, maxCodepoints)
		default:
			t.Logf("%s: %d reachable codepoints", name, n)
		}
	}
}

// TestEmbeddedFontFacesAgree keeps Regular and SemiBold in lockstep: text
// measured in one weight and drawn in the other must not hit a glyph that only
// one face has.
func TestEmbeddedFontFacesAgree(t *testing.T) {
	faces := embeddedFaces(t)
	reg, bold := faces["Inter-Regular.subset"], faces["Inter-SemiBold.subset"]
	var onlyReg, onlyBold []string
	for r := range reg {
		if !bold[r] && len(onlyReg) < 12 {
			onlyReg = append(onlyReg, fmt.Sprintf("U+%04X", r))
		}
	}
	for r := range bold {
		if !reg[r] && len(onlyBold) < 12 {
			onlyBold = append(onlyBold, fmt.Sprintf("U+%04X", r))
		}
	}
	if len(onlyReg) > 0 || len(onlyBold) > 0 {
		sort.Strings(onlyReg)
		sort.Strings(onlyBold)
		t.Errorf("subset faces disagree: only-Regular=[%s] only-SemiBold=[%s]\n"+
			"\tboth faces must be generated from the same KEEP_RANGES",
			strings.Join(onlyReg, " "), strings.Join(onlyBold, " "))
	}
}

// TestEmbeddedFontsCoverEveryShippedString is the guard that cannot drift: it
// walks EVERY key of EVERY shipped locale and asserts the subset can draw every
// rune in the translated string. requiredRunes above is a hand-written floor for
// text that never goes through i18n (template titles, unit suffixes); this one
// tracks the catalogs automatically, so adding a Spanish string with a new
// accented character fails here rather than shipping a tofu box.
//
// i18n is safe to import from this test: it has no beatmo-internal dependencies,
// so there is no cycle back to assets.
func TestEmbeddedFontsCoverEveryShippedString(t *testing.T) {
	restore := i18n.ActiveLocale()
	t.Cleanup(func() { i18n.SetLocale(restore) })

	faces := embeddedFaces(t)
	for _, loc := range i18n.AllLocales() {
		i18n.SetLocale(loc)
		// rune -> one key that uses it, so a failure names where to look.
		seen := map[rune]i18n.Key{}
		for _, k := range i18n.AllKeys() {
			for _, r := range i18n.T(k) {
				if _, dup := seen[r]; !dup {
					seen[r] = k
				}
			}
		}
		if len(seen) == 0 {
			t.Fatalf("locale %q produced no strings -- AllKeys/T wiring changed?", loc)
		}
		for _, name := range sortedNames(faces) {
			cov := faces[name]
			var missing []string
			for r, k := range seen {
				if r == '\n' || r == '\t' {
					continue
				}
				if !cov[r] {
					missing = append(missing, fmt.Sprintf("%q(U+%04X, first used by %s)", r, r, k))
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				t.Errorf("%s cannot draw %d rune(s) used by locale %q:\n\t%s\n"+
					"\twiden KEEP_RANGES in scripts/subset_fonts.mjs, then `make gen-fonts`",
					name, len(missing), loc, strings.Join(missing, "\n\t"))
			}
		}
		t.Logf("locale %q: %d distinct runes, all drawable", loc, len(seen))
	}
}
