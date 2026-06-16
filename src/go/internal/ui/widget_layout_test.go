package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
	if afterRack <= beforeRack {
		t.Fatalf("expected rack to grow after resize: before=%d after=%d", beforeRack, afterRack)
	}
	if afterTimeline >= beforeTimeline {
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
	if after <= before {
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
		{"play", dv.playBtn().Rect(), tr},
		{"stop", dv.stopBtn().Rect(), tr},
		{"bpmBox", dv.bpmBox().Rect, tr},
		{"bpmInc", dv.bpmIncBtn().Rect(), tr},
		{"bpmDec", dv.bpmDecBtn().Rect(), tr},
		{"lenInc", dv.lenIncBtn.Rect(), tl},
		{"lenDec", dv.lenDecBtn.Rect(), tl},
		{"track", dv.trackBtn().Rect(), tl},
		// File ops (upload/import/export) moved behind the overflow "..." menu
		// on desktop; the overflow button is the only inline file-ops control
		// and must stay bounded by the transport widget.
		{"overflow", dv.overflowBtn().Rect(), tr},
		{"rowLabel0", dv.rowLabels()[0].Rect(), rack},
		// rowEdit0 and rowSave0 are hidden on desktop (empty rects).
		{"addRow", dv.addRowBtn().Rect(), rack},
		{"timeline", dv.timelineRect, tl},
		{"eq", dv.eqRect, wave},
		{"eqToggle", dv.eqToggleBtn().Rect(), wave},
	}
	for _, c := range controls {
		if c.rect.Empty() {
			t.Fatalf("%s rect empty", c.name)
		}
		if !inside(c.rect, c.box) {
			t.Fatalf("%s must live inside widget: rect=%v widget=%v", c.name, c.rect, c.box)
		}
	}
	// File ops are hidden inline on desktop (reachable via the overflow menu);
	// guard against them leaking back into the toolbar.
	for _, h := range []struct {
		name string
		rect image.Rectangle
	}{
		{"upload", dv.uploadBtn().Rect()},
		{"import", dv.importBtn().Rect()},
		{"export", dv.exportBtn().Rect()},
	} {
		if !h.rect.Empty() {
			t.Fatalf("%s should be hidden inline (behind overflow menu), got %v", h.name, h.rect)
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
		{"addRow", dv.addRowBtn().Rect(), rack},
		{"rowLabel0", dv.rowLabels()[0].Rect(), rack},
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

// TestAddRowButtonAnchoredToRackBottom asserts the addRow button is
// anchored to the bottom of the rack column — independent of row count
// or scroll offset. (Previously this test asserted "follows last row",
// which made the button a moving target; see footer_button_anchor_test.go
// for the cross-row-count invariant.)
func TestAddRowButtonAnchoredToRackBottom(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1100, 520)
	dv.widgets.ResizeAxis("row", 1, -260)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	rh := dv.rowHeight()
	// Anchor reference is the rack ZONE rect (z.rect after the
	// rowsBottom() clamp in drumview_layout.go) — NOT the raw
	// widgetRects[WidgetRack] which extends to dv.Bounds.Max.Y. The
	// addRow's top edge sits at `zoneRect.Max.Y - rh + SpaceXS`.
	zoneRect := dv.rowRackZone.rect
	expected := zoneRect.Max.Y - rh + SpaceXS
	got := dv.addRowBtn().Rect().Min.Y
	if got != expected {
		t.Fatalf("addRowBtn must be anchored to rack zone bottom: got Y=%d expected=%d (zoneRect.Max.Y=%d rh=%d)",
			got, expected, zoneRect.Max.Y, rh)
	}
	if dv.addRowBtn().Rect().Min.X < rack.Min.X || dv.addRowBtn().Rect().Max.X > rack.Max.X {
		t.Fatalf("add button must stay within rack width: btn=%v rack=%v", dv.addRowBtn().Rect(), rack)
	}
}

// TestAddRowButtonStaysAnchoredAfterShrink asserts the addRow button
// stays anchored to the rack column's bottom edge after a layout
// shrink, even when scroll has been forced to the end. The button's
// screen position must be the same as before/after the shrink — the
// scroll offset only changes which ROWS are visible, not the button's
// anchor.
func TestAddRowButtonStaysAnchoredAfterShrink(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1100, 420)
	for i := 0; i < 6; i++ {
		dv.AddRow()
	}
	dv.widgets.ResizeAxis("row", 1, -260) // shrink rack area
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	maxOff := len(dv.Rows) - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset != maxOff {
		t.Fatalf("rowOffset should clamp to end after shrink. got=%d want=%d", dv.rowOffset, maxOff)
	}
	btn := dv.addRowBtn().Rect()
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if btn.Max.Y > rack.Max.Y || btn.Min.Y < rack.Min.Y {
		t.Fatalf("add button not within rack after shrink: btn=%v rack=%v", btn, rack)
	}
	// After the 2026-05-10 tight-layout fix, addRow lives in the band
	// `[rowsTop + rendered*rh, zoneRect.Max.Y - rh]` where `rendered` is
	// the number of rows actually painted (≤ capacity). When the rack
	// is heavily oversubscribed (this test adds 6 rows then shrinks),
	// `rendered == capacity` and `flushY == zoneRect.Max.Y - rh - slack`
	// where slack is the rack-height modulo (< rh). Either anchor (flush
	// or bottom-pinned) is acceptable; both keep the button inside the
	// rack and within one rh of the rack bottom.
	rh := dv.rowHeight()
	zoneRect := dv.rowRackZone.rect
	bottomAnchor := zoneRect.Max.Y - rh + SpaceXS
	maxOffsetFromBottom := rh
	if delta := bottomAnchor - btn.Min.Y; delta < 0 || delta > maxOffsetFromBottom {
		t.Fatalf("addRowBtn out of allowed band after shrink: got=%d expected within [%d, %d] (zoneRect.Max.Y=%d rh=%d rowOffset=%d)",
			btn.Min.Y, bottomAnchor-maxOffsetFromBottom, bottomAnchor, zoneRect.Max.Y, rh, dv.rowOffset)
	}
}

func TestTransportButtonsResizeWithWidgetHeight(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	basePlay := dv.playBtn().Rect()
	// Upload is hidden inline on desktop (behind the overflow menu); use the
	// overflow button — the rightmost inline control in the same row — as the
	// representative right-edge button for the resize check.
	baseOverflow := dv.overflowBtn().Rect()
	if basePlay.Dy() <= 0 || baseOverflow.Dy() <= 0 {
		t.Fatalf("base button heights invalid: play=%v overflow=%v", basePlay, baseOverflow)
	}

	// Grow height and ensure both rows expand together.
	dv.widgets.SetRowHeight(0, 120)
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	largePlay := dv.playBtn().Rect()
	largeOverflow := dv.overflowBtn().Rect()
	if diff := absInt(largePlay.Dy() - largeOverflow.Dy()); diff > 10 {
		t.Fatalf("grown heights should stay aligned: play=%d overflow=%d diff=%d", largePlay.Dy(), largeOverflow.Dy(), diff)
	}
	// Padding: ensure gap between play and stop exists
	if dv.stopBtn().Rect().Min.X-dv.playBtn().Rect().Max.X <= 1 {
		t.Fatalf("expected horizontal padding between play and stop buttons: play=%v stop=%v", dv.playBtn().Rect(), dv.stopBtn().Rect())
	}
}

func TestAddRowButtonNeverOverlapsEQPanel(t *testing.T) {
	assertDefaultParityState(t)

	// Phase 0a audio-panel redesign raised the default panel height
	// multiplier to 3×; this test exercises the legacy 180-px layout
	// where the rows area is sized for many rows. Pin the multiplier
	// back to 1× for the duration of this test (and re-derive the
	// active runtime profile so the change takes effect).
	restoreProfile := SetRuntimeProfileForTest(func() *RuntimeProfile {
		p := *RuntimeProf()
		p.AudioPanelHeightMultiplier = 1
		p.AudioPanelHeightScreenFrac = 0.0
		return &p
	}())
	t.Cleanup(restoreProfile)

	dv := newTestDrumView(t, 1280, 720)

	// Add enough rows to exceed visible area.
	for len(dv.Rows) < 20 {
		dv.AddRow()
	}

	// Set production eqPanelHeight AFTER construction (recalcButtons resets
	// it to 0 under test). This must be set before refreshWidgetLayout so
	// the "runningUnderGoTest && eqPanelHeight==0" guard doesn't zero eqH.
	eqPanelHeight = 180
	t.Cleanup(func() { eqPanelHeight = 0 })

	dv.eqH = 180
	dv.refreshWidgetLayout()
	dv.calcLayout()

	wave := dv.widgetRects[WidgetWave]
	if wave.Empty() {
		t.Fatalf("wave rect empty — eqH not applied")
	}

	btn := dv.addRowBtn().Rect()
	if btn.Empty() {
		t.Fatalf("add button rect empty")
	}
	if btn.Max.Y > wave.Min.Y {
		t.Fatalf("add button overlaps EQ panel: btn.Max.Y=%d > wave.Min.Y=%d (btn=%v wave=%v)",
			btn.Max.Y, wave.Min.Y, btn, wave)
	}

	// Stronger invariant: button must stay within the rows area.
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	rowsBottom := rowsTop + dv.rowsAreaHeight()
	if btn.Max.Y > rowsBottom {
		t.Fatalf("add button extends below rows area: btn.Max.Y=%d > rowsBottom=%d (eqH=%d rowsAreaH=%d)",
			btn.Max.Y, rowsBottom, dv.eqH, dv.rowsAreaHeight())
	}

	// Also test with few rows (should still not overlap).
	dv2 := newTestDrumView(t, 1280, 720)
	dv2.eqH = 180
	dv2.refreshWidgetLayout()
	dv2.calcLayout()
	wave2 := dv2.widgetRects[WidgetWave]
	btn2 := dv2.addRowBtn().Rect()
	if !btn2.Empty() && !wave2.Empty() && btn2.Max.Y > wave2.Min.Y {
		t.Fatalf("add button overlaps EQ panel with default rows: btn=%v wave=%v", btn2, wave2)
	}
}

// TestAddRowButtonNoOverlapAfterUpdateRowRects verifies the per-frame
// fast path (updateRowRects) positions the "+" button identically to
// calcLayout. This catches the historical bug where updateRowRects
// capped the slot to visibleRows()-1 instead of visibleRows().
func TestAddRowButtonNoOverlapAfterUpdateRowRects(t *testing.T) {
	assertDefaultParityState(t)

	// --- Sub-test 1: parity between calcLayout and updateRowRects ---
	t.Run("parity", func(t *testing.T) {
		dv := newTestDrumView(t, 1280, 720)
		// Add rows to exceed visible area and force scrolling.
		for len(dv.Rows) < dv.visibleRows()+2 {
			dv.AddRow()
		}
		dv.calcLayout()
		btnAfterCalc := dv.addRowBtn().Rect()
		if btnAfterCalc.Empty() {
			t.Fatalf("add button rect empty after calcLayout")
		}
		dv.updateRowRects()
		btnAfterUpdate := dv.addRowBtn().Rect()
		if btnAfterUpdate.Empty() {
			t.Fatalf("add button rect empty after updateRowRects")
		}
		if btnAfterCalc != btnAfterUpdate {
			t.Fatalf("calcLayout and updateRowRects produce different addRowBtn rects: calc=%v update=%v",
				btnAfterCalc, btnAfterUpdate)
		}
	})

	// --- Sub-test 2: no overlap when rows fit in rack ---
	t.Run("no_overlap_few_rows", func(t *testing.T) {
		dv := newTestDrumView(t, 1280, 720)
		// Use a small number of rows that definitely fit in the rack.
		for len(dv.Rows) < 4 {
			dv.AddRow()
		}
		dv.updateRowRects()

		btn := dv.addRowBtn().Rect()
		if btn.Empty() {
			t.Fatalf("add button rect empty after updateRowRects")
		}
		for i := 0; i < len(dv.rowLabels()); i++ {
			rowRect := dv.rowLabels()[i].Rect()
			if rowRect.Empty() {
				continue
			}
			if btn.Overlaps(rowRect) {
				t.Fatalf("add button overlaps row %d label: btn=%v row=%v", i, btn, rowRect)
			}
		}
		// Verify the button starts at or below the last row's bottom.
		lastIdx := len(dv.rowLabels()) - 1
		if lastIdx >= 0 {
			lastRowBottom := dv.rowLabels()[lastIdx].Rect().Max.Y
			if btn.Min.Y < lastRowBottom {
				t.Fatalf("add button starts above last row bottom: btn.Min.Y=%d lastRowBottom=%d",
					btn.Min.Y, lastRowBottom)
			}
		}
	})

	// --- Sub-test 3: row widget parity ---
	t.Run("row_widget_parity", func(t *testing.T) {
		dv := newTestDrumView(t, 1280, 720)
		for len(dv.Rows) < 3 {
			dv.AddRow()
		}
		dv.calcLayout()
		rects := make([]image.Rectangle, len(dv.rowLabels()))
		for i := range dv.rowLabels() {
			rects[i] = dv.rowLabels()[i].Rect()
		}
		dv.updateRowRects()
		for i := range dv.rowLabels() {
			if dv.rowLabels()[i].Rect() != rects[i] {
				t.Fatalf("row %d label rect differs: calcLayout=%v updateRowRects=%v",
					i, rects[i], dv.rowLabels()[i].Rect())
			}
		}
	})
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
