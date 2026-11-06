//go:build test

package ui

import (
	"image"
	"testing"
)

// These tests pin Task 8: SMOOTH continuous + momentum scroll for
// ControlGrid, gated behind SetSmoothScroll so shared callers (sampler knob
// grid, synth macro grid) keep the original stepped behavior byte-for-byte.

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
	// Negative steps scroll DOWN (First increases) — see
	// TestControlGrid_WheelStepDirection for the sign convention pinned on
	// the default stepped path; HandleWheel shares the same convention.
	if !g.WheelStep(-3) {
		t.Fatal("wheel should move the smooth grid")
	}
	if g.Scroll().VS.First <= first {
		t.Fatalf("wheel should advance the window: First=%d, want > %d", g.Scroll().VS.First, first)
	}
	// A continuous 3-notch spin should be able to move more than one row
	// (unlike the stepped path's hard one-row-per-notch cap).
	if got := g.Scroll().VS.First; got < first+2 {
		t.Errorf("smooth wheel should honor notch magnitude: First=%d, want >= %d", got, first+2)
	}
}

// TestControlGrid_DefaultStaysStepped guards the sampler / synth-macro path:
// a grid that never calls SetSmoothScroll must behave EXACTLY as before —
// stepped wheel (one row per notch, cooldown-gated) and a stepped content
// drag that never accrues momentum.
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
