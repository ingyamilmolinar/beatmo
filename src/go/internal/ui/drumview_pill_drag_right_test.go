//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// dragColPillRight simulates the user grabbing the column-divider pill
// (between rack and timeline) and dragging it as far right as possible.
// It mirrors what handleDrag does: ResizeAxis + refreshWidgetLayout, on
// each step. Returns when no further movement is possible (clamp by
// minColW or by the right edge).
func dragColPillRight(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv.widgets == nil || len(dv.widgets.cols) < 2 {
		t.Fatalf("widgets not configured: %v", dv.widgets)
	}
	const stride = 32
	const maxIters = 200
	for i := 0; i < maxIters; i++ {
		before := dv.widgets.ColWidth(0)
		dv.widgets.ResizeAxis("col", 0, stride)
		dv.refreshWidgetLayout()
		dv.recalcButtons()
		dv.calcLayout()
		after := dv.widgets.ColWidth(0)
		if after == before {
			return
		}
	}
}

// TestPillDragRightKeepsRackAndTimelineDisjoint asserts that no matter
// how far right the user drags the column-divider pill, the rack and
// timeline widget rectangles remain edge-adjacent (rack.Max.X ==
// timeline.Min.X) with no overlap and no gap. A regression on either
// side manifests as instrument-row UI distortion.
func TestPillDragRightKeepsRackAndTimelineDisjoint(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	dragColPillRight(t, dv)

	rack := dv.widgetRects[WidgetRack]
	timeline := dv.widgetRects[WidgetTimeline]
	if rack.Empty() || timeline.Empty() {
		t.Fatalf("widget rects empty after drag: rack=%v timeline=%v", rack, timeline)
	}
	if rack.Overlaps(timeline) {
		t.Errorf("rack overlaps timeline after pill drag right: rack=%v timeline=%v",
			rack, timeline)
	}
	if rack.Max.X != timeline.Min.X {
		t.Errorf("rack and timeline must be edge-adjacent: rack.Max.X=%d timeline.Min.X=%d",
			rack.Max.X, timeline.Min.X)
	}
	if rack.Max.X > timeline.Max.X {
		t.Errorf("rack extends past timeline.Max.X: rack=%v timeline=%v", rack, timeline)
	}
	if timeline.Dx() <= 0 {
		t.Errorf("timeline column collapsed: timeline=%v", timeline)
	}
}

// TestPillDragRightKeepsRowControlsInsideRack asserts every per-row
// control button (mute/solo/fx/origin/edit/save/menu/delete/label/volume)
// stays fully contained within the rack widget rect after a far-right
// pill drag. Buttons drifting outside the rack indicates the rack column
// shrank below required content width while controls were positioned
// using the pre-drag layout.
func TestPillDragRightKeepsRowControlsInsideRack(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	dragColPillRight(t, dv)

	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty after drag")
	}
	groups := dv.rowGroups()
	if len(groups) == 0 {
		t.Fatalf("no row groups configured")
	}
	for i, g := range groups {
		btns := []struct {
			name string
			b    *Button
		}{
			{"label", g.Label},
			{"mute", g.Mute},
			{"solo", g.Solo},
			{"fx", g.FX},
			{"origin", g.Origin},
			{"edit", g.Edit},
			{"save", g.Save},
			{"menu", g.Menu},
			{"delete", g.Delete},
		}
		for _, tc := range btns {
			if tc.b == nil {
				continue
			}
			r := tc.b.Rect()
			if r.Empty() {
				continue
			}
			if !r.In(rack) {
				t.Errorf("row %d %s button %v not inside rack %v", i, tc.name, r, rack)
			}
		}
	}
	for i, s := range dv.rowVolSliders() {
		if s == nil {
			continue
		}
		r := s.Rect()
		if r.Empty() {
			continue
		}
		if !r.In(rack) {
			t.Errorf("row %d volume slider %v not inside rack %v", i, r, rack)
		}
	}
}

