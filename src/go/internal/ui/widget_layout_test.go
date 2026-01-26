package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func newTestDrumView(t *testing.T, w, h int) *DrumView {
	t.Helper()
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	return NewDrumView(image.Rect(0, 0, w, h), nil, logger)
}

func TestAddButtonInsideRackWidget(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	snap := dv.widgetRectsSnapshot()
	if snap.Rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if !snap.AddButton.In(snap.Rack) {
		t.Fatalf("add button must live inside rack widget; add=%v rack=%v", snap.AddButton, snap.Rack)
	}
}

func TestWidgetResizeAdjustsColumns(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1200, 700)
	beforeRack := dv.widgetRects[WidgetRack].Dx()
	beforeTimeline := dv.widgetRects[WidgetTimeline].Dx()
	dv.widgets.ResizeAxis("col", 0, 40) // expand left column
	dv.refreshWidgetLayout()
	afterRack := dv.widgetRects[WidgetRack].Dx()
	afterTimeline := dv.widgetRects[WidgetTimeline].Dx()
	if !(afterRack > beforeRack) {
		t.Fatalf("expected rack to grow after resize: before=%d after=%d", beforeRack, afterRack)
	}
	if !(afterTimeline < beforeTimeline) {
		t.Fatalf("expected timeline to shrink after resize: before=%d after=%d", beforeTimeline, afterTimeline)
	}
}

func TestToggleWaveWidget(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1024, 600)
	dv.widgets.ToggleWidget(WidgetWave, false)
	dv.refreshWidgetLayout()
	snap := dv.widgetRectsSnapshot()
	if !snap.Wave.Empty() {
		t.Fatalf("wave widget should be hidden; got %v", snap.Wave)
	}
	// Re-enable and ensure EQ height returns.
	dv.widgets.ToggleWidget(WidgetWave, true)
	dv.refreshWidgetLayout()
	snap = dv.widgetRectsSnapshot()
	if snap.Wave.Empty() {
		t.Fatalf("wave widget did not reappear after toggle")
	}
}

// The bottom Wave/EQ widget should occupy the full width of the bottom row so
// no black gap hides the left column when the EQ panel grows.
func TestWaveWidgetSpansFullWidth(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	wave := dv.widgetRects[WidgetWave]
	if wave.Empty() {
		t.Fatalf("wave rect empty")
	}
	if wave.Min.X != dv.Bounds.Min.X || wave.Max.X != dv.Bounds.Max.X {
		t.Fatalf("wave must span entire width of bottom row; got wave=%v bounds=%v", wave, dv.Bounds)
	}
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if rack.Max.Y != wave.Min.Y {
		t.Fatalf("rack and wave rows should be contiguous without overlap/gap: rack=%v wave=%v", rack, wave)
	}
}

func TestWidgetSnapshotCounts(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 900, 540)
	snap := dv.widgetRectsSnapshot()
	if len(snap.Layout.Widgets) < 3 {
		t.Fatalf("expected at least 3 widgets in snapshot, got %d", len(snap.Layout.Widgets))
	}
}

func TestWidgetHoverDragResizes(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1200, 700)
	mx, my := 0, 0
	left := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	before := dv.widgetRects[WidgetRack].Dx()
	colX := dv.widgets.colPos[1]

	// Hover near boundary then drag
	mx, my = colX, dv.Bounds.Min.Y+10
	dv.handleLayoutResize() // hover sets hoverIdx
	left = true
	dv.handleLayoutResize() // start drag
	mx += 40
	dv.handleLayoutResize() // apply delta
	left = false
	dv.handleLayoutResize() // release

	after := dv.widgetRects[WidgetRack].Dx()
	if !(after > before) {
		t.Fatalf("dragging boundary should widen rack: before=%d after=%d", before, after)
	}
}

func TestTopLevelControlsBoundedByWidgets(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	rack := dv.widgetRects[WidgetRack]
	tl := dv.widgetRects[WidgetTimeline]
	tr := dv.widgetRects[WidgetTransport]
	wave := dv.widgetRects[WidgetWave]
	if rack.Empty() || tl.Empty() || tr.Empty() || wave.Empty() {
		t.Fatalf("widget rects must be non-empty: rack=%v tl=%v tr=%v wave=%v", rack, tl, tr, wave)
	}
	inside := func(r image.Rectangle, w image.Rectangle) bool { return r.In(w) }

	controls := []struct {
		name string
		rect image.Rectangle
		box  image.Rectangle
	}{
		{"play", dv.playBtn.Rect(), tr},
		{"stop", dv.stopBtn.Rect(), tr},
		{"bpmBox", dv.bpmBox.Rect, tr},
		{"bpmInc", dv.bpmIncBtn.Rect(), tr},
		{"bpmDec", dv.bpmDecBtn.Rect(), tr},
		{"lenInc", dv.lenIncBtn.Rect(), tr},
		{"lenDec", dv.lenDecBtn.Rect(), tr},
		{"track", dv.trackBtn.Rect(), tr},
		{"upload", dv.uploadBtn.Rect(), tr},
		{"import", dv.importBtn.Rect(), tr},
		{"export", dv.exportBtn.Rect(), tr},
		{"rowLabel0", dv.rowLabels[0].Rect(), rack},
		{"rowEdit0", dv.rowEditBtns[0].Rect(), rack},
		{"rowSave0", dv.rowSaveBtns[0].Rect(), rack},
		{"addRow", dv.addRowBtn.Rect(), rack},
		{"timeline", dv.timelineRect, tl},
		{"eq", dv.eqRect, wave},
		{"eqToggle", dv.eqToggleBtn.Rect(), wave},
	}
	for _, c := range controls {
		if c.rect.Empty() {
			t.Fatalf("%s rect empty", c.name)
		}
		if !inside(c.rect, c.box) {
			t.Fatalf("%s must live inside widget: rect=%v widget=%v", c.name, c.rect, c.box)
		}
	}
}

