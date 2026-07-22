//go:build test

package ui

import (
	"math"
	"testing"
)

// TestRibbonDragSeeksToBeat drives the ribbon scrub adapter's
// press+drag+release directly (bypassing the input dispatcher) and asserts
// the resulting seek request lands the playhead on a whole-beat boundary.
// Task 6 makes the ribbon drag-only: a pure press+release at the same point
// no longer seeks at all (see TestRibbonSingleClickDoesNotSeek in
// ribbon_progress_test.go) — only a press-then-drag-to-a-different-beat
// commits a seek, on release, snapped to the nearest beat under the
// pointer. This test was TestRibbonPressSeeksToBeat under the old
// click-to-seek-on-press contract (Task 10); it now exercises the drag path
// instead of the removed press-seeks path.
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

	// Expected beat = round(frac * timelineBeats), independently computed
	// here (not via z.ribbonStepAt) so the test doesn't just echo the
	// production formula back at itself.
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
