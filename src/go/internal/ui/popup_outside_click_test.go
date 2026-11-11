//go:build test

package ui

import (
	"image"
	"testing"
)

// Part C — uniform outside-click consumption. A press outside ANY interactive
// overlay (blocking OR not) must close it without leaking to the surface below.
// Blocking overlays already did this; these pin the NON-blocking cases (docked
// FX panel, EQ channel dropdown) and the passive-tooltip exclusion.

// fxPanelGame builds a Game with the demo rows and opens row 0's (non-blocking)
// FX panel. It returns the game and a grid-pane point clear of the panel.
func fxPanelGame(t *testing.T) (g *Game, gx, gy int) {
	t.Helper()
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 700)
	g.Update() // build demo → rows exist

	if g.drum == nil || len(g.drum.Rows) == 0 {
		t.Fatal("no rows to open an FX panel for")
	}
	g.drum.OpenFXPanel(0)
	g.Update()
	if !g.drum.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	// Precondition: the FX panel is a NON-blocking overlay (no scrim/modal).
	if g.drum.PortalHasBlocking() {
		t.Fatal("precondition: FX panel must be non-blocking (no scrim/modal)")
	}
	gx, gy = 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Skipf("test point (%d,%d) not in the grid pane", gx, gy)
	}
	return g, gx, gy
}

// TestFXPanelOpenBlocksGridTap: while the non-blocking FX panel is open, a tap
// on the grid beneath must NOT create/select a node — the out-of-tree grid
// handler must stand down for any interactive overlay, not only blocking ones.
func TestFXPanelOpenBlocksGridTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := fxPanelGame(t)

	nodesBefore := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) != nodesBefore {
		t.Fatalf("grid tap leaked through the open FX panel: nodes %d -> %d "+
			"(a non-blocking overlay must still swallow an outside tap)", nodesBefore, len(g.nodes))
	}
}

// TestFXPanelOpenBlocksCameraPan: a two-finger pan while the FX panel is open
// must NOT move the camera.
func TestFXPanelOpenBlocksCameraPan(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := fxPanelGame(t)

	offX, offY := g.cam.OffsetX, g.cam.OffsetY
	g.handleTouchTwoFingerPan(40, 25, gx, gy)
	if g.cam.OffsetX != offX || g.cam.OffsetY != offY {
		t.Fatalf("two-finger pan leaked to the camera while the FX panel was open: "+
			"offset (%.1f,%.1f) -> (%.1f,%.1f)", offX, offY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// TestPassiveTooltipDoesNotBlockGridTap: a display-only tooltip is Passive, so
// it must NOT own input — a grid tap while it is up still creates a node, unlike
// the interactive overlays above.
func TestPassiveTooltipDoesNotBlockGridTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 700)
	g.Update() // build demo → rows exist
	if g.drum == nil || len(g.drum.Rows) == 0 {
		t.Fatal("no rows")
	}

	// Open a passive tooltip exactly as the production long-press paths do.
	g.drum.portal().Open(PortalEntry{
		ID:      "levels-agg-tt",
		Overlay: NewTooltipOverlay("hint"),
		Modal:   false,
		Passive: true,
		Anchor:  image.Rect(10, 10, 40, 30),
	})
	if !g.drum.portal().Has("levels-agg-tt") {
		t.Fatal("tooltip did not open")
	}
	if g.drum.PortalOwnsInput() {
		t.Fatal("a passive tooltip must not own input")
	}

	gx, gy := 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Skipf("test point (%d,%d) not in the grid pane", gx, gy)
	}
	nodesBefore := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) == nodesBefore {
		t.Fatal("a passive tooltip wrongly blocked a grid tap (no node created)")
	}
}
