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

	// X-ranges must be disjoint.
	if xRangesOverlap(trackR, tlR) {
		t.Fatalf("track button X-range %d..%d overlaps timeline bar X-range %d..%d (button paints over the start of the timeline, hiding initial progress)",
			trackR.Min.X, trackR.Max.X, tlR.Min.X, tlR.Max.X)
	}

	// Track button must be on the LEFT side of the timeline.
	if trackR.Max.X > tlR.Min.X {
		t.Fatalf("track button must sit entirely left of the timeline: trackBtn.Max.X=%d, timelineRect.Min.X=%d",
			trackR.Max.X, tlR.Min.X)
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
		if xRangesOverlap(c.rect, tlR) {
			t.Errorf("%s X-range %d..%d overlaps timeline bar X-range %d..%d",
				c.name, c.rect.Min.X, c.rect.Max.X, tlR.Min.X, tlR.Max.X)
		}
	}
}

// xRangesOverlap reports whether two rectangles share any X coordinate
// (regardless of Y). Empty rects never overlap.
func xRangesOverlap(a, b image.Rectangle) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X
}
