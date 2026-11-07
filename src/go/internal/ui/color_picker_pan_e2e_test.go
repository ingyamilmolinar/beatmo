package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Untagged, so it runs on the real build too: the stub declares the arrow spellings distinct while real Ebiten
// aliases them. A user HOLDS an arrow, and the grid's pan is level-triggered, so a one-frame press cannot catch a
// gate that fails on a later frame.

func holdKey(g *Game, k ebiten.Key, frames int) {
	restore := SetInputForTest(
		func() (int, int) { return g.winW / 2, g.winH / 4 },
		func(ebiten.MouseButton) bool { return false },
		func(kk ebiten.Key) bool { return kk == k },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	for i := 0; i < frames; i++ {
		_ = g.Update()
	}
	restore()
	_ = g.Update()
}

// The way the row colour button does.
func openColorPickerViaRowButton(t *testing.T) *Game {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	// The callback the colour button fires, not an internal opener.
	z := g.drum.rowRackZone
	if z == nil || z.callbacks.OnColorWheelOpen == nil {
		t.Fatal("row colour-wheel callback is not wired")
	}
	z.callbacks.OnColorWheelOpen(0)
	_ = g.Update()

	if g.drum.colorWheelComp == nil || !g.drum.colorWheelComp.IsOpen() {
		t.Fatal("colour picker did not open from the row colour button")
	}
	return g
}

// No frame of a held arrow pans the grid while the picker is open.
func TestColorPickerE2E_HeldArrowNeverPansTheGrid(t *testing.T) {
	g := openColorPickerViaRowButton(t)

	for _, k := range []ebiten.Key{
		ebiten.KeyArrowDown, ebiten.KeyArrowUp, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	} {
		beforeX, beforeY := g.cam.OffsetX, g.cam.OffsetY
		holdKey(g, k, 20)
		if g.cam.OffsetX != beforeX || g.cam.OffsetY != beforeY {
			t.Fatalf("holding %v panned the camera (%v,%v) → (%v,%v) with the picker open "+
				"(open=%v, claimed=%v)",
				k, beforeX, beforeY, g.cam.OffsetX, g.cam.OffsetY,
				g.drum.colorWheelComp.IsOpen(), g.KeyboardClaimed())
		}
	}
}

// A guard that worked by closing the picker would pass the test above while being useless.
func TestColorPickerE2E_StaysOpenAcrossAHeldArrow(t *testing.T) {
	g := openColorPickerViaRowButton(t)
	holdKey(g, ebiten.KeyArrowDown, 20)
	if !g.drum.colorWheelComp.IsOpen() {
		t.Error("the picker closed while an arrow was held; it must stay open and navigate")
	}
}

// The control: with no picker, a held arrow pans every frame.
func TestColorPickerE2E_HeldArrowPansWhenNoPickerOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	before := g.cam.OffsetX
	holdKey(g, ebiten.KeyArrowLeft, 5)
	if g.cam.OffsetX == before {
		t.Error("a held arrow did not pan with nothing open — the test cannot detect a pan")
	}
}

// A tooltip, a notification: any transient surface.
type noKeyboardOverlay struct{}

func (noKeyboardOverlay) Layout(anchor, screen image.Rectangle) {}
func (noKeyboardOverlay) HitAreas() []HitArea                   { return nil }
func (noKeyboardOverlay) Draw(dst *ebiten.Image)                {}
func (noKeyboardOverlay) ShouldClose() bool                     { return false }

// The claim considers every open overlay, not only the top one: anything stacked above an open picker must not drop
// its claim and let the arrows pan.
func TestColorPickerE2E_KeepsKeyboardWhenAnotherOverlayIsOnTop(t *testing.T) {
	g := openColorPickerViaRowButton(t)
	if !g.KeyboardClaimed() {
		t.Fatal("picker does not claim the keyboard even on its own")
	}

	p := g.drum.portal()
	if p == nil {
		t.Fatal("no overlay portal")
	}
	p.Open(PortalEntry{ID: "test-tooltip", Overlay: noKeyboardOverlay{}})
	_ = g.Update()

	if !g.drum.colorWheelComp.IsOpen() {
		t.Fatal("picker closed when another overlay opened; cannot test the claim")
	}
	if !g.KeyboardClaimed() {
		t.Fatal("the keyboard claim was LOST because a non-claiming overlay is on top; " +
			"the picker is still open and must keep owning the arrows")
	}

	beforeX, beforeY := g.cam.OffsetX, g.cam.OffsetY
	holdKey(g, ebiten.KeyArrowLeft, 20)
	if g.cam.OffsetX != beforeX || g.cam.OffsetY != beforeY {
		t.Errorf("arrows panned the grid (%v,%v) → (%v,%v) with the picker open under "+
			"another overlay", beforeX, beforeY, g.cam.OffsetX, g.cam.OffsetY)
	}
}
