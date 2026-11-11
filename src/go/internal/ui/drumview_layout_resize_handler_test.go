//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestWidgetSpansColumn(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Wave/EQ widget is at row 2, spans both columns (ColSpan=2)
	// Row 0: Transport (col 0), Timeline (col 1) - no spanning
	// Row 1: Rack (col 0), Timeline (col 1) - no spanning
	// Row 2: Wave (col 0, ColSpan=2) - spans column boundary

	// Column divider is between col 0 and col 1 (index 0)
	// Row 0 should NOT span (Transport is col 0 only, Timeline is col 1 only)
	if h.widgetSpansColumn(0, 0) {
		t.Error("Row 0 should not span column boundary (Transport col=0, Timeline col=1)")
	}

	// Row 1 should NOT span (Rack is col 0 only, Timeline spans row 0-1 but col 1 only)
	if h.widgetSpansColumn(0, 1) {
		t.Error("Row 1 should not span column boundary (Rack col=0, Timeline col=1)")
	}

	// Row 2 SHOULD span (Wave is col 0, ColSpan=2)
	if !h.widgetSpansColumn(0, 2) {
		t.Error("Row 2 should span column boundary (Wave col=0, ColSpan=2)")
	}
}

func TestColumnDividerSegments(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get segments for column divider between col 0 and col 1
	segments := h.columnDividerSegments(0)

	// Should have 2 segments (rows 0 and 1), not 3 (row 2 spans)
	if len(segments) != 2 {
		t.Errorf("Expected 2 segments (rows 0,1), got %d", len(segments))
		for i, seg := range segments {
			t.Logf("  segment[%d]: %v", i, seg)
		}
	}

	// Verify segments are in the correct Y ranges
	if len(segments) >= 2 {
		// Each segment should span its row's Y range
		row0Y := dv.widgets.rowPos[0] + dv.widgets.offset.Y
		row1Y := dv.widgets.rowPos[1] + dv.widgets.offset.Y
		row2Y := dv.widgets.rowPos[2] + dv.widgets.offset.Y

		if segments[0].Min.Y != row0Y || segments[0].Max.Y != row1Y {
			t.Errorf("Segment 0 Y range mismatch: got [%d,%d], expected [%d,%d]",
				segments[0].Min.Y, segments[0].Max.Y, row0Y, row1Y)
		}
		if segments[1].Min.Y != row1Y || segments[1].Max.Y != row2Y {
			t.Errorf("Segment 1 Y range mismatch: got [%d,%d], expected [%d,%d]",
				segments[1].Min.Y, segments[1].Max.Y, row1Y, row2Y)
		}
	}
}

func TestDetectColumnDivider_ValidInRackArea(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get the column divider X position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X

	// Get a Y position in row 1 (Rack area)
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10 // 10px into row 1

	axis, idx := h.detectColumnDivider(colX, rackY)
	if axis != "col" || idx != 0 {
		t.Errorf("Column divider should be detected in Rack area: got axis=%q idx=%d, expected axis=\"col\" idx=0", axis, idx)
	}
}

func TestDetectColumnDivider_NotInEQArea(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get the column divider X position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X

	// Get a Y position in row 2 (EQ/Wave area)
	eqY := dv.widgets.rowPos[2] + dv.widgets.offset.Y + 10 // 10px into row 2

	axis, idx := h.detectColumnDivider(colX, eqY)
	if axis != "" || idx != -1 {
		t.Errorf("Column divider should NOT be detected in EQ area: got axis=%q idx=%d, expected axis=\"\" idx=-1", axis, idx)
	}
}

