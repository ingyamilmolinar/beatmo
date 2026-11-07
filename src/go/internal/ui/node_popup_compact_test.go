package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Content-sized, with the grid pane height only as a ceiling (a tall card scrolls), and clamped to the GRID pane
// so it never paints over the drum view.
func TestNodePopupCompactSize(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)
	// Build a regular node and open sidebar
	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.layout()
	r, ok := g.sidebar.rects[sbPanel]
	if !ok || r.Empty() {
		t.Fatalf("missing panel rect")
	}
	// Desktop: width == sidebarDefaultW
	if r.Dx() != sidebarDefaultW {
		t.Fatalf("expected panel width %d, got %d", sidebarDefaultW, r.Dx())
	}
	// On this small 640x360 window the content does reach the ceiling.
	gridH := g.split.GridH(g.winH)
	if r.Dy() <= 0 {
		t.Fatalf("panel height must be positive, got %d", r.Dy())
	}
	if r.Dy() > gridH {
		t.Fatalf("panel height %d must not exceed the gridH ceiling %d", r.Dy(), gridH)
	}
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	if r.Min.X < gridPane.Min.X || r.Max.X > gridPane.Max.X ||
		r.Min.Y < gridPane.Min.Y || r.Max.Y > gridPane.Max.Y {
		t.Fatalf("panel out of grid-pane bounds: %v gridPane=%v", r, gridPane)
	}
}

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
	r, ok := g.sidebar.rects[sbPanel]
	if !ok || r.Empty() {
		t.Fatalf("missing panel rect")
	}
	// Mobile: sidebar uses same sidebarDefaultW
	if r.Dx() != sidebarDefaultW {
		t.Fatalf("expected mobile panel width %d, got %d", sidebarDefaultW, r.Dx())
	}
	// Compact: content-sized, never taller than the gridH ceiling.
	gridH := g.split.GridH(g.winH)
	if r.Dy() <= 0 {
		t.Fatalf("panel height must be positive, got %d", r.Dy())
	}
	if r.Dy() > gridH {
		t.Fatalf("panel height %d must not exceed the gridH ceiling %d", r.Dy(), gridH)
	}
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	if r.Min.X < gridPane.Min.X || r.Max.X > gridPane.Max.X {
		t.Fatalf("panel overflows horizontally: %v gridPane=%v", r, gridPane)
	}
	if r.Min.Y < gridPane.Min.Y || r.Max.Y > gridPane.Max.Y {
		t.Fatalf("panel overflows vertically: %v gridPane=%v", r, gridPane)
	}
}
