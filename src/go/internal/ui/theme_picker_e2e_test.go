//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// swatchCellNamed returns the on-screen cell of the named swatch in the open picker.
func swatchCellNamed(t *testing.T, g *Game, name string) image.Rectangle {
	t.Helper()
	cells := g.drum.colorWheelComp.SwatchCells()
	for i, s := range SuggestedInstrumentSwatches {
		if s.Name == name && i < len(cells) {
			return cells[i]
		}
	}
	t.Fatalf("swatch %q not in picker cells (%d cells)", name, len(cells))
	return image.Rectangle{}
}

// Full path: palette button → overlay → Accent chip → picker → orchid-300
// released → screen recolors, prefs saved, picker closed, overlay still open
// with the chip ringed.
func TestThemePickAccentEndToEnd(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{}
	SetThemeSink(sink)
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	p := g.drum.portal()
	if orchid, _ := swatchRGBA("orchid-300"); genColorPrimary == orchid {
		t.Fatal("precondition: a previous test leaked an orchid accent theme")
	}

	expandThemeAdjust(t, g, ov, tap)
	scrollThemeRowIntoView(t, g, ov, "accent")
	cx, cy := center(ov.ChipRect("accent"))
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if !p.Has(themePickerID) || !g.drum.colorWheelComp.IsOpen() {
		t.Fatal("chip press did not open the swatch picker")
	}
	if !p.Has(themeOverlayID) {
		t.Fatal("theme overlay must stay open under the picker")
	}
	cx, cy = center(swatchCellNamed(t, g, "orchid-300"))
	tap(cx, cy, 3)
	advanceFrames(g, 3)

	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != orchid {
		t.Fatalf("pick did not apply: primary=%v", genColorPrimary)
	}
	if got := g.UserTheme().Categories["accent"]; got != "orchid-300" {
		t.Fatalf("user theme not updated: %q", got)
	}
	if len(sink.saved) == 0 || sink.saved[len(sink.saved)-1].Categories["accent"] != "orchid-300" {
		t.Fatalf("pick not persisted: %+v", sink.saved)
	}
	if p.Has(themePickerID) || g.drum.colorWheelComp.IsOpen() {
		t.Fatal("picker must close after a pick")
	}
	if !p.Has(themeOverlayID) {
		t.Fatal("overlay must remain open after a pick")
	}
	if set, _ := ov.chipState("accent"); !set {
		t.Fatal("chip must show the explicit ring after the pick")
	}
	// The frame actually paints the new accent.
	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })
	screen := ebiten.NewImage(1280, 720)
	rects := collectAllFills(t, func() { g.Draw(screen) })
	if rectsWithColorInside(rects, screen.Bounds(), orchid) == 0 {
		t.Fatal("derived accent not painted on the frame after the pick")
	}
}

// Circuit scope: the pick lands in the circuit theme, is undoable, and the
// user theme is untouched.
func TestThemePickCircuitScopeUndoEndToEnd(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	_, circuit := ov.ScopePillRects()
	cx, cy := center(circuit)
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	expandThemeAdjust(t, g, ov, tap)
	scrollThemeRowIntoView(t, g, ov, "grid")
	cx, cy = center(ov.ChipRect("grid"))
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	cx, cy = center(swatchCellNamed(t, g, "cyan-300"))
	tap(cx, cy, 3)
	advanceFrames(g, 3)
	if g.CircuitTheme().Categories["grid"] != "cyan-300" {
		t.Fatalf("circuit theme: %+v", g.CircuitTheme())
	}
	if len(g.UserTheme().Categories) != 0 {
		t.Fatal("user theme must be untouched by a circuit-scope pick")
	}
	before := genColorGridQuarter
	if g.undoManager.UndoLabel() == "" {
		t.Fatal("circuit pick not undoable")
	}
	g.undoManager.Undo()
	advanceFrames(g, 2)
	if genColorGridQuarter == before || !g.CircuitTheme().IsEmpty() {
		t.Fatal("undo did not revert the circuit pick")
	}
}

// Override rule through the UI: with a circuit theme loaded, toggling
// "Always use my theme" flips which accent is on screen.
func TestThemeOverrideToggleFlipsEffectiveThemeEndToEnd(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	g.setUserThemeCategory("accent", "orchid-300")
	mint, _ := swatchRGBA("mint-300")
	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != mint {
		t.Fatalf("override off: circuit must win, got %v", genColorPrimary)
	}
	cx, cy := center(ov.OverrideRect())
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if genColorPrimary != orchid {
		t.Fatalf("override on: user must win, got %v", genColorPrimary)
	}
}

// Esc closes the picker first, then the overlay (stacked portals).
func TestThemePickerEscapeOrder(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	p := g.drum.portal()
	expandThemeAdjust(t, g, ov, tap)
	scrollThemeRowIntoView(t, g, ov, "viz")
	cx, cy := center(ov.ChipRect("viz"))
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if !p.Has(themePickerID) {
		t.Fatal("picker did not open")
	}
	pressEscape := func() { // repo pattern (undo_keyboard_test.go): stub the key for one frame
		restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
		advanceFrames(g, 1)
		restore()
		advanceFrames(g, 1)
	}
	pressEscape()
	if p.Has(themePickerID) || !p.Has(themeOverlayID) {
		t.Fatalf("first Esc must close only the picker: picker=%v overlay=%v", p.Has(themePickerID), p.Has(themeOverlayID))
	}
	pressEscape()
	if p.Has(themeOverlayID) {
		t.Fatal("second Esc must close the overlay")
	}
}

// A tap outside the picker (on the theme panel itself) closes ONLY the picker
// — the way a mobile user cancels a pick, since the picker header carries no
// close button there. The overlay beneath stays open.
func TestThemePickerOutsideTapClosesPickerOnly(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	p := g.drum.portal()
	expandThemeAdjust(t, g, ov, tap)
	scrollThemeRowIntoView(t, g, ov, "focus")
	cx, cy := center(ov.ChipRect("focus"))
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if !p.Has(themePickerID) {
		t.Fatal("picker did not open")
	}
	// Any point on the theme panel that the picker does not cover is inside
	// the (now filtered-out) theme overlay: the press must dismiss the picker.
	pk := g.drum.colorWheelComp.InputBounds()
	tx, ty, found := 0, 0, false
	for y := ov.Rect().Min.Y + 4; y < ov.Rect().Max.Y-4 && !found; y += 8 {
		for x := ov.Rect().Min.X + 4; x < ov.Rect().Max.X-4; x += 8 {
			if !image.Pt(x, y).In(pk) {
				tx, ty, found = x, y, true
				break
			}
		}
	}
	if !found {
		t.Fatalf("picker %v covers the whole panel %v", pk, ov.Rect())
	}
	tap(tx, ty, 3)
	advanceFrames(g, 2)
	if p.Has(themePickerID) {
		t.Fatal("outside tap did not close the picker")
	}
	if !p.Has(themeOverlayID) {
		t.Fatal("outside tap must not close the theme overlay")
	}
}
