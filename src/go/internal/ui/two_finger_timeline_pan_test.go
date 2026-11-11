//go:build test

package ui

import (
	"testing"
)

// growRowsForScroll adds rows until the rack can scroll (total > visible).
func growRowsForScroll(g *Game) {
	for i := 0; len(g.drum.Rows) < g.drum.visibleRows()+5 && i < 32; i++ {
		g.drum.AddRow()
		g.Update()
	}
	g.Update()
}

// twoFingerSwipe drives a two-finger pan through the real Update loop: two
// fingers land centered at (cx,cy), then translate by (perDX,perDY) each frame
// for the given number of frames, then both lift.
func twoFingerSwipe(g *Game, mock *mockTouchState, cx, cy, perDX, perDY, frames int) {
	x1, x2 := cx-50, cx+50
	y1, y2 := cy, cy
	mock.addTouch(1, x1, y1)
	mock.addTouch(2, x2, y2)
	advanceFrames(g, 1) // init two-finger tracking (no gesture on first frame)
	for i := 0; i < frames; i++ {
		x1 += perDX
		x2 += perDX
		y1 += perDY
		y2 += perDY
		mock.moveTouch(1, x1, y1)
		mock.moveTouch(2, x2, y2)
		advanceFrames(g, 1)
	}
	mock.removeTouch(1)
	mock.removeTouch(2)
	advanceFrames(g, 3)
	// Clear the multi-touch cooldown so this gesture never leaks into a later
	// test's RecentMultiTouch() gate (e.g. the timeline scrub adapter).
	globalTouchState.Reset()
}

// cellGridCenter returns a point near the top of the drum cell grid (steps
// region), where a single-finger drag already moves the timeline.
func cellGridCenter(g *Game) (int, int) {
	steps := g.drum.timelineZone.StepsRect()
	return steps.Min.X + steps.Dx()/2, steps.Min.Y + 20
}

// TestTwoFingerHorizontalPanMovesTimeline verifies the NEW behavior: a
// two-finger left/right swipe over the drum cell grid moves the timeline view
// (dv.Offset), exactly like the single-finger horizontal grid drag.
func TestTwoFingerHorizontalPanMovesTimeline(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	growRowsForScroll(g)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	offBefore := g.drum.Offset
	rowOffBefore := g.drum.rowOffset
	camXBefore, camYBefore := g.cam.OffsetX, g.cam.OffsetY

	cx, cy := cellGridCenter(g)
	twoFingerSwipe(g, mock, cx, cy, -24, 0, 6) // swipe left → timeline forward

	if g.drum.Offset <= offBefore {
		t.Fatalf("expected timeline Offset to advance on left swipe; before=%d after=%d", offBefore, g.drum.Offset)
	}
	if g.drum.rowOffset != rowOffBefore {
		t.Fatalf("horizontal two-finger pan must NOT scroll rows; rowOffset before=%d after=%d", rowOffBefore, g.drum.rowOffset)
	}
	if g.cam.OffsetX != camXBefore || g.cam.OffsetY != camYBefore {
		t.Fatalf("horizontal two-finger pan over cell grid must NOT pan the grid camera; cam before=(%.0f,%.0f) after=(%.0f,%.0f)",
			camXBefore, camYBefore, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// TestTwoFingerHorizontalPanDoesNotScrollRows is the explicit bug-fix guard:
// the reported bug was that a left/right two-finger motion ALSO scrolled the
// instrument rows up/down. With direction lock, a horizontal swipe must leave
// the row offset untouched even with a scrollable rack.
func TestTwoFingerHorizontalPanDoesNotScrollRows(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	growRowsForScroll(g)
	if len(g.drum.Rows) <= g.drum.visibleRows() {
		t.Skipf("rack not scrollable (rows=%d visible=%d)", len(g.drum.Rows), g.drum.visibleRows())
	}

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	rowOffBefore := g.drum.rowOffset
	cx, cy := cellGridCenter(g)
	// Slight downward drift each frame to mimic an imperfect horizontal swipe.
	twoFingerSwipe(g, mock, cx, cy, 24, 1, 6) // swipe right, tiny vertical jitter

	if g.drum.rowOffset != rowOffBefore {
		t.Fatalf("left/right two-finger swipe scrolled rows (the bug); rowOffset before=%d after=%d", rowOffBefore, g.drum.rowOffset)
	}
}

// TestTwoFingerVerticalPanScrollsRows verifies the vertical counterpart mirrors
// the single-finger grid drag: a two-finger up/down swipe scrolls the rows and
// leaves the timeline offset untouched.
func TestTwoFingerVerticalPanScrollsRows(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	growRowsForScroll(g)
	if len(g.drum.Rows) <= g.drum.visibleRows() {
		t.Skipf("rack not scrollable (rows=%d visible=%d)", len(g.drum.Rows), g.drum.visibleRows())
	}

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	offBefore := g.drum.Offset
	rowOffBefore := g.drum.rowOffset
	cx, cy := cellGridCenter(g)
	cy = g.drum.timelineZone.StepsRect().Min.Y + g.drum.rowHeight()*3 // mid-grid so an up swipe stays inside
	twoFingerSwipe(g, mock, cx, cy, 0, -g.drum.rowHeight(), 6)        // swipe up → scroll rows down

	if g.drum.rowOffset <= rowOffBefore {
		t.Fatalf("expected vertical two-finger swipe to scroll rows; rowOffset before=%d after=%d", rowOffBefore, g.drum.rowOffset)
	}
	if g.drum.Offset != offBefore {
		t.Fatalf("vertical two-finger pan must NOT move the timeline; Offset before=%d after=%d", offBefore, g.drum.Offset)
	}
}

// TestTwoFingerPanOverGridPaneStillPansCamera is a regression guard: over the
// node/graph pane (NOT the cell grid) a two-finger pan must still pan the
// camera as before.
func TestTwoFingerPanOverGridPaneStillPansCamera(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	camXBefore := g.cam.OffsetX
	cx := g.winW / 2
	cy := gridTopOffset() + 40 // well inside the graph pane
	twoFingerSwipe(g, mock, cx, cy, 20, 0, 6)

	if g.cam.OffsetX == camXBefore {
		t.Fatalf("two-finger pan over the graph pane must still pan the camera; cam.OffsetX unchanged at %.0f", camXBefore)
	}
}

// TestTwoFingerHorizontalPanMovesTimeline_Mobile verifies the same timeline
// behavior on the mobile layout.
func TestTwoFingerHorizontalPanMovesTimeline_Mobile(t *testing.T) {
	setupMobileTest(t, true)
	globalTouchState.Reset()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 780)
	growRowsForScroll(g)

	steps := g.drum.timelineZone.StepsRect()
	if steps.Empty() {
		t.Skip("no cell-grid steps region on this mobile layout")
	}

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	defer resetTouchOverride()

	offBefore := g.drum.Offset
	rowOffBefore := g.drum.rowOffset
	cx := steps.Min.X + steps.Dx()/2
	cy := steps.Min.Y + 20
	twoFingerSwipe(g, mock, cx, cy, -16, 0, 6)

	if g.drum.Offset <= offBefore {
		t.Fatalf("[mobile] expected timeline Offset to advance on left swipe; before=%d after=%d", offBefore, g.drum.Offset)
	}
	if g.drum.rowOffset != rowOffBefore {
		t.Fatalf("[mobile] horizontal two-finger pan must NOT scroll rows; before=%d after=%d", rowOffBefore, g.drum.rowOffset)
	}
}
