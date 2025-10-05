package ui

import (
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"testing"
)

// Ensure the node popup is reasonably compact while showing all controls.
func TestNodePopupCompactSize(t *testing.T) {
	g := New(game_log.New(nil, game_log.LevelError))
	g.Layout(640, 360)
	// Build a regular node and open menu
	n := g.tryAddNode(2, 0, 0)
	g.nodeMenuOpen = true
	g.nodeMenuNode = g.nodeByID(n.ID)
	g.updateNodeMenuRects()
	r, ok := g.nodeMenuRects["panel"]
	if !ok || r.Empty() {
		t.Fatalf("missing panel rect")
	}
	// Compact expectations: width <= 240, height <= 160 for defaults
	if r.Dx() > 240 {
		t.Fatalf("panel too wide: %d", r.Dx())
	}
	if r.Dy() > 180 { // allow some flexibility for font/spacing
		t.Fatalf("panel too tall: %d", r.Dy())
	}
	// Also ensure rect lies within the top pane
	if r.Min.Y < 0 || r.Max.Y > g.split.Y || r.Min.X < 0 || r.Max.X > g.winW {
		t.Fatalf("panel out of bounds: %v (win=%dx%d splitY=%d)", r, g.winW, g.winH, g.split.Y)
	}
}
