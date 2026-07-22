package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The node popup panel is a compact, node-anchored card rendered as a
// GridTree layer ("grid-sidebar", clipped to the grid pane — see
// game_node_sidebar.go's layout()), so it must stay fully visible within the
// grid view (top pane), clamped away from the split line even for nodes near
// the borders — it can never paint over the bottom drum-view pane.
func TestNodePopupClampedInsideTopPane(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)
	// Create a node near the right edge of the grid area
	// Position in grid units: use a high I so its screen X is near winW
	n := g.tryAddNode(g.grid.MaxDiv()-1, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()
	panel, ok := g.sidebar.rects["panel"]
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
