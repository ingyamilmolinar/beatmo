//go:build test

package ui

import (
	"image"
	"testing"
)

// ControlGrid is a reusable geometry helper that lays controls out in an
// adaptive grid (>= 2 columns, more when wide), wraps to multiple rows, and
// scrolls vertically when the rows overflow the content rect. These tests pin
// the contract independently of the synth panel that consumes it.

const (
	tgCellIdealW = 72
	tgCellH      = 90
	tgHGap       = 8
	tgVGap       = 8
)

func newTestControlGrid() *ControlGrid {
	return NewControlGrid(DefaultScrollbarStyle)
}

func TestControlGrid_TwoColumnsWhenNarrow(t *testing.T) {
	g := newTestControlGrid()
	// 160px wide only fits ~2 ideal cells: (160+8)/(72+8) = 2.1 -> 2.
	g.Layout(image.Rect(0, 0, 160, 400), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.Cols() != 2 {
		t.Fatalf("narrow grid: Cols()=%d, want 2", g.Cols())
	}
}

func TestControlGrid_MoreColumnsWhenWide(t *testing.T) {
	g := newTestControlGrid()
	// 400px wide fits ~5 ideal cells.
	g.Layout(image.Rect(0, 0, 400, 400), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.Cols() <= 2 {
		t.Fatalf("wide grid: Cols()=%d, want > 2", g.Cols())
	}
}

func TestControlGrid_SetMaxColsCapsWideGrid(t *testing.T) {
	g := newTestControlGrid()
	g.SetMaxCols(2)
	// 400px would fit ~5 columns adaptively; the cap forces 2.
	g.Layout(image.Rect(0, 0, 400, 400), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.Cols() != 2 {
		t.Fatalf("maxCols=2 on wide grid: Cols()=%d, want 2", g.Cols())
	}
}

func TestControlGrid_SetMaxColsOneOverridesFloor(t *testing.T) {
	g := newTestControlGrid()
	g.SetMaxCols(1)
	// The 2-column floor must yield to an explicit single-column cap (mobile).
	g.Layout(image.Rect(0, 0, 400, 400), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.Cols() != 1 {
		t.Fatalf("maxCols=1: Cols()=%d, want 1 (cap overrides the 2-col floor)", g.Cols())
	}
}

func TestControlGrid_NeverLessThanTwoColumnsWithMultipleItems(t *testing.T) {
	g := newTestControlGrid()
	// Even an absurdly narrow rect must keep 2 columns when there are >=2 items.
	g.Layout(image.Rect(0, 0, 30, 400), 6, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.Cols() != 2 {
		t.Fatalf("min columns: Cols()=%d, want 2", g.Cols())
	}
}

func TestControlGrid_NoScrollWhenFits(t *testing.T) {
	g := newTestControlGrid()
	// 2 items, 1 row, tall rect -> no scroll, both visible.
	g.Layout(image.Rect(0, 0, 200, 400), 2, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if g.HasScroll() {
		t.Fatalf("short list should not scroll")
	}
	for i := 0; i < 2; i++ {
		if _, vis := g.CellRect(i); !vis {
			t.Errorf("cell %d should be visible when nothing scrolls", i)
		}
	}
	if g.VisibleCount() != 2 {
		t.Errorf("VisibleCount()=%d, want 2", g.VisibleCount())
	}
}

func TestControlGrid_ScrollWhenOverflow(t *testing.T) {
	g := newTestControlGrid()
	// 12 items in a narrow + short rect -> 2 cols, 6 rows, only ~1 row visible.
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if !g.HasScroll() {
		t.Fatalf("overflowing list should scroll (rows=%d visible≈1)", (12+g.Cols()-1)/g.Cols())
	}
	// Initially the top row (cells 0,1) is visible; lower cells are not.
	if _, vis := g.CellRect(0); !vis {
		t.Fatal("cell 0 should be visible initially")
	}
	lastIdx := 11
	if _, vis := g.CellRect(lastIdx); vis {
		t.Fatalf("cell %d (bottom row) should be hidden initially", lastIdx)
	}
	before := visibleSet(g, 12)

	// Scroll down one row (HandleWheel steps>0 scrolls UP, so negate to go down).
	g.Scroll().HandleWheel(-1)
	after := visibleSet(g, 12)
	if sameSet(before, after) {
		t.Fatalf("scrolling did not change the visible window: %v", before)
	}
	if _, vis := g.CellRect(0); vis {
		t.Error("cell 0 should be hidden after scrolling down")
	}
}

func TestControlGrid_CellsDoNotOverlapAndFit(t *testing.T) {
	g := newTestControlGrid()
	rect := image.Rect(10, 20, 170, 400)
	g.Layout(rect, 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	var rects []image.Rectangle
	for i := 0; i < 12; i++ {
		r, vis := g.CellRect(i)
		if !vis {
			continue
		}
		if !r.In(rect) {
			t.Errorf("cell %d rect %v not inside content rect %v", i, r, rect)
		}
		rects = append(rects, r)
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Errorf("visible cells overlap: %v vs %v", rects[i], rects[j])
			}
		}
	}
}

func TestControlGrid_ScrollbarReservesWidth(t *testing.T) {
	g := newTestControlGrid()
	rect := image.Rect(0, 0, 160, 120)
	g.Layout(rect, 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if !g.HasScroll() {
		t.Fatal("precondition: grid must scroll for this test")
	}
	bar := g.Scroll().BarRect()
	if bar.Empty() {
		t.Fatal("scrollbar BarRect is empty despite HasScroll()")
	}
	for i := 0; i < 12; i++ {
		r, vis := g.CellRect(i)
		if !vis {
			continue
		}
		if r.Overlaps(bar) {
			t.Errorf("cell %d rect %v overlaps scrollbar track %v", i, r, bar)
		}
	}
}

func TestControlGrid_ThumbDragStepsOneRowAtATime(t *testing.T) {
	g := newTestControlGrid()
	// 12 items, narrow + short -> 2 cols, 6 rows, ~1 visible row, scrolls.
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if !g.HasScroll() {
		t.Fatal("precondition: grid must scroll")
	}
	thumb := g.Scroll().ThumbRect()
	y0 := (thumb.Min.Y + thumb.Max.Y) / 2
	if !g.BeginDrag(y0) {
		t.Fatalf("BeginDrag did not start (thumb=%v y0=%d)", thumb, y0)
	}
	if g.Scroll().VS.First != 0 {
		t.Fatalf("First=%d at drag start, want 0", g.Scroll().VS.First)
	}
	// A drag shorter than one step must NOT move (no hypersensitive jump).
	g.DragTo(y0 + controlGridDragStepPx - 1)
	if got := g.Scroll().VS.First; got != 0 {
		t.Errorf("sub-step drag moved First to %d, want 0", got)
	}
	// Exactly one step -> advance exactly one row.
	g.DragTo(y0 + controlGridDragStepPx)
	if got := g.Scroll().VS.First; got != 1 {
		t.Errorf("one-step drag: First=%d, want 1", got)
	}
	// Three steps from the start -> three rows (anchored to press point).
	g.DragTo(y0 + 3*controlGridDragStepPx)
	if got := g.Scroll().VS.First; got != 3 {
		t.Errorf("three-step drag: First=%d, want 3", got)
	}
	// Dragging back up one step returns to row 2 (reversible, no drift).
	g.DragTo(y0 + 2*controlGridDragStepPx)
	if got := g.Scroll().VS.First; got != 2 {
		t.Errorf("step-back drag: First=%d, want 2", got)
	}
}

func TestControlGrid_ThumbDragClampsAtEnds(t *testing.T) {
	g := newTestControlGrid()
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	maxFirst := g.Scroll().VS.Total - g.Scroll().VS.Visible
	thumb := g.Scroll().ThumbRect()
	y0 := (thumb.Min.Y + thumb.Max.Y) / 2
	if !g.BeginDrag(y0) {
		t.Fatal("drag did not start")
	}
	// Drag far past the bottom -> clamp at maxFirst, never beyond.
	g.DragTo(y0 + 100*controlGridDragStepPx)
	if got := g.Scroll().VS.First; got != maxFirst {
		t.Errorf("over-drag down: First=%d, want maxFirst=%d", got, maxFirst)
	}
	// Drag far above the top -> clamp at 0.
	g.DragTo(y0 - 100*controlGridDragStepPx)
	if got := g.Scroll().VS.First; got != 0 {
		t.Errorf("over-drag up: First=%d, want 0", got)
	}
}

func TestControlGrid_WheelStepsOneRowThenWaitsCooldown(t *testing.T) {
	g := newTestControlGrid()
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	if !g.HasScroll() {
		t.Fatal("precondition: grid must scroll")
	}
	// First wheel-down notch advances exactly one row.
	if !g.WheelStep(-1) {
		t.Fatal("first wheel notch should move one row")
	}
	if got := g.Scroll().VS.First; got != 1 {
		t.Fatalf("after first notch: First=%d, want 1", got)
	}
	// Immediately spinning again (same/next frames) is gated by the cooldown —
	// this is what stops the wheel from flying through the whole list.
	for i := 0; i < controlGridScrollCooldownFrames-1; i++ {
		if g.WheelStep(-1) {
			t.Fatalf("wheel moved during cooldown (frame %d)", i)
		}
		if got := g.Scroll().VS.First; got != 1 {
			t.Fatalf("First changed during cooldown: %d", got)
		}
		g.Tick()
	}
	// One more tick clears the cooldown; the next notch advances one more row.
	g.Tick()
	if !g.WheelStep(-1) {
		t.Fatal("wheel should move again after cooldown elapsed")
	}
	if got := g.Scroll().VS.First; got != 2 {
		t.Fatalf("after cooldown: First=%d, want 2", got)
	}
}

func TestControlGrid_WheelStepIgnoresLargeDeltaMagnitude(t *testing.T) {
	g := newTestControlGrid()
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	// A big wheel delta (fast flick / hi-res wheel) must still move ONE row.
	if !g.WheelStep(-9) {
		t.Fatal("wheel should move")
	}
	if got := g.Scroll().VS.First; got != 1 {
		t.Fatalf("large delta moved %d rows, want exactly 1", got)
	}
}

func TestControlGrid_WheelStepDirection(t *testing.T) {
	g := newTestControlGrid()
	g.Layout(image.Rect(0, 0, 160, 120), 12, tgCellIdealW, tgCellH, tgHGap, tgVGap)
	// Move down two rows (ticking past the cooldown between notches).
	g.WheelStep(-1)
	for i := 0; i <= controlGridScrollCooldownFrames; i++ {
		g.Tick()
	}
	g.WheelStep(-1)
	if got := g.Scroll().VS.First; got != 2 {
		t.Fatalf("setup: First=%d, want 2", got)
	}
	// Wheel up moves toward the top (one row).
	for i := 0; i <= controlGridScrollCooldownFrames; i++ {
		g.Tick()
	}
	if !g.WheelStep(1) {
		t.Fatal("wheel up should move")
	}
	if got := g.Scroll().VS.First; got != 1 {
		t.Fatalf("wheel up: First=%d, want 1", got)
	}
}

func visibleSet(g *ControlGrid, count int) map[int]bool {
	out := map[int]bool{}
	for i := 0; i < count; i++ {
		if _, vis := g.CellRect(i); vis {
			out[i] = true
		}
	}
	return out
}

func sameSet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