// addRowsForLayoutTest appends n extra rows to dv (so total = 1 + n) and
// drives the layout pipeline so per-row widgets are positioned. Mirrors
// what real-time AddRow() does plus the explicit relayout the test harness
// needs (newTestDrumView only exercises the constructor).
func addRowsForLayoutTest(t *testing.T, dv *DrumView, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
}

// newTestDrumViewBottomHalf builds a DrumView whose bounds match the
// runtime "bottom half of screen" geometry (graph view above, drum view
// below). This mirrors the real-app proportions where the drum-rows area
// is small enough that visibleRows() can be smaller than len(Rows) — the
// scenario that triggers the screenshot bug. The drumview must be created
// at offset (0, screenH/2) — like Game does in real life — because
// internal logic uses Bounds.Min.Y as rowsTop after subtracting headerH.
func newTestDrumViewBottomHalf(t *testing.T, screenW, screenH int) *DrumView {
	t.Helper()
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	return NewDrumView(image.Rect(0, screenH/2, screenW, screenH), nil, logger)
}

// TestPillDragRightWithSixRowsKeepsLayoutCoherent reproduces the
// screenshot bug: with 6 instrument rows, dragging the column-divider pill
// fully right leaves intermediate rows half-rendered and pushes the "+" add
// button into the middle of the row stack. This test asserts the
// post-drag layout is coherent for every row simultaneously.
func TestPillDragRightWithSixRowsKeepsLayoutCoherent(t *testing.T) {
	assertDefaultParityState(t)
	// Use a viewport that comfortably fits all 6 rows after the desktop
	// row-height bump (28→36): 1280x900 gives ~450 px of bottom-half drum
	// view, leaving the rack rect tall enough for 6 rows of 36 px plus the
	// "+" footer plus EQ chrome.
	dv := newTestDrumViewBottomHalf(t, 1280, 900)
	addRowsForLayoutTest(t, dv, 5) // 1 default + 5 = 6 rows

	dv.refreshWidgetLayout()
	dragColPillRight(t, dv)
	// Final relayout to lock state (mirrors real handleDrag tail).
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty after drag")
	}
	if got, want := len(dv.Rows), 6; got != want {
		t.Fatalf("row count: got %d want %d", got, want)
	}

	rh := dv.rowHeight()
	rowsTop := rack.Min.Y
	vis := dv.visibleRows()
	t.Logf("post-drag: rack=%v rh=%d rowsTop=%d visibleRows=%d rowOffset=%d labelW=%d controlsW=%d",
		rack, rh, rowsTop, vis, dv.rowOffset, dv.labelW, dv.controlsW)

	// All 6 rows should fit in the typical 1280x720 fixture.
	if vis < 6 {
		t.Fatalf("visibleRows=%d expected >=6 for 1280x720 with 6 rows; rack=%v rh=%d", vis, rack, rh)
	}

	groups := dv.rowGroups()
	if len(groups) != 6 {
		t.Fatalf("rowGroups: got %d want 6", len(groups))
	}
	sliders := dv.rowVolSliders()
	if len(sliders) != 6 {
		t.Fatalf("rowVolSliders: got %d want 6", len(sliders))
	}

	// Per-row coherence: every visible row must have non-empty controls
	// AND every control must lie inside the row's expected y-band AND the
	// rack widget rect.
	for i := 0; i < 6; i++ {
		expectedY := rowsTop + i*rh
		expectedRowRect := image.Rect(rack.Min.X, expectedY, rack.Max.X, expectedY+rh)

		check := func(name string, r image.Rectangle) {
			t.Helper()
			if r.Empty() {
				t.Errorf("row %d %s rect empty after drag (rack=%v expectedRow=%v)",
					i, name, rack, expectedRowRect)
				return
			}
			if !r.In(rack) {
				t.Errorf("row %d %s rect %v not inside rack %v", i, name, r, rack)
			}
			if r.Min.Y < expectedRowRect.Min.Y || r.Max.Y > expectedRowRect.Max.Y {
				t.Errorf("row %d %s rect %v outside row y-band %v",
					i, name, r, expectedRowRect)
			}
		}

		check("label", groups[i].Label.Rect())
		check("mute", groups[i].Mute.Rect())
		check("solo", groups[i].Solo.Rect())
		check("fx", groups[i].FX.Rect())
		check("menu", groups[i].Menu.Rect())
		check("vol-slider", sliders[i].Rect())
	}

	// addRowBtn must sit BELOW the last visible row (no row overlap).
	addBtn := dv.addRowBtn().Rect()
	lastRowBottom := rowsTop + 6*rh
	if addBtn.Empty() {
		t.Fatalf("addRowBtn rect empty after drag")
	}
	if addBtn.Min.Y < lastRowBottom {
		t.Errorf("addRowBtn %v starts above row 5 bottom %d (rows must not overlap +)",
			addBtn, lastRowBottom)
	}
	for i := 0; i < 6; i++ {
		expectedY := rowsTop + i*rh
		expectedRowRect := image.Rect(rack.Min.X, expectedY, rack.Max.X, expectedY+rh)
		if addBtn.Overlaps(expectedRowRect) {
			t.Errorf("addRowBtn %v overlaps row %d band %v", addBtn, i, expectedRowRect)
		}
	}
}

