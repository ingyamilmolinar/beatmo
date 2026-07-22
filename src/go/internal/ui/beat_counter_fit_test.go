//go:build test

package ui

import (
	"testing"
)

// TestBeatCounterFitsMobile asserts the beat-counter slot is wide enough to
// render the live "Beat 1 · 0:00" readout without clipping. Regression guard for
// the mobile "Beat 1 · 0:00 clipped to Beat 1" bug: the band split clamped the
// counter slot to HALF the band, starving the essential readout below its text
// width on narrow phones.
func TestBeatCounterFitsMobile(t *testing.T) {
	setupMobileTest(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 780)
	advanceFrames(g, 3)
	if !Profile().IsMobile() {
		t.Fatalf("expected mobile profile at 360x780")
	}

	// The actual drawn readout (mobile-compact "Beat N") must fit the slot so
	// the live counter never clips to garbage.
	readout := g.drum.timelineInfo(0)
	want := TextWidth(readout) + 2*beatCounterPillPadX
	got := g.drum.beatCounterRect.Dx()
	t.Logf("readout=%q counterSlot=%d want(textfit)=%d stableSlot=%d",
		readout, got, want, beatCounterSlotWidth())
	if got < want {
		t.Fatalf("beat counter slot too narrow: got %d, need %d to fit %q without clipping",
			got, want, readout)
	}
}
