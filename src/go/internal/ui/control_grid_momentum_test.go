//go:build test

package ui

import (
	"image"
	"testing"
)

// SetSmoothScroll adds continuous + momentum scrolling; grids that never call it keep the stepped behaviour.

func TestControlGrid_SmoothContentDragHasMomentum(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetSmoothScroll(true)
	g.Layout(image.Rect(0, 0, 200, 100), 40, 60, 26, 6, 6) // 40 cells, small viewport
	if !g.HasScroll() {
		t.Fatal("precondition: grid must overflow to scroll")
	}
	// Fast upward content drag then release should keep coasting.
	if !g.BeginContentDrag(90) {
		t.Fatal("BeginContentDrag should capture on an overflowing smooth grid")
	}
	for y := 90; y >= 10; y -= 20 {
		g.DragTo(y)
	}
	g.EndDrag()
	if !g.HasMomentum() {
		t.Fatal("a flick should leave momentum")
	}
	ticks := 0
	for g.UpdateMomentum() && ticks < 600 {
		ticks++
	}
	if ticks == 0 {
		t.Fatal("momentum should coast for several frames")
	}
}

func TestControlGrid_SmoothWheelIsContinuous(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetSmoothScroll(true)
	g.Layout(image.Rect(0, 0, 200, 100), 40, 60, 26, 6, 6)
	if !g.HasScroll() {
		t.Fatal("precondition: grid must overflow to scroll")
	}
	first := g.Scroll().VS.First
	// Negative scrolls down (First grows), as on the stepped path.
	if !g.WheelStep(-3) {
		t.Fatal("wheel should move the smooth grid")
	}
	if g.Scroll().VS.First <= first {
		t.Fatalf("wheel should advance the window: First=%d, want > %d", g.Scroll().VS.First, first)
	}
	// More than one row, unlike the stepped path's cap.
	if got := g.Scroll().VS.First; got < first+2 {
		t.Errorf("smooth wheel should honor notch magnitude: First=%d, want >= %d", got, first+2)
	}
}

// Without SetSmoothScroll: one row per notch, cooldown-gated, and a content drag with no momentum.
func TestControlGrid_DefaultStaysStepped(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.Layout(image.Rect(0, 0, 200, 100), 40, 60, 26, 6, 6)
	if !g.HasScroll() {
		t.Fatal("precondition: grid must overflow to scroll")
	}

	// Stepped wheel: one row per notch, regardless of magnitude.
	if !g.WheelStep(-9) {
		t.Fatal("wheel should move")
	}
	if got := g.Scroll().VS.First; got != 1 {
		t.Fatalf("default grid wheel moved %d rows, want exactly 1 (stepped)", got)
	}

	// Stepped content drag: no momentum accrues, ever.
	if !g.BeginContentDrag(90) {
		t.Fatal("BeginContentDrag did not start on an overflowing default grid")
	}
	for y := 90; y >= 10; y -= 20 {
		g.DragTo(y)
	}
	g.EndDrag()
	if g.HasMomentum() {
		t.Fatal("default (non-smooth) grid must never accrue momentum")
	}
	if g.UpdateMomentum() {
		t.Fatal("UpdateMomentum on a default grid must be a no-op")
	}
}
