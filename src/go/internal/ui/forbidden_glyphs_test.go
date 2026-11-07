// Raw chrome glyphs must route through IconID; only DESIGN.md's permitted text-glyph exceptions may appear in
// string literals. The AST is parsed, so comments and identifiers are exempt by construction.

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

// Each entry also needs an IconID so call sites draw the icon instead of typing the rune.
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

// Each entry needs its row in DESIGN.md's permitted text-glyph table. A literal holding a forbidden glyph is
// rejected even if listed here.
var permittedLiterals = map[string]string{
	// ÷ is a math operator, not chrome; the label is built at runtime (÷4…÷32), so only the prefix is listed.
	"÷": "transport_zone.go subdivision label (DESIGN.md:1163)",
}

// go/parser, not grep, so glyphs in comments ("Layout → Update") are ignored.
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
		// The icon definitions list the glyphs as documentation.
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
			// The raw literal, not strconv.Unquote: the check is on the source bytes as written.
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

func TestPermittedGlyphsAreNotForbidden(t *testing.T) {
	for lit := range permittedLiterals {
		for _, fr := range forbiddenChromeGlyphs {
			if strings.ContainsRune(lit, fr) {
				t.Errorf("permitted literal %q contains forbidden rune %q — fix one or the other", lit, fr)
			}
		}
	}
}
