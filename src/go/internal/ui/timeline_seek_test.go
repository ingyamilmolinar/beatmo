//go:build test

package ui

import (
	"math"
	"testing"
)

// TestRibbonPressSeeksToBeat drives the ribbon scrub adapter's press+release
// directly (bypassing the input dispatcher) and asserts the resulting seek
// request lands the playhead on a whole-beat boundary. Task 10 redesigns the
// ribbon from a view-offset scrub surface into a click-to-seek surface:
// press commits an immediate seek to the nearest beat under the pointer;
// drag only previews a ghost position; release commits the final beat if it
// differs from the press position.
func TestRibbonPressSeeksToBeat(t *testing.T) {
	g := newGameWithStart(t)
	_ = g.Update() // lay out zones so timelineBarRect is populated
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	if bar.Empty() {
		t.Fatal("expected non-empty timelineBarRect after layout")
	}
	// Press at ~40% across the bar.
	x := bar.Min.X + bar.Dx()*2/5
	y := bar.Min.Y + bar.Dy()/2
	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(x, y)
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
		t.Fatalf("ribbon press at frac=%.2f must move the playhead off zero (want step=%d)", frac, wantStep)
	}
	if got != wantStep {
		t.Fatalf("ribbon press landed on step=%d, want step=%d (beat=%d, div=%d)", got, wantStep, wantBeat, div)
	}
}
