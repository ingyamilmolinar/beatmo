//go:build test

package ui

import (
	"testing"
)

// TestBeatCounterFitsMobile asserts the beat-counter slot is wide enough to
// render the live "Beat 1 · 0:00" readout without the dedicated notification
// area (drawn to its right) painting over the overflow. Regression guard for
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
	band := g.drum.notifRect.Max.X - g.drum.beatCounterRect.Min.X
	t.Logf("readout=%q counterSlot=%d want(textfit)=%d stableSlot=%d band=%d notif=%v",
		readout, got, want, beatCounterSlotWidth(), band, g.drum.notifRect)
	if got < want {
		t.Fatalf("beat counter slot too narrow: got %d, need %d to fit %q without clipping",
			got, want, readout)
	}
	// The notification area must remain present (it shares the band).
	if g.drum.notifRect.Empty() {
		t.Fatalf("notif area collapsed to empty; band split must keep both counter and notif")
	}
}
