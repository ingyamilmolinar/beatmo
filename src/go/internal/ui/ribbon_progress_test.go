//go:build test

package ui

import "testing"

// TestRibbonSingleClickDoesNotSeek asserts the ribbon is now drag-only: a
// pure tap (press+release at the same X, no drag in between) must NOT emit
// a seek. Task 6 removes the old click-to-seek-on-press behavior.
func TestRibbonSingleClickDoesNotSeek(t *testing.T) {
	g := newGameWithStart(t)
	_ = g.Update() // lay out zones so timelineBarRect is populated
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	if bar.Empty() {
		t.Fatal("expected non-empty timelineBarRect after layout")
	}
	adapter := &timelineScrubHitAdapter{zone: z}
	before := g.playheadAbsSubdiv()
	// a pure tap (press+release at same x, no drag) must NOT seek.
	x := bar.Min.X + bar.Dx()/2
	y := bar.Min.Y + bar.Dy()/2
	adapter.OnPress(x, y)
	adapter.OnRelease(x, y)
	g.drainSeekRequest()
	if g.playheadAbsSubdiv() != before {
		t.Fatalf("single click must not seek (before=%d after=%d)", before, g.playheadAbsSubdiv())
	}
}

// TestRibbonDragStillSeeks asserts a real drag (press then move to a
// different beat, then release) still commits a seek on release.
func TestRibbonDragStillSeeks(t *testing.T) {
	g := newGameWithStart(t)
	_ = g.Update() // lay out zones so timelineBarRect is populated
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	if bar.Empty() {
		t.Fatal("expected non-empty timelineBarRect after layout")
	}
	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(bar.Min.X+5, bar.Min.Y+bar.Dy()/2)
	adapter.OnDrag(bar.Min.X+bar.Dx()*3/5, bar.Min.Y+bar.Dy()/2) // move → different beat
	adapter.OnRelease(bar.Min.X+bar.Dx()*3/5, bar.Min.Y+bar.Dy()/2)
	g.drainSeekRequest()
	if g.playheadAbsSubdiv() == 0 {
		t.Fatal("a drag across the ribbon must still commit a seek")
	}
}
