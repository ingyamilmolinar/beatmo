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

// TestMobileTrackButtonHidden verifies the Track button is hidden on mobile.
func TestMobileTrackButtonHidden(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	trackR := dv.TrackBtnForTest().Rect()

	if !trackR.Empty() {
		t.Fatalf("Track button should be hidden on mobile, got %v", trackR)
	}
}

// TestMobileTransportButtonUniformWidth verifies all mobile transport buttons
// have equal widths, +/- buttons are half height, and Track matches transport width.
func TestMobileTransportButtonUniformWidth(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)

	// Row 0 buttons should have equal width (±1px tolerance).
	row0Btns := []*Button{dv.playBtn, dv.stopBtn, dv.subdivBtn}
	refW := dv.playBtn.Rect().Dx()
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
	// Row 1 buttons should have equal width within their row (±1px tolerance).
	var row1Btns []*Button
	if dv.viewSwitchBtn != nil {
		row1Btns = append(row1Btns, dv.viewSwitchBtn)
	}
	if dv.overflowBtn != nil {
		row1Btns = append(row1Btns, dv.overflowBtn)
	}
	if len(row1Btns) > 1 {
		ref1W := row1Btns[0].Rect().Dx()
		for _, b := range row1Btns[1:] {
			w := b.Rect().Dx()
			diff := w - ref1W
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				t.Errorf("row1 button width %d differs from ref %d by %d (max 1px)", w, ref1W, diff)
			}
		}
	}

	// BPM +/- height should be approximately half of play button height (±2px).
	playH := dv.playBtn.Rect().Dy()
	bpmIncH := dv.bpmIncBtn.Rect().Dy()
	halfH := playH / 2
	diff := bpmIncH - halfH
	if diff < 0 {
		diff = -diff
	}
	if diff > 2 {
		t.Errorf("bpmIncBtn height %d should be ~half of play height %d (half=%d, diff=%d)", bpmIncH, playH, halfH, diff)
	}

	// Track button is hidden on mobile (desktop-only).
	trackR := dv.trackBtn.Rect()
	if !trackR.Empty() {
		t.Errorf("track button should be hidden on mobile, got %v", trackR)
	}
}
