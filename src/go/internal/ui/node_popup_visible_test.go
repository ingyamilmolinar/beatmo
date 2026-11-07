package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Clamped inside the grid pane even for a node near the border, so it never paints over the drum view.
func TestNodePopupClampedInsideTopPane(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)
	// A high I puts its screen X near winW.
	n := g.tryAddNode(g.grid.MaxDiv()-1, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()
	panel, ok := g.sidebar.rects[sbPanel]
	if !ok || panel.Empty() {
		t.Fatalf("panel rect missing")
	}
	// Panel must be fully within the grid pane: x in [0, gridW], y in [0, gridH].
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	if panel.Min.X < gridPane.Min.X || panel.Max.X > gridPane.Max.X ||
		panel.Min.Y < gridPane.Min.Y || panel.Max.Y > gridPane.Max.Y {
		t.Fatalf("panel out of grid-pane bounds: %v gridPane=%v", panel, gridPane)
	}
}