// TestPillDragRightWithVisLessThanRows is the smoking-gun reproducer for
// the screenshot bug. With drum-view bounds small enough that
// visibleRows() < len(Rows), the addRowBtn lands at row index `nBelow`
// (mid-stack). The bug manifests if rendering ALSO emits row chrome for
// indices past `nBelow` — i.e. stale cache or misplaced widgets that
// survive the vis shrink.
func TestPillDragRightWithVisLessThanRows(t *testing.T) {
	assertDefaultParityState(t)
	// Small drum-view: bounds ~180px tall → rowsAreaHeight ~120 → vis ~3
	// with rh=28. With 6 rows total, only the first 3 should be drawn
	// from row chrome; addRowBtn lands at row 3 position.
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 540, 1280, 720), nil, logger)
	addRowsForLayoutTest(t, dv, 5) // 6 rows

	dv.refreshWidgetLayout()
	dragColPillRight(t, dv)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	rh := dv.rowHeight()
	rowsTop := rack.Min.Y
	// Use rackVisibleRows() — the value the row rack zone actually uses
	// for layout. dv.visibleRows() is bounds-derived and may
	// over-estimate when the rack rect is shorter than rowsAreaHeight.
	vis := dv.rackVisibleRows()
	t.Logf("small-bounds: rack=%v rh=%d rowsTop=%d vis=%d rowOff=%d", rack, rh, rowsTop, vis, dv.rowOffset)

	if vis >= 6 {
		t.Skipf("test fixture failed to constrain vis below row count: vis=%d (need <6); rack=%v rh=%d", vis, rack, rh)
	}

	// 1. addRowBtn must NOT overlap any visible-row band.
	addBtn := dv.addRowBtn().Rect()
	if addBtn.Empty() {
		t.Fatalf("addRowBtn rect empty")
	}
	for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
		expectedY := rowsTop + (i-dv.rowOffset)*rh
		expectedRowRect := image.Rect(rack.Min.X, expectedY, rack.Max.X, expectedY+rh)
		if addBtn.Overlaps(expectedRowRect) {
			t.Errorf("addRowBtn %v overlaps visible row %d band %v", addBtn, i, expectedRowRect)
		}
	}

	// 2. ALL rows past `vis` must have empty per-row widget rects (no leftover
	//    state from previous larger-vis layouts).
	groups := dv.rowGroups()
	sliders := dv.rowVolSliders()
	for i := dv.rowOffset + vis; i < len(dv.Rows); i++ {
		check := func(name string, r image.Rectangle) {
			t.Helper()
			if !r.Empty() {
				t.Errorf("row %d (out of vis range, vis=%d) has non-empty %s rect %v — stale layout state",
					i, vis, name, r)
			}
		}
		if i < len(groups) {
			check("label", groups[i].Label.Rect())
			check("mute", groups[i].Mute.Rect())
			check("solo", groups[i].Solo.Rect())
			check("fx", groups[i].FX.Rect())
			check("menu", groups[i].Menu.Rect())
		}
		if i < len(sliders) {
			check("vol-slider", sliders[i].Rect())
		}
	}

	// 3. addRowBtn must lie at or below the last visible row's bottom edge.
	addBtnBottom := dv.addRowBtn().Rect()
	lastVisibleRowBottom := rowsTop + vis*rh
	if !addBtnBottom.Empty() && addBtnBottom.Min.Y < lastVisibleRowBottom {
		t.Errorf("addRowBtn %v starts above last visible row bottom %d",
			addBtnBottom, lastVisibleRowBottom)
	}
}

