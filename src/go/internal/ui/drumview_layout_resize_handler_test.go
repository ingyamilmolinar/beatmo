//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
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

	// Get the column divider position in row 1 (valid area)
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10

	// Start drag
	result := h.HandleInput(colX, rackY, true)
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
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Record initial column width
	initialColWidth := dv.widgets.ColWidth(0)

	// Get the column divider position in row 1 (valid area)
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10

	// Start drag
	h.HandleInput(colX, rackY, true)

	// Drag right by 20 pixels
	h.HandleInput(colX+20, rackY, true)

	// Release
	h.HandleInput(colX+20, rackY, false)

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

	// Get row divider position (between row 0 and row 1)
	rowY := dv.widgets.rowPos[1] + dv.widgets.offset.Y
	midX := dv.Bounds.Min.X + dv.Bounds.Dx()/2

	axis, idx := h.detectRowDivider(midX, rowY)
	if axis != "row" || idx != 0 {
		t.Errorf("Row divider should be detected: got axis=%q idx=%d, expected axis=\"row\" idx=0", axis, idx)
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