func TestLayoutResizeHandler_CaptureSemanticsOutsideBounds(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Detect the column divider to get its index
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10
	_, idx := h.detectColumnDivider(colX, rackY)
	if idx < 0 {
		t.Fatal("column divider not detected")
	}
	// Click on the pill handle center to initiate drag
	hr := h.columnHandleRect(idx)
	hcx := (hr.Min.X + hr.Max.X) / 2
	hcy := (hr.Min.Y + hr.Max.Y) / 2

	// Start drag on handle
	result := h.HandleInput(hcx, hcy, true)
	if result != InputConsumed {
		t.Errorf("Starting drag should return InputConsumed, got %v", result)
	}
	if !h.Capturing() {
		t.Error("Handler should be capturing after starting drag")
	}

	// Move cursor outside DrumView bounds (above)
	outsideY := dv.Bounds.Min.Y - 50
	result = h.HandleInput(colX, outsideY, true)
	if result != InputCaptured {
		t.Errorf("Drag should continue outside bounds with InputCaptured, got %v", result)
	}
	if !h.Capturing() {
		t.Error("Handler should still be capturing during drag outside bounds")
	}

	// Release mouse
	result = h.HandleInput(colX, outsideY, false)
	if h.Capturing() {
		t.Error("Handler should stop capturing after mouse release")
	}
}

func TestLayoutResizeHandler_ResizeApplied(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 1200, 600))
	h := dv.layoutHandler

	// Record initial column width
	initialColWidth := dv.widgets.ColWidth(0)

	// Detect column divider to get handle position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10
	_, idx := h.detectColumnDivider(colX, rackY)
	if idx < 0 {
		t.Fatal("column divider not detected")
	}
	hr := h.columnHandleRect(idx)
	hcx := (hr.Min.X + hr.Max.X) / 2
	hcy := (hr.Min.Y + hr.Max.Y) / 2

	// Start drag on handle
	h.HandleInput(hcx, hcy, true)

	// Drag right by 20 pixels
	h.HandleInput(hcx+20, hcy, true)

	// Release
	h.HandleInput(hcx+20, hcy, false)

	// Check that column was resized
	newColWidth := dv.widgets.ColWidth(0)
	if newColWidth != initialColWidth+20 {
		t.Errorf("Column width should increase by 20: got %d (initial %d, expected %d)",
			newColWidth, initialColWidth, initialColWidth+20)
	}
}

func TestLayoutResizeHandler_HandleInput_NotOnDivider(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Click in the middle of the rack area (not near divider)
	midX := dv.widgets.colPos[0] + dv.widgets.offset.X + 50 // 50px into col 0
	midY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 20

	result := h.HandleInput(midX, midY, true)
	if result != InputIgnored {
		t.Errorf("Click away from dividers should return InputIgnored, got %v", result)
	}
	if h.Capturing() {
		t.Error("Handler should not be capturing when click is not on divider")
	}
}

func TestLayoutResizeHandler_RowDividerDetection(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row 0 divider (between row 0 and row 1) is suppressed because the
	// Timeline widget spans rows 0-1 (anyWidgetSpansRow returns true).
	rowY := dv.widgets.rowPos[1] + dv.widgets.offset.Y
	midX := dv.widgets.colPos[0] + dv.widgets.offset.X + dv.widgets.ColWidth(0)/2

	axis, idx := h.detectRowDivider(midX, rowY)
	if axis != "" || idx != -1 {
		t.Errorf("Row 0 divider should NOT be detected (Timeline spans rows 0-1): got axis=%q idx=%d", axis, idx)
	}

	// Row 1 divider (between row 1 and row 2) IS detectable via the EQ
	// boundary's central grab handle (fullWidthWidgetBelow). The handle is
	// centred horizontally — clear of the sticky-bar's edge controls — so the
	// divider is detected at the panel centre, not in the rack column.
	rowY2 := dv.widgets.rowPos[2] + dv.widgets.offset.Y
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2
	axis2, idx2 := h.detectRowDivider(cx, rowY2)
	if axis2 != "row" || idx2 != 1 {
		t.Errorf("Row 1 divider should be detected at the central EQ handle: got axis=%q idx=%d, expected axis=\"row\" idx=1", axis2, idx2)
	}
}

