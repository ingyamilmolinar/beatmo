//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the input-isolation contract for the mobile synth-knob
// wheel popup: while it is open (a modal + scrim portal), gestures and pointer
// actions must act EXCLUSIVELY on the popup. They must never leak through the
// scrim to background surfaces handled OUTSIDE the input tree in Game.Update —
// the grid pane (node create/select, long-press delete), the camera (pinch
// zoom / two-finger pan) or the timeline. The only effect a gesture outside the
// popup may have is closing it (handled by the tree's click-outside + Esc).
//
// The tree itself already isolates correctly via the HitIndex modal filter; the
// leak lives in the Game-level out-of-tree handlers, which is what these
// exercise via direct calls (Game.Update only routes to them once a tap/gesture
// is confirmed inside the grid pane, so calling them directly is faithful).

// openMobileSynthWheel sets up a mobile synth tab with the wheel popup open and
// returns a grid-pane point that is outside the popup (a "tap on the scrim over
// the grid"). It fails the test if the popup did not open or the point is not a
// usable grid-pane location.
func openMobileSynthWheel(t *testing.T) (g *Game, gx, gy int) {
	t.Helper()
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	mobileSynthWheelSceneSetup()(g)

	if g.drum == nil || g.drum.synthWheelPopup == nil || !g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("setup did not open the synth wheel popup")
	}

	// A point near the top-left of the grid pane: above the splitter, inside the
	// grid pane, and clear of the (bottom-anchored) popup rect.
	gx, gy = 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Fatalf("test point (%d,%d) is not in the grid pane", gx, gy)
	}
	if pr := g.drum.synthWheelPopup.Rect(); pr.Min.Y <= gy {
		t.Fatalf("test point y=%d overlaps the popup rect %v — pick a point above it", gy, pr)
	}
	return g, gx, gy
}

// TestSynthWheelPopupBlocksGridTap: a tap in the grid pane while the wheel
// popup is open must NOT create or select a node underneath.
func TestSynthWheelPopupBlocksGridTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openMobileSynthWheel(t)

	nodesBefore := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) != nodesBefore {
		t.Fatalf("grid tap leaked through the open synth wheel popup: nodes %d -> %d", nodesBefore, len(g.nodes))
	}
}

// TestSynthWheelPopupBlocksTwoFingerPan: a two-finger pan while the popup is
// open must NOT pan the camera (the background scene must stay put).
func TestSynthWheelPopupBlocksTwoFingerPan(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openMobileSynthWheel(t)

	offX, offY := g.cam.OffsetX, g.cam.OffsetY
	g.handleTouchTwoFingerPan(40, 25, gx, gy)
	if g.cam.OffsetX != offX || g.cam.OffsetY != offY {
		t.Fatalf("two-finger pan leaked to camera while popup open: offset (%.1f,%.1f) -> (%.1f,%.1f)",
			offX, offY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// TestSynthWheelPopupBlocksPinchZoom: a pinch gesture while the popup is open
// must NOT zoom the camera.
func TestSynthWheelPopupBlocksPinchZoom(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openMobileSynthWheel(t)

	scaleBefore := g.cam.Scale
	// First event records the pinch baseline; the second applies a zoom ratio.
	g.handleTouchPinch(gx, gy, 1.0)
	g.handleTouchPinch(gx, gy, 2.0)
	if g.cam.Scale != scaleBefore {
		t.Fatalf("pinch leaked to camera zoom while popup open: scale %.3f -> %.3f", scaleBefore, g.cam.Scale)
	}
}

// TestSynthWheelPopupClickOutsideCloses verifies the only sanctioned way a tap
// outside the popup affects anything: it closes the popup (via the tree's
// click-outside path) and creates no node beneath it.
func TestSynthWheelPopupClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openMobileSynthWheel(t)

	nodesBefore := len(g.nodes)
	press := func(down bool) {
		restore := SetInputForTest(
			func() (int, int) { return gx, gy },
			func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return g.winW, g.winH },
		)
		g.drum.Update()
		restore()
	}
	press(true)
	press(false)

	if g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("click outside the popup should close it")
	}
	if g.drum.portal().Has("synth-wheel-popup") {
		t.Fatal("portal should drop the synth-wheel-popup entry after click-outside")
	}
	if len(g.nodes) != nodesBefore {
		t.Fatalf("click-outside-to-close leaked a node to the grid: nodes %d -> %d", nodesBefore, len(g.nodes))
	}
}

// TestSynthWheelPopupEscCloses verifies the second sanctioned exception: Esc
// closes the popup.
func TestSynthWheelPopupEscCloses(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, _, _ := openMobileSynthWheel(t)

	restore := SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	g.drum.Update()
	restore()

	if g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("Esc should close the synth wheel popup")
	}
}