func TestControlsStayBoundedAfterWaveGrow(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	// Grow wave widget by shrinking middle row height.
	dv.widgets.ResizeAxis("row", 1, -300)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	tl := dv.widgetRects[WidgetTimeline]
	tr := dv.widgetRects[WidgetTransport]
	wave := dv.widgetRects[WidgetWave]
	if rack.Empty() || tl.Empty() || tr.Empty() || wave.Empty() {
		t.Fatalf("widget rects must be non-empty after resize: rack=%v tl=%v tr=%v wave=%v", rack, tl, tr, wave)
	}
	inside := func(r image.Rectangle, w image.Rectangle) bool { return r.In(w) }
	items := []struct {
		name string
		rect image.Rectangle
		box  image.Rectangle
	}{
		{"addRow", dv.addRowBtn.Rect(), rack},
		{"rowLabel0", dv.rowLabels[0].Rect(), rack},
		{"timeline", dv.timelineRect, tl},
		{"eq", dv.eqRect, wave},
	}
	for _, it := range items {
		if it.rect.Empty() {
			t.Fatalf("%s rect empty after resize", it.name)
		}
		if !inside(it.rect, it.box) {
			t.Fatalf("%s escaped widget after resize: rect=%v widget=%v", it.name, it.rect, it.box)
		}
	}
}

func TestAddRowButtonStaysAfterLastRow(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1100, 520)
	// Force rack height to be tight to reproduce floating issue.
	dv.widgets.ResizeAxis("row", 1, -260)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rowsTop := dv.Bounds.Min.Y + dv.headerH
	expected := rowsTop + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight() + buttonPad
	got := dv.addRowBtn.Rect().Min.Y
	if got != expected {
		t.Fatalf("add row button should follow last row. got Y=%d expected=%d (rowsTop=%d rows=%d offset=%d)", got, expected, rowsTop, len(dv.Rows), dv.rowOffset)
	}
	// Ensure it still sits inside rack horizontally.
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if !(dv.addRowBtn.Rect().Min.X >= rack.Min.X && dv.addRowBtn.Rect().Max.X <= rack.Max.X) {
		t.Fatalf("add button must stay within rack width: btn=%v rack=%v", dv.addRowBtn.Rect(), rack)
	}
}

func TestAddRowButtonAutoScrollsIntoViewOnShrink(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1100, 420)
	// Add extra rows to force scrolling.
	for i := 0; i < 6; i++ {
		dv.AddRow()
	}
	dv.widgets.ResizeAxis("row", 1, -260) // shrink rack area
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset != maxOff {
		t.Fatalf("rowOffset should clamp to end after shrink. got=%d want=%d", dv.rowOffset, maxOff)
	}
	btn := dv.addRowBtn.Rect()
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if btn.Max.Y > rack.Max.Y || btn.Min.Y < rack.Min.Y {
		t.Fatalf("add button not within rack after shrink: btn=%v rack=%v", btn, rack)
	}
	// Ensure the button is positioned immediately after the last visible row offset.
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	wantY := rowsTop + (len(dv.Rows)-dv.rowOffset)*dv.rowHeight() + buttonPad
	if btn.Min.Y != wantY {
		t.Fatalf("add button Y mismatch after shrink: got=%d want=%d (rowOffset=%d vis=%d)", btn.Min.Y, wantY, dv.rowOffset, dv.visibleRows())
	}
}

func TestTransportButtonsResizeWithWidgetHeight(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	basePlay := dv.playBtn.Rect()
	baseUpload := dv.uploadBtn.Rect()
	if basePlay.Dy() <= 0 || baseUpload.Dy() <= 0 {
		t.Fatalf("base button heights invalid: play=%v upload=%v", basePlay, baseUpload)
	}

	// Grow height and ensure both rows expand together.
	dv.widgets.SetRowHeight(0, 120)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	largePlay := dv.playBtn.Rect()
	largeUpload := dv.uploadBtn.Rect()
	if diff := absInt(largePlay.Dy() - largeUpload.Dy()); diff > 10 {
		t.Fatalf("grown heights should stay aligned: play=%d upload=%d diff=%d", largePlay.Dy(), largeUpload.Dy(), diff)
	}
	// Padding: ensure gap between play and stop exists
	if !(dv.stopBtn.Rect().Min.X-dv.playBtn.Rect().Max.X > 1) {
		t.Fatalf("expected horizontal padding between play and stop buttons: play=%v stop=%v", dv.playBtn.Rect(), dv.stopBtn.Rect())
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
