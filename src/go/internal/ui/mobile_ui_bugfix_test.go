//go:build test

package ui

import (
	"testing"
)

// Real font metrics, not the debug font's per-glyph width.
func TestSidebarTextWidthUsesTextWidth(t *testing.T) {
	assertDefaultParityState(t)

	// Override textMeasureWidth so TextWidth returns 10px/rune (wider than debugCharW=6).
	oldMeasure := textMeasureWidth
	textMeasureWidth = func(s string) int { return 10 * len([]rune(s)) }
	t.Cleanup(func() { textMeasureWidth = oldMeasure })

	s := "Every N"
	scale := sidebarLabelScale()

	got := sidebarScaledTextWidth(s, scale)
	want := int(float64(TextWidth(s)) * scale) // 10*7*1.2 = 84

	if got != want {
		t.Fatalf("sidebarScaledTextWidth(%q, %.1f) = %d, want %d (using TextWidth)", s, scale, got, want)
	}
}

// An inline chip on the timeline ruler header.
func TestMobileTrackButtonInline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	trackR := dv.TrackBtnForTest().Rect()

	if trackR.Empty() {
		t.Fatalf("Track chip should be visible on mobile (inline on timeline header)")
	}
}

// Equal widths, and Track matches them.
func TestMobileTransportButtonUniformWidth(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)

	// Row 0 buttons should have equal width (±1px tolerance).
	row0Btns := []*Button{dv.playBtn(), dv.stopBtn(), dv.subdivBtn()}
	refW := dv.playBtn().Rect().Dx()
	for _, b := range row0Btns {
		w := b.Rect().Dx()
		diff := w - refW
		if diff < 0 {
			diff = -diff
		}
		if diff > 1 {
			t.Errorf("row0 button width %d differs from play width %d by %d (max 1px)", w, refW, diff)
		}
	}
	// The binary viewSwitchBtn is hidden on mobile (the segmented control replaces it), so there is no second
	// width to compare.

	// Full row height, matching play, on the horizontal stepper.
	playH := dv.playBtn().Rect().Dy()
	bpmIncH := dv.bpmIncBtn().Rect().Dy()
	if diff := bpmIncH - playH; diff < -2 || diff > 2 {
		t.Errorf("bpmIncBtn height %d should match play height %d within 2 px on mobile horizontal stepper (B1)", bpmIncH, playH)
	}

	// Its touch-expanded hit area meets the touch-target floor.
	trackR := dv.trackBtn().Rect()
	if trackR.Empty() {
		t.Errorf("track chip should be visible on mobile (inline on timeline)")
	} else if tr := trackTouchRect(t, dv); tr.Dx() < TouchMinTarget() || tr.Dy() < TouchMinTarget() {
		t.Errorf("track chip touch target %v below touch target %d", tr, TouchMinTarget())
	}
}
