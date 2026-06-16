package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestDesktopBeatCounterRectNonEmpty verifies that the beat counter rect is
// computed as a non-empty rectangle on desktop.
func TestDesktopBeatCounterRectNonEmpty(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty on desktop")
	}
}

// TestDesktopBeatCounterAboveTimeline verifies that the beat counter sits
// above the timeline bar (not behind it).
func TestDesktopBeatCounterAboveTimeline(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if dv.beatCounterRect.Max.Y > dv.timelineRect.Min.Y {
		t.Fatalf("beatCounterRect.Max.Y=%d exceeds timelineRect.Min.Y=%d — counter is behind the bar",
			dv.beatCounterRect.Max.Y, dv.timelineRect.Min.Y)
	}
}

// TestDesktopBeatCounterNotBehindToolbar verifies that the beat counter rect
// does not overlap the transport widget.
func TestDesktopBeatCounterNotBehindToolbar(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	transport := dv.widgetRects[WidgetTransport]
	if !transport.Empty() && dv.beatCounterRect.Overlaps(transport) {
		t.Fatalf("beatCounterRect %v overlaps transport %v", dv.beatCounterRect, transport)
	}
	// Also check it doesn't overlap any primary toolbar buttons.
	for _, btn := range []*Button{dv.playBtn(), dv.stopBtn(), dv.uploadBtn(), dv.importBtn(), dv.exportBtn()} {
		if btn == nil {
			continue
		}
		r := btn.Rect()
		if !r.Empty() && dv.beatCounterRect.Overlaps(r) {
			t.Fatalf("beatCounterRect %v overlaps button %v", dv.beatCounterRect, r)
		}
	}
}

// TestDesktopBeatCounterWithinBounds verifies that the beat counter rect is
// fully contained within the drum view bounds.
func TestDesktopBeatCounterWithinBounds(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if !dv.beatCounterRect.In(dv.Bounds) {
		t.Fatalf("beatCounterRect %v not within Bounds %v", dv.beatCounterRect, dv.Bounds)
	}
}

// TestDesktopSingleRowTransport verifies the desktop transport is a single-row
// layout — play (leftmost) and overflow (rightmost) are in the same row. Upload
// is no longer inline on desktop (it moved behind the overflow menu), so the
// overflow button is the rightmost inline control used to prove the single row.
func TestDesktopSingleRowTransport(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	playR := dv.playBtn().Rect()
	overflowR := dv.overflowBtn().Rect()
	if playR.Empty() || overflowR.Empty() {
		t.Fatalf("play or overflow button has empty rect: play=%v overflow=%v", playR, overflowR)
	}
	// Overflow should be in the same row as play (overlapping Y ranges).
	if overflowR.Min.Y >= playR.Max.Y || overflowR.Max.Y <= playR.Min.Y {
		t.Fatalf("overflow %v should overlap Y range with play %v in single-row layout", overflowR, playR)
	}
}

// TestMobileBeatCounterAboveTimeline verifies the beat counter is visible and
// above the timeline bar on mobile (not inside/behind it).
func TestMobileBeatCounterAboveTimeline(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), graph, testLogger)
	dv.recalcButtons()
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty on mobile")
	}
	if dv.beatCounterRect.Max.Y > dv.timelineRect.Min.Y {
		t.Fatalf("mobile beatCounterRect.Max.Y=%d exceeds timelineRect.Min.Y=%d — counter is behind the bar",
			dv.beatCounterRect.Max.Y, dv.timelineRect.Min.Y)
	}
	// Verify the counter has enough height for text.
	if dv.beatCounterRect.Dy() < debugCharH {
		t.Fatalf("mobile beatCounterRect height %d < debugCharH %d — text won't fit",
			dv.beatCounterRect.Dy(), debugCharH)
	}
}

// TestTopBarHeightMatchesActiveSpec verifies both platforms lock the top
// control bar to ActiveTopBarSpec().Height (sourced from DESIGN.md
// `profileOverrides.headerMinH/headerMaxH`). Desktop is a compact single-row
// bar; mobile keeps a two-row tools layout but at a tighter height than the
// pre-compaction 72 px.
func TestTopBarHeightMatchesActiveSpec(t *testing.T) {
	assertDefaultParityState(t)
	// Desktop
	graph1 := model.NewGraph(testLogger)
	dvD := NewDrumView(image.Rect(0, 0, 1280, 720), graph1, testLogger)
	dvD.recalcButtons()
	desktopSpec := DesktopTopBarSpec()
	if dvD.headerH != desktopSpec.Height {
		t.Fatalf("desktop headerH=%d should equal DesktopTopBarSpec().Height=%d", dvD.headerH, desktopSpec.Height)
	}

	// Mobile
	withSmallScreen(t, true)
	graph2 := model.NewGraph(testLogger)
	dvM := NewDrumView(image.Rect(0, 0, 390, 844), graph2, testLogger)
	dvM.recalcButtons()
	mobileSpec := MobileTopBarSpec()
	if dvM.headerH != mobileSpec.Height {
		t.Fatalf("mobile headerH=%d should equal MobileTopBarSpec().Height=%d", dvM.headerH, mobileSpec.Height)
	}
}

