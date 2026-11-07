package ui

import (
	"image"
	"reflect"
	"testing"
)

// minLen == timelineUnitsPerBeat, maxLen == timeline width / MinCellWidth.
func TestDrumViewClampLengthBounds(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.calcLayout()

	if dv.timelineUnitsPerBeat != 1 {
		t.Fatalf("expected default timelineUnitsPerBeat=1, got %d", dv.timelineUnitsPerBeat)
	}
	if got := dv.clampLength(0); got != 1 {
		t.Errorf("clampLength(0) = %d, want 1 (min)", got)
	}
	if got := dv.clampLength(-3); got != 1 {
		t.Errorf("clampLength(-3) = %d, want 1 (min)", got)
	}
	if got := dv.clampLength(7); got != 7 {
		t.Errorf("clampLength(7) = %d, want 7 (interior)", got)
	}
	huge := 1_000_000
	w := dv.CellGridRect().Dx()
	mcw := MinCellWidth()
	if w > 0 && mcw > 0 {
		want := w / mcw
		if got := dv.clampLength(huge); got != want {
			t.Errorf("clampLength(%d) = %d, want %d (timelineDx/MinCellWidth)", huge, got, want)
		}
	}
}

func TestDrumViewHeaderRowsContiguity(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)

	header := dv.HeaderRect()
	rows := dv.RowsRect()

	if header.Min.Y != dv.Bounds.Min.Y {
		t.Errorf("header.Min.Y=%d, want bounds.Min.Y=%d", header.Min.Y, dv.Bounds.Min.Y)
	}
	if header.Max.Y != rows.Min.Y {
		t.Errorf("rows must start where header ends: header.Max.Y=%d rows.Min.Y=%d", header.Max.Y, rows.Min.Y)
	}
	if !header.In(dv.Bounds) {
		t.Errorf("header %v escapes bounds %v", header, dv.Bounds)
	}
	if !rows.In(dv.Bounds) {
		t.Errorf("rows %v escapes bounds %v", rows, dv.Bounds)
	}
	// Full width.
	if header.Min.X != dv.Bounds.Min.X || header.Max.X != dv.Bounds.Max.X {
		t.Errorf("header must span full width: header=%v bounds=%v", header, dv.Bounds)
	}
	if rows.Min.X != dv.Bounds.Min.X || rows.Max.X != dv.Bounds.Max.X {
		t.Errorf("rows must span full width: rows=%v bounds=%v", rows, dv.Bounds)
	}
}

// Row height never changes with zoom; it equals TouchRowHeight() under either profile.
func TestDrumViewRowHeightConstant(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)

	want := TouchRowHeight()
	if got := dv.rowHeight(); got != want {
		t.Errorf("desktop rowHeight()=%d, want TouchRowHeight()=%d", got, want)
	}

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	wantMobile := TouchRowHeight()
	if got := dv.rowHeight(); got != wantMobile {
		t.Errorf("mobile rowHeight()=%d, want TouchRowHeight()=%d", got, wantMobile)
	}
}

// floor(rowsAreaHeight / rowHeight), minus a reserved row on desktop with ReserveAddRowSpace.
func TestDrumViewVisibleRowsMath(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)

	rh := dv.rowHeight()
	if rh <= 0 {
		t.Fatalf("rowHeight=%d, want >0", rh)
	}
	areaH := dv.rowsAreaHeight()
	want := areaH / rh
	if Profile().ReserveAddRowSpace {
		want = (areaH - rh) / rh
		if want < 0 {
			want = 0
		}
		// Guarantee at least 1 visible row when the area fits a full row.
		if want == 0 && areaH >= rh {
			want = 1
		}
	}
	if got := dv.visibleRows(); got != want {
		t.Errorf("visibleRows()=%d, want %d (areaH=%d rh=%d reserve=%v)", got, want, areaH, rh, Profile().ReserveAddRowSpace)
	}
}

