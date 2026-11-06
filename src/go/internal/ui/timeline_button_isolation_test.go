//go:build test

package ui

import (
	"image"
	"testing"
)

// TestTrackBtnDoesNotOverlapTimelineBar guards a UI bug where the track/free
// button was painted on top of the leftmost portion of the timeline progress
// bar, hiding the initial seconds of playback progress from the user.
//
// Invariant: on desktop the track button must sit entirely to the LEFT of
// where the timeline begins. The two rectangles must not share any X range.
//
// This is the spatial-isolation test the user explicitly requested: a guard
// against the broader class of bugs where two unrelated UI components claim
// the same screen real estate.
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

	// The keycap sits in the counter band ABOVE the ruler; it must never paint
	// over the bar (the ruler keeps its full width below it).
	if trackR.Overlaps(tlR) {
		t.Fatalf("track button %v overlaps timeline bar %v (button paints over the start of the timeline, hiding initial progress)",
			trackR, tlR)
	}
	if trackR.Max.Y > tlR.Min.Y {
		t.Fatalf("track button must sit above the timeline bar: trackBtn.Max.Y=%d, timelineRect.Min.Y=%d",
			trackR.Max.Y, tlR.Min.Y)
	}
}

// TestTimelineZoneButtonsDoNotOverlapBar generalises the rule to every button
// the TimelineZone draws inside its rect: trackBtn (left), and lenInc/lenDec
// (right). None of them may share X range with the timeline progress bar —
// otherwise users lose visibility of either the initial or trailing portion
// of the timeline progress.
//
// Adding a new button to TimelineZone? Either give it its own non-overlapping
// X band, or this test will catch the regression.
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
