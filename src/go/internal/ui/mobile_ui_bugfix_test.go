//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestSidebarTextWidthUsesTextWidth verifies that sidebarScaledTextWidth uses
// proper font metrics (TextWidth) instead of the debug font's debugCharW*len.
func TestSidebarTextWidthUsesTextWidth(t *testing.T) {
	assertDefaultParityState(t)

	// Override textMeasureWidth so TextWidth returns 10px/rune (wider than debugCharW=6).
	oldMeasure := textMeasureWidth
	textMeasureWidth = func(s string) int { return 10 * len([]rune(s)) }
	t.Cleanup(func() { textMeasureWidth = oldMeasure })

	s := "Every N"
	scale := sidebarTextScale

	got := sidebarScaledTextWidth(s, scale)
	want := int(float64(TextWidth(s)) * scale) // 10*7*1.2 = 84

	if got != want {
		t.Fatalf("sidebarScaledTextWidth(%q, %.1f) = %d, want %d (using TextWidth)", s, scale, got, want)
	}
}

// TestSidebarResizeHandlePillOrientation verifies the sidebar resize pill
// is oriented vertically (taller than wide) for the vertical divider edge.
func TestSidebarResizeHandlePillOrientation(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	r := g.sidebar.resizeHandleRect()
	if r.Empty() {
		t.Fatal("resize handle rect is empty")
	}

	// Vertical divider → pill should be taller than wide.
	if r.Dy() <= r.Dx() {
		t.Fatalf("pill should be taller than wide for vertical divider: Dx=%d Dy=%d", r.Dx(), r.Dy())
	}
}

// TestMobileTrackButtonInline verifies the Track button is rendered as an
// inline chip on the timeline ruler header on mobile (Theme 2 — was
// previously hidden).
func TestMobileTrackButtonInline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	trackR := dv.TrackBtnForTest().Rect()

	if trackR.Empty() {
		t.Fatalf("Track chip should be visible on mobile (inline on timeline header)")
	}
}

// TestMobileTransportButtonUniformWidth verifies all mobile transport buttons
// have equal widths, +/- buttons are half height, and Track matches transport width.
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
	// Row 1 on mobile now hosts the segmented Pads/EQ/Wave control (col 1)
	// and the overflow button (col 2). The binary viewSwitchBtn is hidden on
	// mobile (replaced by the segmented control in B4). The old 2-button
	// uniform-width check becomes a 1-button no-op; skip it to avoid a false
	// reference-width of 0 when the legacy button rect is empty.
	//
	// Instead, verify the overflow button has a non-empty rect on mobile
	// (regression guard for the placement staying in the bottom bar).
	if dv.overflowBtn() != nil && dv.overflowBtn().Rect().Empty() {
		t.Errorf("overflowBtn rect empty on mobile — should be in bottom action bar")
	}

	// BPM ± buttons render at full row height (matching play) on the
	// mobile horizontal stepper. Regression guard for B1: prior layout
	// halved the row to 22 px via stackVerticalTransport.
	playH := dv.playBtn().Rect().Dy()
	bpmIncH := dv.bpmIncBtn().Rect().Dy()
	if diff := bpmIncH - playH; diff < -2 || diff > 2 {
		t.Errorf("bpmIncBtn height %d should match play height %d within 2 px on mobile horizontal stepper (B1)", bpmIncH, playH)
	}

	// Track button now lives inline as a chip on the timeline ruler
	// header on mobile (Theme 2). Verify it's non-empty and meets the
	// touch-target floor.
	trackR := dv.trackBtn().Rect()
	if trackR.Empty() {
		t.Errorf("track chip should be visible on mobile (inline on timeline)")
	} else if trackR.Dx() < TouchMinTarget() || trackR.Dy() < TouchMinTarget() {
		t.Errorf("track chip %v below touch target %d", trackR, TouchMinTarget())
	}
}
