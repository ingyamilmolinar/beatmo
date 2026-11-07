//go:build test

package ui

import (
	"testing"
)

// Wide enough for the live readout: a half-band clamp once starved it on narrow phones.
func TestBeatCounterFitsMobile(t *testing.T) {
	setupMobileTest(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 780)
	advanceFrames(g, 3)
	if !Profile().IsMobile() {
		t.Fatalf("expected mobile profile at 360x780")
	}

	// The mobile-compact readout fits, so it never clips.
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
