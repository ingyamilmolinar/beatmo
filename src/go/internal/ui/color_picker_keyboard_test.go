//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// While open the picker claims the keyboard and handles the arrows itself, like every sibling overlay, so an arrow
// never also pans the camera beneath it.

// On a real Game, driven through Game.Update.
func openColorPickerInGame(t *testing.T) *Game {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)
	g.drum.openColorPickerForRow(0)
	if g.drum.colorWheelComp == nil || !g.drum.colorWheelComp.IsOpen() {
		t.Fatal("colour picker did not open")
	}
	g.drum.colorWheelComp.ClearHold()
	_ = g.Update()
	return g
}

func TestColorPicker_ArrowKeysDoNotPanTheGrid(t *testing.T) {
	g := openColorPickerInGame(t)

	beforeX, beforeY := g.cam.OffsetX, g.cam.OffsetY
	for _, k := range []ebiten.Key{
		ebiten.KeyArrowDown, ebiten.KeyArrowUp, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	} {
		nmKey(g, k)
	}
	if g.cam.OffsetX != beforeX || g.cam.OffsetY != beforeY {
		t.Errorf("arrows panned the camera while the colour picker was open: "+
			"(%v,%v) → (%v,%v); the picker must own the keyboard",
			beforeX, beforeY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// The counterpart, so the guard cannot be satisfied by breaking arrow-pan.
func TestColorPicker_ArrowKeysStillPanWhenClosed(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	beforeX := g.cam.OffsetX
	nmKey(g, ebiten.KeyArrowLeft)
	if g.cam.OffsetX == beforeX {
		t.Error("arrow-pan is dead with no picker open — the guard is too broad")
	}
}

func TestColorPicker_ClaimsKeyboardWhileOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)
	if g.KeyboardClaimed() {
		t.Fatal("keyboard is claimed before the picker opens")
	}

	g.drum.openColorPickerForRow(0)
	g.drum.colorWheelComp.ClearHold()
	_ = g.Update()
	if !g.KeyboardClaimed() {
		t.Error("an open colour picker does not claim the keyboard, so the grid still handles arrows")
	}

	g.drum.colorWheelComp.Close()
	_ = g.Update()
	if g.KeyboardClaimed() {
		t.Error("keyboard is still claimed after the picker closed")
	}
}

// ─── The picker must HANDLE the arrows, not merely block them ──────────────

func TestColorPicker_ArrowKeysMoveTheCursor(t *testing.T) {
	g := openColorPickerInGame(t)
	c := g.drum.colorWheelComp

	nmKey(g, ebiten.KeyArrowDown)
	first := c.KeyNav().Selected()
	if first == MenuKeyNavNoSelection {
		t.Fatal("ArrowDown did not place a cursor in the colour grid")
	}

	cols := colorWheelNavHost{c: c}.MenuGridCols()
	if cols <= 1 {
		t.Fatal("colour picker reports no grid columns; MenuKeyNav would treat it as a list")
	}

	nmKey(g, ebiten.KeyArrowDown)
	second := c.KeyNav().Selected()
	if second == first {
		t.Error("a second ArrowDown did not move the cursor")
	}
	if (second-first)%cols != 0 {
		t.Errorf("ArrowDown moved by %d, not a whole row of %d columns", second-first, cols)
	}

	before := c.KeyNav().Selected()
	nmKey(g, ebiten.KeyArrowRight)
	if after := c.KeyNav().Selected(); after == before {
		t.Error("ArrowRight did not move the cursor")
	}
}

// Enter applies and keeps the picker open; a second Enter on the same swatch confirms.
func TestColorPicker_EnterAppliesThenConfirms(t *testing.T) {
	g := openColorPickerInGame(t)
	c := g.drum.colorWheelComp

	nmKey(g, ebiten.KeyArrowDown)
	nmKey(g, ebiten.KeyArrowRight)
	sel := c.KeyNav().Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("no cursor after arrow keys")
	}
	sw := c.swatches()
	if sel < 0 || sel >= len(sw) {
		t.Fatalf("cursor %d out of palette range %d", sel, len(sw))
	}
	want := sw[sel].RGBA

	nmKey(g, ebiten.KeyEnter)
	if !colorsEqualRGB(g.drum.rowColorAt(0), want) {
		t.Errorf("Enter applied %v, want %v", g.drum.rowColorAt(0), want)
	}
	if !c.IsOpen() {
		t.Fatal("first Enter closed the picker; it must apply and stay open")
	}

	nmKey(g, ebiten.KeyEnter)
	if c.IsOpen() {
		t.Error("second Enter on the same swatch did not confirm and close")
	}
}

// So the first arrow moves relative to the current colour.
func TestColorPicker_CursorStartsOnAppliedSwatch(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	want := SuggestedInstrumentSwatches[7].RGBA
	g.drum.Rows[0].Color = want

	g.drum.openColorPickerForRow(0)
	g.drum.colorWheelComp.ClearHold()
	_ = g.Update()

	c := g.drum.colorWheelComp
	sel := c.KeyNav().Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("picker opened with no cursor; it must start on the applied swatch")
	}
	if got := c.swatches()[sel].RGBA; !colorsEqualRGB(got, want) {
		t.Errorf("cursor started on %v, want the applied %v", got, want)
	}
}
