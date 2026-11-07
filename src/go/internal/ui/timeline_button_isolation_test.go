//go:build test

package ui

import (
	"image"
	"testing"
)

// On desktop the track button sits wholly left of where the timeline begins, so it never hides the first
// seconds of progress.
func TestTrackBtnDoesNotOverlapTimelineBar(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, false)

	dv := newDrumViewForTrackTest(t, false)

	trackR := dv.TrackBtnForTest().Rect()
	tlR := dv.timelineRect
	if trackR.Empty() {
		t.Fatalf("track button rect is empty on desktop — fixture invalid")
	}
	if tlR.Empty() {
		t.Fatalf("timelineRect is empty — fixture invalid")
	}

	// In the counter band above the ruler, which keeps its full width.
	if trackR.Overlaps(tlR) {
		t.Fatalf("track button %v overlaps timeline bar %v (button paints over the start of the timeline, hiding initial progress)",
			trackR, tlR)
	}
	if trackR.Max.Y > tlR.Min.Y {
		t.Fatalf("track button must sit above the timeline bar: trackBtn.Max.Y=%d, timelineRect.Min.Y=%d",
			trackR.Max.Y, tlR.Min.Y)
	}
}

// No button in the zone shares X range with the progress bar; a new one needs its own band.
func TestTimelineZoneButtonsDoNotOverlapBar(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, false)

	dv := newDrumViewForTrackTest(t, false)
	tlR := dv.timelineRect
	if tlR.Empty() {
		t.Fatalf("timelineRect is empty — fixture invalid")
	}

	cases := []struct {
		name string
		rect image.Rectangle
	}{
		{"trackBtn", dv.TrackBtnForTest().Rect()},
		{"lenIncBtn", dv.lenIncBtn.Rect()},
		{"lenDecBtn", dv.lenDecBtn.Rect()},
	}
	for _, c := range cases {
		if c.rect.Empty() {
			t.Errorf("%s rect empty (desktop fixture should populate it)", c.name)
			continue
		}
		if c.rect.Overlaps(tlR) {
			t.Errorf("%s %v overlaps timeline bar %v", c.name, c.rect, tlR)
		}
	}
}
