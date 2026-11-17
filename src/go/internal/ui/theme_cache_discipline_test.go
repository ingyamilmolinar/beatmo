//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// themeSafeSpriteCaches classifies every package-level sprite cache
// (map[...]*ebiten.Image) by WHY it cannot serve a previous theme's pixels.
// A package-level cache outlives every Game, so Game.OnThemeChanged reaches it
// only if someone remembered to wire it — the wheel sprites were not, and kept
// the boot theme's surfaces forever. Adding a new one fails this test until it
// is classified here with a reason (and actually made theme-safe).
var themeSafeSpriteCaches = map[string]string{
	"pixelCache":         "keyed by the fill COLOR — a new theme color is a new key",
	"cornerSpriteCache":  "keyed by the corner COLOR",
	"roundedBtnCache":    "keyed by fill + border COLOR",
	"iconSpriteCache":    "colorless glyph mask, tinted per draw via ColorScale",
	"textSprites":        "colorless glyph mask, tinted per draw via ColorScale",
	"styledSprites":      "colorless glyph mask, tinted per draw via ColorScale",
	"addRowBtnCache":     "dropped by Game.OnThemeChanged (invalidateAddRowBtnCache)",
	"wheelCylinderCache": "theme-scoped: syncWheelSpriteTheme drops it on a ThemeGeneration change",
	"wheelGripCache":     "theme-scoped: syncWheelSpriteTheme drops it on a ThemeGeneration change",
}

// TestPackageLevelSpriteCachesAreThemeSafe — AST scan of internal/ui for
// package-level vars whose type is map[...]*ebiten.Image.
func TestPackageLevelSpriteCachesAreThemeSafe(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := map[string]string{}
	isSpriteMap := func(e ast.Expr) bool {
		m, ok := e.(*ast.MapType)
		if !ok {
			return false
		}
		star, ok := m.Value.(*ast.StarExpr)
		if !ok {
			return false
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Image"
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, d := range af.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, name := range vs.Names {
					typed := vs.Type != nil && isSpriteMap(vs.Type)
					if !typed && i < len(vs.Values) {
						if cl, ok := vs.Values[i].(*ast.CompositeLit); ok && isSpriteMap(cl.Type) {
							typed = true
						}
					}
					if typed {
						found[name.Name] = fset.Position(name.Pos()).String()
					}
				}
			}
		}
	}
	var missing []string
	for name, pos := range found {
		if _, ok := themeSafeSpriteCaches[name]; !ok {
			missing = append(missing, pos+" "+name)
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("unclassified package-level sprite cache %s — key it on the color, make it a colorless mask, or "+
			"drop it on a ThemeGeneration change; then classify it in themeSafeSpriteCaches", m)
	}
	for name := range themeSafeSpriteCaches {
		if _, ok := found[name]; !ok {
			t.Errorf("themeSafeSpriteCaches lists %q but no such package-level sprite cache exists — remove the entry", name)
		}
	}
}
