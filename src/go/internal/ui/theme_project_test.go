//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestProjectThemeRoundTrip(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"grid": "cyan-400"}, Tokens: map[string]string{"focus-ring": "#3FD67A"}})
	b, err := g.drum.exportBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"theme"`) {
		t.Fatalf("export lacks theme block: %s", b)
	}
	g.SetCircuitTheme(theme.Doc{})
	if err := g.Import(b); err != nil {
		t.Fatal(err)
	}
	got := g.CircuitTheme()
	if got.Categories["grid"] != "cyan-400" || got.Tokens["focus-ring"] != "#3FD67A" {
		t.Fatalf("import did not restore circuit theme: %+v", got)
	}
	cyan, _ := swatchRGBA("cyan-400")
	h0, _, _ := rgbToHSL(cyan)
	h1, _, _ := rgbToHSL(genColorGridQuarter)
	if hueDistance(h0, h1) > 20 {
		t.Fatalf("imported circuit theme not applied to grid lines: %v", genColorGridQuarter)
	}
}

func TestProjectWithoutThemeExportsNoBlockAndClearsOnImport(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	b, _ := g.drum.exportBytes()
	if strings.Contains(string(b), `"theme"`) {
		t.Fatalf("unthemed project must omit theme: %s", b)
	}
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	if err := g.Import(b); err != nil { // replace-not-merge
		t.Fatal(err)
	}
	if !g.CircuitTheme().IsEmpty() || genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("import of an unthemed project must clear the circuit theme")
	}
}

func TestUndoRestoresCircuitTheme(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.setCircuitThemeCategory("accent", "orchid-300")
	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != orchid {
		t.Fatal("circuit pick not applied")
	}
	if g.undoManager.UndoLabel() == "" {
		t.Fatal("circuit theme pick must be undoable (ScopeDocument kind): no undo step recorded")
	}
	g.undoManager.Undo()
	advanceFrames(g, 2)
	if genColorPrimary != genColorDefaultByName["primary"] || !g.CircuitTheme().IsEmpty() {
		t.Fatalf("undo did not restore: primary=%v circuit=%+v", genColorPrimary, g.CircuitTheme())
	}
	g.undoManager.Redo()
	advanceFrames(g, 2)
	if genColorPrimary != orchid {
		t.Fatal("redo did not re-apply the circuit theme")
	}
}

func TestUndoDetailForTheme(t *testing.T) {
	before := []byte(`{"bpm":120,"instruments":[],"nodes":[]}`)
	after := []byte(`{"bpm":120,"instruments":[],"nodes":[],"theme":{"categories":{"accent":"orchid-300"}}}`)
	d, ok := describeUndoChange(before, after)
	if !ok || d.param != "Theme" || d.new != "accent: orchid-300" || d.old != "accent: (default)" {
		t.Fatalf("detail = %+v ok=%v", d, ok)
	}
}