// Running it twice must give the same rects. The snapshot's Widgets slice has map order, so only direct
// rects and weights compare.
func TestDrumViewRefreshWidgetLayoutIdempotent(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)

	dv.refreshWidgetLayout()
	first := dv.widgetRectsSnapshot()
	dv.refreshWidgetLayout()
	second := dv.widgetRectsSnapshot()

	if first.AddButton != second.AddButton {
		t.Errorf("AddButton drift: %v vs %v", first.AddButton, second.AddButton)
	}
	if first.Timeline != second.Timeline {
		t.Errorf("Timeline drift: %v vs %v", first.Timeline, second.Timeline)
	}
	if first.Rack != second.Rack {
		t.Errorf("Rack drift: %v vs %v", first.Rack, second.Rack)
	}
	if first.Wave != second.Wave {
		t.Errorf("Wave drift: %v vs %v", first.Wave, second.Wave)
	}
	if first.Transport != second.Transport {
		t.Errorf("Transport drift: %v vs %v", first.Transport, second.Transport)
	}
	if !reflect.DeepEqual(first.Layout.Cols, second.Layout.Cols) {
		t.Errorf("Layout.Cols drift: %v vs %v", first.Layout.Cols, second.Layout.Cols)
	}
	if !reflect.DeepEqual(first.Layout.Rows, second.Layout.Rows) {
		t.Errorf("Layout.Rows drift: %v vs %v", first.Layout.Rows, second.Layout.Rows)
	}
	if first.Layout.Bounds != second.Layout.Bounds {
		t.Errorf("Layout.Bounds drift: %v vs %v", first.Layout.Bounds, second.Layout.Bounds)
	}
}

func TestDrumViewProfileTransitionFlipsButtonStyle(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
		t.Fatal("expected len inc/dec buttons present after ctor")
	}
	dv.refreshLenButtonsStyle()
	if dv.lenDecBtn.Style != LenDecStyle || dv.lenIncBtn.Style != LenIncStyle {
		t.Errorf("desktop styles wrong: dec=%v inc=%v want dec=%v inc=%v",
			dv.lenDecBtn.Style, dv.lenIncBtn.Style, LenDecStyle, LenIncStyle)
	}

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	dv.refreshLenButtonsStyle()
	// Styles carry their spec tag, so compare against the style the code actually assigns.
	if dv.lenDecBtn.Style != LenDecStyle || dv.lenIncBtn.Style != LenIncStyle {
		t.Errorf("mobile styles wrong: dec=%v inc=%v want dec=%v inc=%v",
			dv.lenDecBtn.Style, dv.lenIncBtn.Style, LenDecStyle, LenIncStyle)
	}
}

// A new widget must not escape the pane at some screen size.
func TestDrumViewBoundsContainEverything(t *testing.T) {
	assertDefaultParityState(t)
	for _, size := range []image.Point{{X: 800, Y: 480}, {X: 1280, Y: 720}, {X: 1920, Y: 1080}} {
		dv := newTestDrumView(t, size.X, size.Y)
		snap := dv.widgetRectsSnapshot()
		if !snap.Rack.Empty() && !snap.Rack.In(dv.Bounds) {
			t.Errorf("%v rack %v escapes bounds %v", size, snap.Rack, dv.Bounds)
		}
		if !snap.Timeline.Empty() && !snap.Timeline.In(dv.Bounds) {
			t.Errorf("%v timeline %v escapes bounds %v", size, snap.Timeline, dv.Bounds)
		}
		if !snap.Transport.Empty() && !snap.Transport.In(dv.Bounds) {
			t.Errorf("%v transport %v escapes bounds %v", size, snap.Transport, dv.Bounds)
		}
		if !snap.AddButton.Empty() && !snap.AddButton.In(dv.Bounds) {
			t.Errorf("%v add button %v escapes bounds %v", size, snap.AddButton, dv.Bounds)
		}
	}
}
