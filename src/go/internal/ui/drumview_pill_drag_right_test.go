//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Mirrors handleDrag (ResizeAxis + refreshWidgetLayout) per step until the clamp stops it.
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

// Rack and timeline stay edge-adjacent (rack.Max.X == timeline.Min.X): any gap or overlap distorts the rows.
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

// A control outside the rack means the column shrank below its content while controls kept the pre-drag layout.
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

// newTestDrumView only runs the constructor, so the relayout is explicit.
func addRowsForLayoutTest(t *testing.T, dv *DrumView, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
}

// The real proportions, where visibleRows() can be below len(Rows); offset to (0, screenH/2) because rowsTop
// derives from Bounds.Min.Y.
func newTestDrumViewBottomHalf(t *testing.T, screenW, screenH int) *DrumView {
	t.Helper()
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	return NewDrumView(image.Rect(0, screenH/2, screenW, screenH), nil, logger)
}

// With 6 rows a full-right drag must keep every row coherent, with no half-rendered rows and no "+" mid-stack.
func TestPillDragRightWithSixRowsKeepsLayoutCoherent(t *testing.T) {
	assertDefaultParityState(t)
	// Tall enough for 6 rows of 36 px plus the "+" footer and EQ chrome.
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

	// Every visible row has controls, each inside its y-band and the rack.
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

// With visibleRows() < len(Rows) the add button lands mid-stack; no row chrome may be drawn past it (stale
// cache or leftover widgets).
func TestPillDragRightWithVisLessThanRows(t *testing.T) {
	assertDefaultParityState(t)
	// ~180 px tall: about 3 visible rows of 6.
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
	// rackVisibleRows, what the rack lays out; visibleRows() can over-estimate.
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

	// No leftover rects from an earlier, larger layout.
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

// Every row contributes a draw inside its band: none silently missing.
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
		// The inner band, so neighbouring mask/stripe edges are not counted.
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

// No row stripe may land inside the rack: cells drawn behind the labels.
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
		// Stripe tokens alias the background by design, so only single-row-height fills can be real bleed.
		if c.Rect.Dy() > rh {
			continue
		}
		if c.Rect.Overlaps(rack) {
			t.Errorf("row stripe drawRect overlaps rack column after pill drag: rect=%v color=%v rack=%v timeline=%v",
				c.Rect, c.Color, rack, timeline)
		}
	}
}
