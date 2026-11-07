//go:build test

package ui

import (
	"math"
	"testing"
)

// The ribbon is drag-only: a press+release seeks nothing, and a drag to another beat seeks on release, snapped
// to the beat under the pointer.
func TestRibbonDragSeeksToBeat(t *testing.T) {
	g := newGameWithStart(t)
	_ = g.Update() // lay out zones so timelineBarRect is populated
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	if bar.Empty() {
		t.Fatal("expected non-empty timelineBarRect after layout")
	}
	// Press near the left edge, drag to ~40% across the bar, release there.
	pressX := bar.Min.X + 2
	x := bar.Min.X + bar.Dx()*2/5
	y := bar.Min.Y + bar.Dy()/2
	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(pressX, y)
	adapter.OnDrag(x, y)
	adapter.OnRelease(x, y)
	g.drainSeekRequest()

	// Computed independently of ribbonStepAt, so the test does not echo the production formula.
	div := g.grid.MaxDiv()
	frac := float64(x-bar.Min.X) / float64(bar.Dx())
	totalBeats := z.callbacks.TimelineBeats()
	wantBeat := int(math.Round(frac * float64(totalBeats)))
	wantStep := wantBeat * div

	got := g.playheadAbsSubdiv()
	if got%div != 0 {
		t.Fatalf("ribbon seek must snap to a whole beat, got step=%d (div=%d)", got, div)
	}
	if got == 0 {
		t.Fatalf("ribbon drag to frac=%.2f must move the playhead off zero (want step=%d)", frac, wantStep)
	}
	if got != wantStep {
		t.Fatalf("ribbon drag landed on step=%d, want step=%d (beat=%d, div=%d)", got, wantStep, wantBeat, div)
	}
}
