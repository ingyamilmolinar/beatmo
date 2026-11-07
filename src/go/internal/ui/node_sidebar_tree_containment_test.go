//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The sidebar is a grid-pane surface and never renders into the drum view. Near the pane's bottom a card
// anchored below its node has no room: clamped to the grid pane it flips above, clamped to the window it spills.
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
	g.cam.OffsetY = float64(gridH) - float64(gridTopOffset()) - 30
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()

	panel, ok := g.sidebar.rects[sbPanel]
	if !ok || panel.Empty() {
		t.Fatalf("sidebar panel rect missing")
	}
	if !panel.In(gridPane) {
		t.Fatalf("node sidebar panel %v escapes the grid pane %v — it renders over the "+
			"bottom drum-view panel (a separate component tree). The node pop-up must "+
			"stay within the grid pane.", panel, gridPane)
	}
}
