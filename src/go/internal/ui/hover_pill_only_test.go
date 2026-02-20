//go:build test

package ui

import (
	"image"
	"testing"
)

// TestHoverOnlyActivatesNearColumnPill verifies that hover feedback only
// activates when the cursor is near the pill handle, not anywhere on the divider line.
func TestHoverOnlyActivatesNearColumnPill(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get column divider position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X

	// The pill is centered vertically on the full divider line
	handleR := h.columnHandleRect(0)
	pillCY := (handleR.Min.Y + handleR.Max.Y) / 2

	// Point ON the divider but FAR from the pill (well above the pill top)
	farY := handleR.Min.Y - 20 - SpaceSM
	if farY < dv.Bounds.Min.Y {
		farY = dv.Bounds.Min.Y + 5
	}

	// Verify this point is detected as on the column divider segment
	_, idx := h.detectColumnDivider(colX, farY)

	if idx >= 0 {
		// This point is on the divider — hover should NOT activate (far from pill)
		h.HandleInput(colX, farY, false)
		if dv.layoutHoverAxis != "" {
			t.Errorf("Hover should NOT activate far from pill: got axis=%q idx=%d, farY=%d, pill=[%d,%d]",
				dv.layoutHoverAxis, dv.layoutHoverIdx, farY, handleR.Min.Y, handleR.Max.Y)
		}
	} else {
		t.Logf("No valid column divider segment at farY=%d; skipping far-from-pill check", farY)
	}

	// Point AT the pill center — hover SHOULD activate
	h.HandleInput(colX, pillCY, false)
	if dv.layoutHoverAxis != "col" || dv.layoutHoverIdx != 0 {
		t.Errorf("Hover should activate at pill center: got axis=%q idx=%d, expected axis=\"col\" idx=0",
			dv.layoutHoverAxis, dv.layoutHoverIdx)
	}
}

// TestHoverOnlyActivatesNearRowPill verifies that hover feedback only
// activates when the cursor is near the row pill handle.
// Row divider 0 is fully suppressed (Timeline spans rows 0-1).
// Row divider 1 (EQ boundary) has a pill-only handle centered on full width.
func TestHoverOnlyActivatesNearRowPill(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	if len(dv.widgets.rowPos) < 3 {
		t.Fatal("need at least 3 grid rows")
	}

	// Row divider 0 should be fully suppressed (no pill).
	handleR0 := h.rowHandleRect(0)
	if !handleR0.Empty() {
		t.Errorf("row handle rect 0 should be empty (Timeline spans rows 0-1), got %v", handleR0)
	}

	// Row divider 1 (EQ boundary) should have a pill.
	handleR := h.rowHandleRect(1)
	if handleR.Empty() {
		t.Fatal("row handle rect 1 (EQ boundary pill) is empty")
	}

	rowY := dv.widgets.rowPos[2] + dv.widgets.offset.Y
	pillCX := (handleR.Min.X + handleR.Max.X) / 2

	// Point ON the divider Y but FAR from the pill X
	farX := dv.widgets.colPos[0] + dv.widgets.offset.X + 5

	// detectRowDivider returns the index (EQ pill path), but HandleInput
	// further filters by pill proximity, so hover should not activate.
	h.syncHoverState("", -1)
	h.HandleInput(farX, rowY, false)
	if dv.layoutHoverAxis != "" {
		t.Errorf("Hover should NOT activate far from EQ pill: got axis=%q idx=%d, farX=%d, pill=[%d,%d]",
			dv.layoutHoverAxis, dv.layoutHoverIdx, farX, handleR.Min.X, handleR.Max.X)
	}

	// Point AT the pill center — hover SHOULD activate
	h.HandleInput(pillCX, rowY, false)
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != 1 {
		t.Errorf("Hover should activate at EQ pill center: got axis=%q idx=%d, expected axis=\"row\" idx=1",
			dv.layoutHoverAxis, dv.layoutHoverIdx)
	}
}