func TestFullWidthWidgetBelow(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row 2 has WidgetWave with ColSpan=2 (full width).
	// fullWidthWidgetBelow(1) should be true (row 2 is below row 1).
	if !h.fullWidthWidgetBelow(1) {
		t.Error("fullWidthWidgetBelow(1) should be true — WidgetWave at row 2 spans full width")
	}

	// fullWidthWidgetBelow(0) should be false (row 1 has Rack col=0 + Timeline col=1).
	if h.fullWidthWidgetBelow(0) {
		t.Error("fullWidthWidgetBelow(0) should be false — row 1 has separate widgets per column")
	}
}

func TestRowDividerSuppressedWhenFullWidthBelow(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row divider at index 1 (between rows 1 and 2) should be suppressed
	// because WidgetWave at row 2 spans full width.
	segments := h.rowDividerSegments(1)
	if segments != nil {
		t.Errorf("rowDividerSegments(1) should return nil when full-width widget is below, got %d segments", len(segments))
	}
}

func TestRowDividerSuppressedForRow0(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row divider at index 0 (between rows 0 and 1) should be suppressed
	// because the Timeline widget spans rows 0-1 (anyWidgetSpansRow).
	segments := h.rowDividerSegments(0)
	if segments != nil {
		t.Errorf("rowDividerSegments(0) should return nil — Timeline spans rows 0-1, got %d segments", len(segments))
	}
}

func TestColumnHandleRect_CenteredOnSegments(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	hr := h.columnHandleRect(0)
	if hr.Empty() {
		t.Fatal("columnHandleRect(0) should not be empty")
	}
	hcy := (hr.Min.Y + hr.Max.Y) / 2

	// The pill Y-center should fall within the rows 0-1 range, NOT at
	// the full bounds center (which would include the EQ row).
	row0Y := dv.widgets.rowPos[0] + dv.widgets.offset.Y
	row2Y := dv.widgets.rowPos[2] + dv.widgets.offset.Y
	fullCY := (dv.Bounds.Min.Y + dv.Bounds.Max.Y) / 2

	if hcy < row0Y || hcy > row2Y {
		t.Errorf("pill center Y=%d should be within rows 0-1 range [%d, %d]", hcy, row0Y, row2Y)
	}
	if hcy == fullCY {
		t.Errorf("pill center Y=%d should differ from full bounds center %d", hcy, fullCY)
	}
}

func TestRowHandleRect_EQBoundaryHasPill(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row 1 divider (EQ boundary) should have a pill even though line
	// segments are suppressed, because fullWidthWidgetBelow is true.
	hr := h.rowHandleRect(1)
	if hr.Empty() {
		t.Error("rowHandleRect(1) should NOT be empty — EQ boundary pill")
	}

	// Row 0 divider should be fully suppressed (no pill, no line).
	hr0 := h.rowHandleRect(0)
	if !hr0.Empty() {
		t.Errorf("rowHandleRect(0) should be empty — anyWidgetSpansRow, got %v", hr0)
	}
}

func TestRow1_2DividerDetectableViaPill(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// The row 1/2 divider should be detectable at its central grab handle
	// (the EQ resize pill is centred, clear of the sticky-bar edge controls).
	rowY := dv.widgets.rowPos[2] + dv.widgets.offset.Y
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	axis, idx := h.detectRowDivider(cx, rowY)
	if axis != "row" || idx != 1 {
		t.Errorf("Row 1/2 divider should be detectable at the central EQ handle, got axis=%q idx=%d", axis, idx)
	}
}

// newTestDrumViewWithBounds creates a DrumView with the specified bounds for testing.
func newTestDrumViewWithBounds(bounds image.Rectangle) *DrumView {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(bounds, g, logger)
	// Force widget layout refresh to ensure positions are calculated
	dv.widgets.SetBounds(bounds)
	dv.refreshWidgetLayout()
	return dv
}
