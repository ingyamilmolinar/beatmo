//go:build test

package ui

import (
	"image"
	"testing"
)

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

// Row divider 0 is suppressed (the timeline spans rows 0-1); divider 1, the EQ boundary, is a pill-only handle.
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

	// detectRowDivider still returns the EQ index; HandleInput filters by pill proximity, so hover stays off.
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

// The EQ divider's hover/grab footprint is the visible pill plus SpaceSM, never a band reaching into the sticky
// bar: reaching for a tab control must not light the divider or flip the resize cursor.
func TestEQDividerHoverStaysOnPill(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}
	pill := h.rowHandleRect(idx)
	if pill.Empty() {
		t.Fatal("EQ boundary pill is empty")
	}
	cx := (pill.Min.X + pill.Max.X) / 2
	cy := (pill.Min.Y + pill.Max.Y) / 2

	// Off the pill horizontally, but well within the OLD wide grab band.
	offX := pill.Max.X + SpaceSM + 8
	h.syncHoverState("", -1)
	h.HandleInput(offX, cy, false)
	if dv.layoutHoverAxis != "" {
		t.Errorf("EQ divider should NOT hover off the pill horizontally (offX=%d, pill X=[%d,%d]): got axis=%q idx=%d",
			offX, pill.Min.X, pill.Max.X, dv.layoutHoverAxis, dv.layoutHoverIdx)
	}

	// Below the pill, within the OLD downward band that overlapped the sticky bar.
	belowY := pill.Max.Y + SpaceSM + 8
	h.syncHoverState("", -1)
	h.HandleInput(cx, belowY, false)
	if dv.layoutHoverAxis != "" {
		t.Errorf("EQ divider should NOT hover below the pill (belowY=%d, pill bottom=%d): got axis=%q idx=%d",
			belowY, pill.Max.Y, dv.layoutHoverAxis, dv.layoutHoverIdx)
	}

	// On the pill — hover SHOULD activate.
	h.syncHoverState("", -1)
	h.HandleInput(cx, cy, false)
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != idx {
		t.Errorf("EQ divider SHOULD hover on the pill: got axis=%q idx=%d, want axis=\"row\" idx=%d",
			dv.layoutHoverAxis, dv.layoutHoverIdx, idx)
	}
}
