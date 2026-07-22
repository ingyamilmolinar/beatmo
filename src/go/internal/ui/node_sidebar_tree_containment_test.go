//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestNodeSidebarStaysWithinGridPane pins the two-separate-trees invariant: the
// node sidebar is a GRID-pane surface — it belongs to the grid pane's component
// tree — and must NEVER render into the bottom drum-view panel (a separate
// tree). This is the regression guard for the portal-migration bug where the
// node-anchored card, composited through the GLOBAL OverlayPortal and clamped to
// the whole WINDOW, dropped past the split line and painted over the transport
// bar + rows (reported via screenshot).
//
// A node is panned near the BOTTOM of the grid pane, so anchoring a card below
// it leaves no room before the split: a window-clamped/portal card spills into
// the drum pane, while a grid-pane-clamped/grid-tree card flips above and stays
// contained. The panel rect must lie fully inside the grid pane.
func TestNodeSidebarStaysWithinGridPane(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	gridW := g.split.GridW(g.winW)
	gridH := g.split.GridH(g.winH)
	gridPane := image.Rect(0, 0, gridW, gridH)

	n := g.tryAddNode(0, 0, 0)
	// Pan so the node sits near the bottom edge of the grid pane, leaving no room
	// below the split — a card anchored below it would otherwise spill into the
	// drum-view panel.
	g.cam.OffsetY = float64(gridH) - float64(gridTopOffset()) - 30
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()

	panel, ok := g.sidebar.rects["panel"]
	if !ok || panel.Empty() {
		t.Fatalf("sidebar panel rect missing")
	}
	if !panel.In(gridPane) {
		t.Fatalf("node sidebar panel %v escapes the grid pane %v — it renders over the "+
			"bottom drum-view panel (a separate component tree). The node pop-up must "+
			"stay within the grid pane.", panel, gridPane)
	}
}
