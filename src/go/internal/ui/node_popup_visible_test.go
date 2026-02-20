package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The node popup panel should be fully visible within the grid view (top pane),
// clamped away from screen edges even for nodes near the borders.
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
	// Panel must be fully within screen bounds for the top pane: x in [0, winW], y in [0, split.Y]
	if panel.Min.X < 0 || panel.Max.X > g.winW || panel.Min.Y < 0 || panel.Max.Y > g.split.Y {
		t.Fatalf("panel out of bounds: %v winW=%d splitY=%d", panel, g.winW, g.split.Y)
	}
}
