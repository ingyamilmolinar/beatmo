//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// After long playback the predictor has evicted the range the UI shows; treating "no data" like "no commit"
// greys the whole grid, so the predictor must re-extend to cover [Offset, Offset+Length) on the next Ensure.
func TestLongSessionPauseResumeRetainsVisibleGrid(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodesPerRow */)

	// Compressed retained window: cap 256 → 8 full slides over 2048 abs.
	g.engine.Predictor.SetWindowCap(256)
	g.StopBackgroundPredictorForTest()

	g.SetPlaying(true)
	const N = 2048
	for i := 1; i <= N; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		if i%64 == 0 {
			_ = g.Update()
		}
	}

	// windowStart must have advanced, or the slide path never ran and the check is meaningless.
	winStart, winEnd, _ := g.engine.Predictor.WindowBoundsForTest()
	if winStart == 0 {
		t.Fatalf("predictor window did not slide (start=%d end=%d) — harness too short", winStart, winEnd)
	}
	if winEnd-winStart > 512 { // sanity: cap is 256, expect length ≈ 256
		t.Logf("predictor window=[%d,%d) length=%d (expected ~256)", winStart, winEnd, winEnd-winStart)
	}

	g.SetPlaying(false)
	advanceFrames(g, 30)
	g.SetPlaying(true)
	advanceFrames(g, 4)

	// Entirely below windowStart, where the archive keeps only sparse commits: the render path has no other source
	// for the cells.
	scrolledOffset := winStart - 128
	if scrolledOffset < 0 {
		scrolledOffset = 0
	}
	g.drum.Offset = scrolledOffset
	g.refreshDrumRow()

	// Re-read the window after refreshDrumRow because the fix re-anchors it.
	postStart, postEnd, _ := g.engine.Predictor.WindowBoundsForTest()
	t.Logf("predictor window after scroll-back refresh: [%d,%d) drum.Offset=%d Length=%d",
		postStart, postEnd, g.drum.Offset, g.drum.Length)

	// Any range covering a full loop includes real beats, so an all-Invisible row is the grey grid.
	failedRows := []int{}
	for rowIdx, r := range g.drum.Rows {
		nonInvisible := 0
		for _, ct := range r.CellTypes {
			if ct != model.NodeTypeInvisible {
				nonInvisible++
			}
		}
		if nonInvisible == 0 {
			failedRows = append(failedRows, rowIdx)
		}
	}
	if len(failedRows) == len(g.drum.Rows) {
		t.Fatalf("all %d rows rendered fully NodeTypeInvisible at drum.Offset=%d (predictor window=[%d,%d)) — bug reproduced",
			len(failedRows), g.drum.Offset, postStart, postEnd)
	}
	if len(failedRows) > 0 {
		t.Fatalf("rows %v rendered fully NodeTypeInvisible at drum.Offset=%d (predictor window=[%d,%d))",
			failedRows, g.drum.Offset, postStart, postEnd)
	}

	if postStart > g.drum.Offset {
		t.Fatalf("predictor windowStart=%d > drum.Offset=%d — visible window is evicted (cells render grey)",
			postStart, g.drum.Offset)
	}
}
