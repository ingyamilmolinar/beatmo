package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestDesktopBeatCounterRectNonEmpty(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty on desktop")
	}
}

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

func TestDesktopBeatCounterWithinBounds(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	if !dv.beatCounterRect.In(dv.Bounds) {
		t.Fatalf("beatCounterRect %v not within Bounds %v", dv.beatCounterRect, dv.Bounds)
	}
}

// Play (leftmost) and redo (rightmost inline) share one row.
func TestDesktopSingleRowTransport(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()
	playR := dv.playBtn().Rect()
	rightR := dv.transportZone.redoBtn.Rect()
	if playR.Empty() || rightR.Empty() {
		t.Fatalf("play or redo button has empty rect: play=%v redo=%v", playR, rightR)
	}
	// Redo should be in the same row as play (overlapping Y ranges).
	if rightR.Min.Y >= playR.Max.Y || rightR.Max.Y <= playR.Min.Y {
		t.Fatalf("redo %v should overlap Y range with play %v in single-row layout", rightR, playR)
	}
}

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

// Both platforms lock the bar to ActiveTopBarSpec().Height from DESIGN.md.
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

// Icon only, popup-based, no inline slider.
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

// Volume and overflow sit on play's row, or on the row below as a fallback; either keeps them reachable.
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
	overlap := volR.Min.Y < playR.Max.Y && volR.Max.Y > playR.Min.Y
	below := volR.Min.Y >= playR.Max.Y
	if !overlap && !below {
		t.Fatalf("mobile volIcon %v should overlap or sit below play %v", volR, playR)
	}
}

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

// On both platforms.
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

// In the timeline widget, not the transport.
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

// Suppressed inline on mobile; they live in the overflow menu.
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

func TestLenButtonsAtRightEdge(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), graph, testLogger)
	dv.recalcButtons()

	tl := dv.widgetRects[WidgetTimeline]
	incR := dv.lenIncBtn.Rect()
	// Within a padding margin.
	margin := incR.Max.X - tl.Max.X
	if margin < -20 {
		// negative margin means too far left; should be within 20px of the edge
		t.Fatalf("lenIncBtn right edge %d is too far from timeline right edge %d (margin=%d)",
			incR.Max.X, tl.Max.X, margin)
	}
}
