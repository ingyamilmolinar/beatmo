package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A FAITHFUL reproduction of "I open the colour picker and use the arrows and
// it pans".
//
// DELIBERATELY UNTAGGED, so it runs under BOTH the -tags test ebitestub build
// AND the real-Ebiten build (make test-real). Every other picker test carries
// //go:build test and therefore only ever exercised the stub — which is a real
// blind spot here, because the stub declares KeyUp/KeyArrowUp (and the other
// arrow pairs) as DISTINCT constants while real Ebiten ALIASES them.
//
// The earlier guard test pressed each arrow for exactly ONE frame. A real user
// HOLDS the key for many frames, and gridShortcut's arrow-pan is
// level-triggered (isKeyPressed, not isKeyJustPressed) — so it pans on EVERY
// frame the key is down. A single-frame press therefore cannot catch a gate
// that fails on any frame other than the first.
//
// It also opens the picker through the real row-colour-button callback rather
// than reaching for an internal opener.

// holdKey holds a key down across n consecutive Update frames, then releases,
// mirroring a real key hold.
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

// openColorPickerViaRowButton opens the picker the way the row colour button
// does, then settles a frame.
func openColorPickerViaRowButton(t *testing.T) *Game {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	// Go through the row rack's real OnColorWheelOpen callback — the same one
	// the colour button fires — rather than an internal opener.
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

// THE BUG: holding an arrow with the picker open must not pan the grid on ANY
// frame.
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

// The picker must also survive the hold — a guard that works by closing the
// picker would "pass" the test above while being useless.
func TestColorPickerE2E_StaysOpenAcrossAHeldArrow(t *testing.T) {
	g := openColorPickerViaRowButton(t)
	holdKey(g, ebiten.KeyArrowDown, 20)
	if !g.drum.colorWheelComp.IsOpen() {
		t.Error("the picker closed while an arrow was held; it must stay open and navigate")
	}
}

// And the counterpart: a held arrow with no picker open pans every frame, so
// the guard above is not just "pan is broken".
func TestColorPickerE2E_HeldArrowPansWhenNoPickerOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)

	before := g.cam.OffsetX
	holdKey(g, ebiten.KeyArrowLeft, 5)
	if g.cam.OffsetX == before {
		t.Error("a held arrow did not pan with nothing open — the test cannot detect a pan")
	}
}

// noKeyboardOverlay is a minimal overlay that does NOT claim the keyboard —
// a tooltip, a notification, any transient surface.
type noKeyboardOverlay struct{}

func (noKeyboardOverlay) Layout(anchor, screen image.Rectangle) {}
func (noKeyboardOverlay) HitAreas() []HitArea                   { return nil }
func (noKeyboardOverlay) Draw(dst *ebiten.Image)                {}
func (noKeyboardOverlay) ShouldClose() bool                     { return false }

// THE REAL BUG: the keyboard claim consulted only the portal's TOP overlay, so
// anything stacked above an open picker silently dropped the claim and the
// arrows went back to panning the grid.
//
// The picker is still open and still on screen — the user sees it, presses an
// arrow, and the camera moves. That is the reported symptom, and no test that
// opened the picker in isolation could ever catch it.
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
