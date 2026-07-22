package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure the sidebar panel uses the correct default width on desktop, and is
// content-sized (compact) rather than spanning the full grid pane height. The
// card is a node-anchored popup rendered as a GridTree layer ("grid-sidebar",
// clipped to the grid pane — game_node_sidebar.go's layout()): its height is
// content-sized, capped to gridH only as a ceiling (so a tall node's card
// still scrolls instead of running off-pane), and AnchorPopupRect clamps its
// final position to the GRID PANE (not the whole window) so the card can
// never paint over the bottom drum-view pane.
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
	// Compact: content-sized (header + a couple of sections for this minimal
	// node), capped to gridH only as a ceiling — never taller than the pane,
	// and on this small 640x360 window the content actually reaches that
	// ceiling (the old design's height was ALWAYS gridH by construction; the
	// new design's height is content-derived and merely capped by it).
	gridH := g.split.GridH(g.winH)
	if r.Dy() <= 0 {
		t.Fatalf("panel height must be positive, got %d", r.Dy())
	}
	if r.Dy() > gridH {
		t.Fatalf("panel height %d must not exceed the gridH ceiling %d", r.Dy(), gridH)
	}
	// Ensure the card lies within the GRID PANE (clamped away from the split
	// line, never spilling into the drum pane below).
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	if r.Min.X < gridPane.Min.X || r.Max.X > gridPane.Max.X ||
		r.Min.Y < gridPane.Min.Y || r.Max.Y > gridPane.Max.Y {
		t.Fatalf("panel out of grid-pane bounds: %v gridPane=%v", r, gridPane)
	}
}

// Ensure mobile sidebar uses the same default width, is compact (content-
// sized, not full-pane), and stays within the grid pane bounds.
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
	// Compact: content-sized, never taller than the gridH ceiling.
	gridH := g.split.GridH(g.winH)
	if r.Dy() <= 0 {
		t.Fatalf("panel height must be positive, got %d", r.Dy())
	}
	if r.Dy() > gridH {
		t.Fatalf("panel height %d must not exceed the gridH ceiling %d", r.Dy(), gridH)
	}
	// Panel must fit within the GRID PANE (the card is a GridTree layer clipped
	// to the grid pane and can never extend past the grid/drum split line).
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	if r.Min.X < gridPane.Min.X || r.Max.X > gridPane.Max.X {
		t.Fatalf("panel overflows horizontally: %v gridPane=%v", r, gridPane)
	}
	if r.Min.Y < gridPane.Min.Y || r.Max.Y > gridPane.Max.Y {
		t.Fatalf("panel overflows vertically: %v gridPane=%v", r, gridPane)
	}
}
