//go:build test

package ui

import "testing"

// The ribbon is drag-only: a tap without a drag never seeks.
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
