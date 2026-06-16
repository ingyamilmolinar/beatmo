//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// wheelPanHarness wires SetInputForTest so a test can place the cursor and
// drive wheel deltas (the way a trackpad two-finger scroll arrives) through the
// real Game.Update loop. Returns pointers the test mutates between frames.
type wheelPanHarness struct {
	mx, my *int
	wx, wy *float64
}

func newWheelPanHarness(t *testing.T) *wheelPanHarness {
	t.Helper()
	mx, my := new(int), new(int)
	wx, wy := new(float64), new(float64)
	restore := SetInputForTest(
		func() (int, int) { return *mx, *my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return *wx, *wy },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
	return &wheelPanHarness{mx: mx, my: my, wx: wx, wy: wy}
}

// setupWheelGame builds a scrollable game with the cursor parked over the drum
// cell grid (the steps region).
func setupWheelGame(t *testing.T) (*Game, *wheelPanHarness) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	h := newWheelPanHarness(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	growRowsForScroll(g)
	if len(g.drum.Rows) <= g.drum.visibleRows() {
		t.Skipf("rack not scrollable (rows=%d visible=%d)", len(g.drum.Rows), g.drum.visibleRows())
	}
	steps := g.drum.timelineZone.StepsRect()
	*h.mx = steps.Min.X + steps.Dx()/2
	*h.my = steps.Min.Y + 20
	g.Update() // settle cursor position with no wheel
	return g, h
}

// scrollWheelAt parks the cursor at (px,py), then drives n vertical wheel
// notches there, returning the resulting row-offset delta. It first clears any
// pending step cooldown and resets the offset so two regions are compared from
// the same baseline.
func scrollWheelAt(g *Game, h *wheelPanHarness, px, py, notches int) int {
	g.drum.rowRackZone.SetRowOffset(0)
	*h.wx, *h.wy = 0, 0
	for i := 0; i < 20; i++ { // settle + decay the WheelStep cooldown
		g.Update()
	}
	*h.mx, *h.my = px, py
	g.Update()
	start := g.drum.rowOffset
	for i := 0; i < notches; i++ {
		*h.wx, *h.wy = 0, -1 // scroll down one notch
		g.Update()
	}
	*h.wx, *h.wy = 0, 0
	g.Update()
	return g.drum.rowOffset - start
}

// driveWheel feeds the same wheel delta for n frames, then clears it.
func driveWheel(g *Game, h *wheelPanHarness, wx, wy float64, frames int) {
	for i := 0; i < frames; i++ {
		*h.wx = wx
		*h.wy = wy
		g.Update()
	}
	*h.wx = 0
	*h.wy = 0
	g.Update()
}

// TestCellGridHorizontalWheelMovesTimeline: a mostly-horizontal two-finger
// (trackpad) scroll over the cell grid moves the timeline view, NOT the rows.
func TestCellGridHorizontalWheelMovesTimeline(t *testing.T) {
	g, h := setupWheelGame(t)
	offBefore := g.drum.Offset
	rowBefore := g.drum.rowOffset

	driveWheel(g, h, 3, 0, 5) // pure horizontal

	if g.drum.Offset == offBefore {
		t.Fatalf("horizontal wheel over cell grid must move the timeline; Offset stayed %d", offBefore)
	}
	if g.drum.rowOffset != rowBefore {
		t.Fatalf("horizontal wheel must NOT scroll rows; rowOffset %d -> %d", rowBefore, g.drum.rowOffset)
	}
}

// TestCellGridVerticalWheelScrollsRows: a mostly-vertical scroll over the cell
// grid scrolls rows, NOT the timeline.
func TestCellGridVerticalWheelScrollsRows(t *testing.T) {
	g, h := setupWheelGame(t)
	offBefore := g.drum.Offset
	rowBefore := g.drum.rowOffset

	driveWheel(g, h, 0, -3, 5) // pure vertical

	if g.drum.rowOffset == rowBefore {
		t.Fatalf("vertical wheel over cell grid must scroll rows; rowOffset stayed %d", rowBefore)
	}
	if g.drum.Offset != offBefore {
		t.Fatalf("vertical wheel must NOT move the timeline; Offset %d -> %d", offBefore, g.drum.Offset)
	}
}

// TestCellGridDiagonalHorizontalDominantOnlyTimeline: a diagonal scroll whose
// horizontal component dominates moves ONLY the timeline — never both.
func TestCellGridDiagonalHorizontalDominantOnlyTimeline(t *testing.T) {
	g, h := setupWheelGame(t)
	offBefore := g.drum.Offset
	rowBefore := g.drum.rowOffset

	driveWheel(g, h, 4, 2, 5) // |dx| > |dy|

	if g.drum.Offset == offBefore {
		t.Fatalf("horizontal-dominant diagonal must move the timeline; Offset stayed %d", offBefore)
	}
	if g.drum.rowOffset != rowBefore {
		t.Fatalf("horizontal-dominant diagonal must NOT scroll rows (simultaneous action); rowOffset %d -> %d", rowBefore, g.drum.rowOffset)
	}
}

// TestCellGridDiagonalVerticalDominantOnlyRows: a diagonal scroll whose vertical
// component dominates scrolls ONLY the rows — never both.
func TestCellGridDiagonalVerticalDominantOnlyRows(t *testing.T) {
	g, h := setupWheelGame(t)
	offBefore := g.drum.Offset
	rowBefore := g.drum.rowOffset

	driveWheel(g, h, 2, -4, 5) // |dy| > |dx|

	if g.drum.rowOffset == rowBefore {
		t.Fatalf("vertical-dominant diagonal must scroll rows; rowOffset stayed %d", rowBefore)
	}
	if g.drum.Offset != offBefore {
		t.Fatalf("vertical-dominant diagonal must NOT move the timeline (simultaneous action); Offset %d -> %d", offBefore, g.drum.Offset)
	}
}

// TestVerticalWheelOverCellGridMatchesLabelScroll pins the no-drift contract:
// scrolling up/down over the drum cell grid must move the rows by the EXACT
// same amount as scrolling up/down directly over the instrument labels — the
// two reuse one scroll behavior (one row per notch, cooldown-throttled). Before
// the fix the cell-grid path used HandleWheel (steps rows at once, no cooldown)
// while the label path used WheelStep (one row, throttled), so identical input
// produced different row movement.
func TestVerticalWheelOverCellGridMatchesLabelScroll(t *testing.T) {
	g, h := setupWheelGame(t)
	// Grow the rack well past the visible window so the per-notch vs
	// scroll-by-steps difference is unambiguous (not masked by clamping).
	for i := 0; len(g.drum.Rows) < g.drum.visibleRows()+12 && i < 40; i++ {
		g.drum.AddRow()
		g.Update()
	}
	g.Update()
	if len(g.drum.Rows) <= g.drum.visibleRows()+6 {
		t.Skipf("rack not deep enough (rows=%d visible=%d)", len(g.drum.Rows), g.drum.visibleRows())
	}

	steps := g.drum.timelineZone.StepsRect()
	rr := g.drum.rowsRect()
	labelX, labelY := rr.Min.X+10, rr.Min.Y+g.drum.rowHeight()/2
	cellX, cellY := steps.Min.X+steps.Dx()/2, steps.Min.Y+20

	const notches = 8
	labelDelta := scrollWheelAt(g, h, labelX, labelY, notches)
	cellDelta := scrollWheelAt(g, h, cellX, cellY, notches)

	if labelDelta == 0 {
		t.Fatalf("label-region wheel did not scroll rows (test setup invalid); delta=0")
	}
	if cellDelta != labelDelta {
		t.Fatalf("up/down over the cell grid drifted from the instrument-label scroll: cell-grid moved %d rows, labels moved %d for the SAME %d-notch input — they must reuse one behavior",
			cellDelta, labelDelta, notches)
	}
}
