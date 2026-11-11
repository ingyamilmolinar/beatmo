package ui

import (
	"image"
	"testing"
)

// TestFloatOffPaneEdges verifies the gutter nudge: a popup welded to a pane
// edge is pulled inward by the gutter when (and only when) that axis has slack,
// so the panel reads as a floating card instead of chrome bolted to the frame.
func TestFloatOffPaneEdges(t *testing.T) {
	pane := image.Rect(0, 0, 1000, 300)
	const gutter = 10

	// Welded to the right + bottom edges, with slack on both axes → both edges
	// pulled inward by exactly the gutter; size unchanged.
	r := image.Rect(880, 240, 1000, 300) // 120x60
	got := floatOffPaneEdges(r, pane, gutter)
	want := image.Rect(870, 230, 990, 290)
	if got != want {
		t.Fatalf("welded corner: got %v want %v", got, want)
	}

	// Fills the full pane height (no vertical slack) → vertical position is
	// left untouched even though it touches top+bottom; horizontal still gets
	// its right gutter.
	tall := image.Rect(880, 0, 1000, 300)
	gotTall := floatOffPaneEdges(tall, pane, gutter)
	if gotTall.Min.Y != 0 || gotTall.Max.Y != 300 {
		t.Fatalf("tall panel: vertical should be unchanged, got %v", gotTall)
	}
	if gotTall.Max.X != 990 {
		t.Fatalf("tall panel: right edge should gutter to 990, got %v", gotTall)
	}

	// Already inside the gutter on every edge → no-op.
	inside := image.Rect(100, 100, 200, 200)
	if got := floatOffPaneEdges(inside, pane, gutter); got != inside {
		t.Fatalf("inside panel: should be unchanged, got %v", got)
	}

	// Zero gutter → no-op.
	if got := floatOffPaneEdges(r, pane, 0); got != r {
		t.Fatalf("zero gutter: should be unchanged, got %v", got)
	}
}
