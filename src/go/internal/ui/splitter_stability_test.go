package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSplitterStability is the main regression test for the drum-view divider
// jitter bug. It enables forceAutoSize and production eqPanelHeight, then
// calls Layout() 10 times at each window size and asserts split.Y and
// drum.eqH never change frame-to-frame.
func TestSplitterStability(t *testing.T) {
	assertDefaultParityState(t)
	withForceAutoSize(t, true)
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	// Use production eqPanelHeight (180).
	prev := eqPanelHeight
	eqPanelHeight = 180
	t.Cleanup(func() { eqPanelHeight = prev })

	sizes := [][2]int{
		{800, 600},
		{1024, 768},
		{1920, 1080},
		{801, 601}, // odd dimensions to stress rounding
		{1023, 767},
		{640, 480},
	}

	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		logger := log.New(testLogOutput(), log.LevelInfo)
		g := New(logger)
		t.Cleanup(g.CloseForTest)

		// First call establishes the baseline.
		g.Layout(w, h)
		baseY := g.split.Y
		baseEqH := g.drum.eqH

		for frame := 1; frame <= 10; frame++ {
			g.Layout(w, h)
			if g.split.Y != baseY {
				t.Fatalf("size %dx%d: split.Y jittered on frame %d: base=%d got=%d",
					w, h, frame, baseY, g.split.Y)
			}
			if g.drum.eqH != baseEqH {
				t.Fatalf("size %dx%d: drum.eqH jittered on frame %d: base=%d got=%d",
					w, h, frame, baseEqH, g.drum.eqH)
			}
		}
	}
}

// TestWidgetBoardRowHeightsSumToTotal verifies that cumulative rounding in
// WidgetBoard.recalc() produces row heights that sum exactly to the total.
func TestWidgetBoardRowHeightsSumToTotal(t *testing.T) {
	weights := []float64{2, 3, 3}
	heights := []int{100, 101, 150, 199, 200, 201, 300, 301, 400, 479, 480, 481, 600}

	for _, h := range heights {
		wb := NewWidgetBoard(image.Rect(0, 0, 400, h), []float64{1}, weights)
		sum := 0
		for r := 0; r < len(weights); r++ {
			sum += wb.RowHeight(r)
		}
		if sum != h {
			t.Errorf("h=%d: sum of row heights=%d, want %d (rows: %d, %d, %d)",
				h, sum, h, wb.RowHeight(0), wb.RowHeight(1), wb.RowHeight(2))
		}
	}
}

// TestSplitterRatioRoundtrip verifies that after a user sets the splitter via
// drag (userSet=true), UpdateResize preserves the exact Y position when the
// window size hasn't changed.
func TestSplitterRatioRoundtrip(t *testing.T) {
	totalH := 800
	s := NewSplitter(totalH)
	s.userSet = true

	testYs := []int{300, 400, 401, 399, 500, 267, 533}
	for _, y := range testYs {
		s.Y = y
		if totalH > 0 {
			s.ratio = float64(y) / float64(totalH)
		}
		s.UpdateResize(totalH, 1024)
		if s.Y != y {
			t.Errorf("ratio round-trip failed: set Y=%d, after UpdateResize got Y=%d (ratio=%f)",
				y, s.Y, s.ratio)
		}
	}
}
