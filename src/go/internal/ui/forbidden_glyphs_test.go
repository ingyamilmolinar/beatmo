// Code review follow-up (P2.1): forbidden-glyph audit, deferred during the
// original Phase 6 of the design-system refactor. The plan specified that
// DESIGN.md's permitted-text-glyph table (lines 1152-1166) be machine-
// enforced — every raw chrome glyph must route through IconID, with a
// short allowlist of ASCII / mathematical exceptions. This test is the
// forcing function.
//
// We parse every internal/ui/*.go source file's AST and inspect string
// literals only — comments and identifiers are exempt by construction.
// A forbidden Unicode glyph that appears as a UI label triggers a
// failure with the precise file:line for the offending literal.

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenChromeGlyphs lists Unicode characters that must never appear
// in a user-facing string literal under internal/ui/. The set mirrors
// CLAUDE.md's "Forbidden glyphs" list and DESIGN.md §icons-and-glyphs.
// New entries should also be added to internal/ui/icons.go as IconID
// values so call sites can drawIconByID instead of typing the rune.
var forbiddenChromeGlyphs = []rune{
	'▶', // play — IconPlay
	'▼', // chevron-down — IconChevronDown
	'▲', // chevron-up — IconChevronUp
	'✕', // close — IconClose
	'≡', // overflow / hamburger — IconOverflow
	'⏸', // pause — IconPause
	'■', // stop — IconStop
	'↑', // arrow-up — IconChevronUp / IconUpload
	'↓', // arrow-down — IconChevronDown
	'→', // arrow-right — IconChevronRight
	'←', // arrow-left — IconChevronRight (mirrored)
	'›', // single-right-pointing-quotation — IconChevronRight (breadcrumb separator)
	'‹', // single-left-pointing-quotation — IconChevronLeft
}

// permittedLiterals enumerates string literals that DESIGN.md's permitted-
// text-glyph table (DESIGN.md:1152-1166) explicitly allows. Each entry is
// the exact literal text a Go source file may contain. Adding to this set
// requires a corresponding row in DESIGN.md and a justification line in
// the table.
//
// Note: this set is intentionally narrow. If a literal contains a glyph
// from forbiddenChromeGlyphs above, it is rejected even if it appears
// here — the two lists must be disjoint by construction.
var permittedLiterals = map[string]string{
	// Subdivision label on the transport bar; ÷ is a math operator, not a
	// chrome glyph. The full literal varies (÷4, ÷8, ÷16, ÷32) and is built
	// at runtime, so we whitelist the prefix only.
	"÷": "transport_zone.go subdivision label (DESIGN.md:1163)",
	// Freeze indicators (analyzer-tab freeze pill + Chain freeze button) now
	// render IconPause (live) / IconPlay (frozen) via applyFreezeVisual — the
	// old ">" / "||" text-glyph exception was retired.
}

// TestNoForbiddenChromeGlyphs walks every non-test, non-generated source
// file under internal/ui/ and asserts that no string literal contains a
// rune from forbiddenChromeGlyphs. It uses go/parser (not byte grep) so
// that runes appearing in comments — including arrows in
// "Layout → Update" prose — are correctly ignored.
func TestNoForbiddenChromeGlyphs(t *testing.T) {
	pkgDir := "."
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("readdir %s: %v", pkgDir, err)
	}

	type offense struct {
		file string
		pos  token.Position
		lit  string
		rune rune
	}
	var offenses []offense

	fset := token.NewFileSet()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		// Skip tests, generated files, and the icon definitions
		// themselves (which list the glyphs as documentation).
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if strings.HasSuffix(name, ".gen.go") {
			continue
		}
		if name == "icons.go" {
			continue
		}

		path := filepath.Join(pkgDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			// Strip the surrounding quotes / backticks. Use the raw value
			// rather than strconv.Unquote — we want the source bytes
			// exactly as written.
			val := lit.Value
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
			for _, fr := range forbiddenChromeGlyphs {
				if strings.ContainsRune(val, fr) {
					offenses = append(offenses, offense{
						file: name,
						pos:  fset.Position(lit.Pos()),
						lit:  val,
						rune: fr,
					})
					break
				}
			}
			return true
		})
	}

	if len(offenses) == 0 {
		return
	}

	for _, o := range offenses {
		t.Errorf("%s: forbidden glyph %q in string literal %q — route through IconID + DrawIcon (DESIGN.md §icons-and-glyphs)",
			o.pos, o.rune, o.lit)
	}
}

// TestPermittedGlyphsAreNotForbidden enforces the construction invariant
// that the two sets above are disjoint: a glyph cannot be both forbidden
// and permitted. This catches inadvertent rule contradictions before
// they confuse a future contributor reading the test output.
func TestPermittedGlyphsAreNotForbidden(t *testing.T) {
	for lit := range permittedLiterals {
		for _, fr := range forbiddenChromeGlyphs {
			if strings.ContainsRune(lit, fr) {
				t.Errorf("permitted literal %q contains forbidden rune %q — fix one or the other", lit, fr)
			}
		}
	}
}