// TestPillDragRightWithSixRowsRendersAllControls runs a full Draw after the
// pill drag with 6 rows and asserts every row contributes at least one
// drawCall inside its expected y-band — i.e. no row is silently missing
// from the rendered output.
func TestPillDragRightWithSixRowsRendersAllControls(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumViewBottomHalf(t, 1280, 720)
	addRowsForLayoutTest(t, dv, 5)

	dv.refreshWidgetLayout()
	dragColPillRight(t, dv)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	rh := dv.rowHeight()
	rowsTop := rack.Min.Y

	rec := &drawCallRecorder{}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	for i := 0; i < 6; i++ {
		// Use the inner half of the row band so we don't double-count the
		// neighbouring panel mask / stripe edges.
		bandTop := rowsTop + i*rh + 2
		bandBottom := rowsTop + (i+1)*rh - 2
		band := image.Rect(rack.Min.X+2, bandTop, rack.Max.X-2, bandBottom)
		hits := 0
		for _, c := range rec.calls {
			if c.Rect.Overlaps(band) && c.Rect.In(rack) {
				hits++
			}
		}
		if hits == 0 {
			t.Errorf("row %d band %v: no drawCalls — row appears unrendered", i, band)
		}
	}
}

// TestPillDragRightKeepsTimelineDrawConfined runs a full Draw after the
// pill drag and asserts no row-stripe drawRect call lands inside the
// rack widget rect. This is the user-visible bleed symptom: cells
// drawn behind the instrument labels.
func TestPillDragRightKeepsTimelineDrawConfined(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	dragColPillRight(t, dv)

	rack := dv.widgetRects[WidgetRack]
	timeline := dv.widgetRects[WidgetTimeline]
	if rack.Empty() || timeline.Empty() {
		t.Fatalf("widget rects empty after drag: rack=%v timeline=%v", rack, timeline)
	}

	stripeEven := genColorDrumStripeEven
	stripeOdd := genColorDrumStripeOdd
	rh := dv.rowHeight()

	rec := &drawCallRecorder{}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	for _, c := range rec.calls {
		if c.Kind != drawCallRect {
			continue
		}
		if c.Color != stripeEven && c.Color != stripeOdd {
			continue
		}
		// The drum-stripe tokens alias the background/surface-1 tokens by
		// design (DESIGN.md: stripes recede into the background), so legitimate
		// full-zone background fills share the stripe RGBA. A real per-row
		// stripe is exactly one rowHeight tall; zone/area backgrounds are
		// taller. Only single-row-height fills can be true bleed.
		if c.Rect.Dy() > rh {
			continue
		}
		if c.Rect.Overlaps(rack) {
			t.Errorf("row stripe drawRect overlaps rack column after pill drag: rect=%v color=%v rack=%v timeline=%v",
				c.Rect, c.Color, rack, timeline)
		}
	}
}