// TestDesktopVolumeIconPopup verifies the desktop master volume uses an
// icon-only control (popup-based, no inline slider).
func TestDesktopVolumeIconPopup(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if dv.mainVolSlider() == nil {
		t.Fatal("mainVolSlider is nil")
	}
	sliderR := dv.mainVolSlider().Rect()
	if !sliderR.Empty() {
		t.Fatal("desktop mainVolSlider rect should be empty — popup mode")
	}
	if dv.mainVolIconRect.Empty() {
		t.Fatal("desktop mainVolIconRect is empty")
	}
}

// TestMobileVolumeIconOpensPopup verifies the mobile master volume icon is
// present and opens a popup (no inline slider).
func TestMobileVolumeIconOpensPopup(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), graph, testLogger)
	dv.recalcButtons()

	// Icon should be visible.
	if dv.mainVolIconRect.Empty() {
		t.Fatal("mobile mainVolIconRect is empty — should be visible")
	}
	// Inline slider should be hidden.
	if dv.mainVolSlider() != nil && !dv.mainVolSlider().Rect().Empty() {
		t.Fatalf("mobile mainVolSlider rect should be empty, got %v", dv.mainVolSlider().Rect())
	}
	// Open popup via the icon.
	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("master volume popup did not open on mobile")
	}
	if dv.masterVolPopup.Rect().Empty() {
		t.Fatal("master volume popup rect is empty")
	}
	dv.closeMasterVolumePopup()
	if dv.masterVolPopup.IsOpen() {
		t.Fatal("master volume popup did not close")
	}
}

// TestDesktopTwoRowNoOverlap verifies that row 0 and row 1 controls do not
// overlap in the desktop two-row layout.
func TestDesktopTwoRowNoOverlap(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	// Row 0: play, stop, bpm, subdiv, len buttons
	row0 := []*Button{dv.playBtn(), dv.stopBtn(), dv.subdivBtn()}
	// Row 1: upload, import, export
	row1 := []*Button{dv.uploadBtn(), dv.importBtn(), dv.exportBtn()}

	for _, b0 := range row0 {
		for _, b1 := range row1 {
			r0, r1 := b0.Rect(), b1.Rect()
			if r0.Empty() || r1.Empty() {
				continue
			}
			if r0.Overlaps(r1) {
				t.Fatalf("row 0 button %v overlaps row 1 button %v", r0, r1)
			}
		}
	}
}

// TestMobileTransportLayout (Theme 4) verifies mobile transport places
// vol icon and overflow either on the same row as play (single-row mode,
// preferred when topBounds height fits one row) or on row 1 below play
// (two-row fallback). Both layouts must keep them reachable.
func TestMobileTransportLayout(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), graph, testLogger)
	dv.recalcButtons()

	playR := dv.playBtn().Rect()
	if playR.Empty() {
		t.Fatal("mobile play button has empty rect")
	}
	volR := dv.mainVolIconRect
	if volR.Empty() {
		t.Fatal("mobile mainVolIconRect is empty")
	}
	// Either single-row (volR overlaps playR's row) or two-row (volR is
	// strictly below playR). Both are accepted.
	overlap := volR.Min.Y < playR.Max.Y && volR.Max.Y > playR.Min.Y
	below := volR.Min.Y >= playR.Max.Y
	if !overlap && !below {
		t.Fatalf("mobile volIcon %v should overlap or sit below play %v", volR, playR)
	}
	// Overflow must also be reachable.
	if dv.overflowBtn() != nil && dv.overflowBtn().Rect().Empty() {
		t.Fatal("mobile overflow button rect empty")
	}
}

// TestMobileTwoRowNoOverlap verifies that row 0 and row 1 controls do not
// overlap in the mobile two-row layout.
func TestMobileTwoRowNoOverlap(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), graph, testLogger)
	dv.recalcButtons()

	// Row 0: play, stop, bpm-related, subdiv, len
	row0 := []*Button{dv.playBtn(), dv.stopBtn(), dv.subdivBtn()}
	// Row 1 buttons
	var row1 []*Button
	if dv.viewSwitchBtn() != nil {
		row1 = append(row1, dv.viewSwitchBtn())
	}
	if dv.overflowBtn() != nil {
		row1 = append(row1, dv.overflowBtn())
	}

	for _, b0 := range row0 {
		for _, b1 := range row1 {
			r0, r1 := b0.Rect(), b1.Rect()
			if r0.Empty() || r1.Empty() {
				continue
			}
			if r0.Overlaps(r1) {
				t.Fatalf("mobile row 0 button %v overlaps row 1 button %v", r0, r1)
			}
		}
	}
	// Also check vol icon doesn't overlap row 0.
	volR := dv.mainVolIconRect
	if !volR.Empty() {
		for _, b0 := range row0 {
			r0 := b0.Rect()
			if !r0.Empty() && r0.Overlaps(volR) {
				t.Fatalf("mobile row 0 button %v overlaps volIcon %v", r0, volR)
			}
		}
	}
}

