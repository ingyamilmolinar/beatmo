package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure the sidebar panel uses the correct default width on desktop.
func TestNodePopupCompactSize(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)
	// Build a regular node and open sidebar
	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()
	r, ok := g.sidebar.rects["panel"]
	if !ok || r.Empty() {
		t.Fatalf("missing panel rect")
	}
	// Desktop: width == sidebarDefaultW
	if r.Dx() != sidebarDefaultW {
		t.Fatalf("expected panel width %d, got %d", sidebarDefaultW, r.Dx())
	}
	// Sidebar spans full grid pane height
	gridH := g.split.GridH(g.winH)
	if r.Dy() != gridH {
		t.Fatalf("expected panel height %d (gridH), got %d", gridH, r.Dy())
	}
	// Ensure rect lies within the top pane
	if r.Min.Y < 0 || r.Max.Y > gridH || r.Min.X < 0 || r.Max.X > g.winW {
		t.Fatalf("panel out of bounds: %v (win=%dx%d gridH=%d)", r, g.winW, g.winH, gridH)
	}
}

// Ensure mobile sidebar uses the same default width and stays within bounds.
func TestNodePopupCompactSizeMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()
	r, ok := g.sidebar.rects["panel"]
	if !ok || r.Empty() {
		t.Fatalf("missing panel rect")
	}
	// Mobile: sidebar uses same sidebarDefaultW
	if r.Dx() != sidebarDefaultW {
		t.Fatalf("expected mobile panel width %d, got %d", sidebarDefaultW, r.Dx())
	}
	// Panel must fit within window
	if r.Min.X < 0 || r.Max.X > g.winW {
		t.Fatalf("panel overflows horizontally: %v (winW=%d)", r, g.winW)
	}
	if r.Min.Y < 0 || r.Max.Y > g.split.GridH(g.winH) {
		t.Fatalf("panel overflows vertically: %v (gridH=%d)", r, g.split.GridH(g.winH))
	}
}