// TestTrackButtonInTimelineArea verifies the Track button is positioned in the
// timeline widget area on both platforms.
func TestTrackButtonInTimelineArea(t *testing.T) {
	assertDefaultParityState(t)

	// Desktop
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	trackR := dv.trackBtn().Rect()
	if trackR.Empty() {
		t.Fatal("desktop trackBtn rect is empty")
	}
	tl := dv.widgetRects[WidgetTimeline]
	if !trackR.In(tl) {
		t.Fatalf("desktop track %v not inside timeline widget %v", trackR, tl)
	}
	// Should not overlap transport.
	tr := dv.widgetRects[WidgetTransport]
	if !tr.Empty() && trackR.Overlaps(tr) {
		t.Fatalf("desktop track %v overlaps transport widget %v", trackR, tr)
	}
}

// TestLenButtonsInTimelineArea verifies that on desktop, len +/- buttons are
// within the timeline widget bounds, NOT within the transport widget bounds.
func TestLenButtonsInTimelineArea(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	tl := dv.widgetRects[WidgetTimeline]
	tr := dv.widgetRects[WidgetTransport]
	incR := dv.lenIncBtn.Rect()
	decR := dv.lenDecBtn.Rect()
	if incR.Empty() || decR.Empty() {
		t.Fatal("len buttons have empty rects")
	}
	if !incR.In(tl) {
		t.Fatalf("lenIncBtn %v not inside timeline widget %v", incR, tl)
	}
	if !decR.In(tl) {
		t.Fatalf("lenDecBtn %v not inside timeline widget %v", decR, tl)
	}
	if !tr.Empty() && incR.Overlaps(tr) {
		t.Fatalf("lenIncBtn %v overlaps transport widget %v", incR, tr)
	}
	if !tr.Empty() && decR.Overlaps(tr) {
		t.Fatalf("lenDecBtn %v overlaps transport widget %v", decR, tr)
	}
}

// TestLenButtonsHiddenOnMobile verifies the inline timeline length +/−
// pair is suppressed on mobile (A7 in the screenshot critique). The
// controls live behind the overflow menu instead — see
// TestLenButtons_ReachableViaOverflowOnMobile.
func TestLenButtonsHiddenOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 390, 844), graph, testLogger)
	dv.recalcButtons()

	if r := dv.lenIncBtn.Rect(); !r.Empty() {
		t.Fatalf("expected lenIncBtn empty on mobile (A7); got %v", r)
	}
	if r := dv.lenDecBtn.Rect(); !r.Empty() {
		t.Fatalf("expected lenDecBtn empty on mobile (A7); got %v", r)
	}
}

// TestLenButtonsNoOverlapBeatCounter verifies len buttons don't overlap the
// beat counter rect.
func TestLenButtonsNoOverlapBeatCounter(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	incR := dv.lenIncBtn.Rect()
	decR := dv.lenDecBtn.Rect()
	bc := dv.beatCounterRect
	if incR.Overlaps(bc) {
		t.Fatalf("lenIncBtn %v overlaps beat counter %v", incR, bc)
	}
	if decR.Overlaps(bc) {
		t.Fatalf("lenDecBtn %v overlaps beat counter %v", decR, bc)
	}
}

// TestLenButtonsNoOverlapTimeline verifies len buttons don't overlap the
// timeline bar rect.
func TestLenButtonsNoOverlapTimeline(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	incR := dv.lenIncBtn.Rect()
	decR := dv.lenDecBtn.Rect()
	tl := dv.timelineRect
	if incR.Overlaps(tl) {
		t.Fatalf("lenIncBtn %v overlaps timeline bar %v", incR, tl)
	}
	if decR.Overlaps(tl) {
		t.Fatalf("lenDecBtn %v overlaps timeline bar %v", decR, tl)
	}
}

// TestLenButtonsAtRightEdge verifies the buttons are positioned at the right
// edge of the timeline widget.
func TestLenButtonsAtRightEdge(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	tl := dv.widgetRects[WidgetTimeline]
	incR := dv.lenIncBtn.Rect()
	// The buttons should be near the right edge of the timeline widget.
	// Allow some margin for padding.
	margin := incR.Max.X - tl.Max.X
	if margin < -20 {
		// negative margin means too far left; should be within 20px of the edge
		t.Fatalf("lenIncBtn right edge %d is too far from timeline right edge %d (margin=%d)",
			incR.Max.X, tl.Max.X, margin)
	}
}
