//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestLongSessionPauseResumeRetainsVisibleGrid reproduces the bug captured in
// screenshot.png: after long-playback the DrumView cell grid renders empty/grey
// because the predictor's sliding window has evicted the abs range the UI is
// trying to display. The render path treats predictor "no data" (idx <
// windowStart) and timeline "no commit" as the same signal — an off cell with
// NodeTypeInvisible — so every cell of every row goes grey.
//
// Invariant under test: for any drum.Offset the UI may scroll to (within the
// historical play range), refreshDrumRow must produce a row window whose cells
// reflect the row's beat pattern. The predictor must extend / re-anchor its
// retained window to cover the visible region; it must never silently report
// false for cells the UI is currently displaying.
//
// Reproduction shape (compressed from the 76-min production case):
//   - windowCap = 256 subdivisions (production default is 4096).
//   - 6 rows × buildSoakScene loop → predictor sees a non-trivial path per row.
//   - Drive Predictor.Ensure(i+1) for i = 1..2048 → 8 full window slides.
//   - Pause via SetPlaying(false), advance idle frames, resume via
//     SetPlaying(true) — mirrors the user-reported pause-then-resume flow.
//   - Force drum.Offset to a value strictly before windowStart and refresh.
//
// Before the predictor-visible-anchor fix this test fails: every row's Steps
// stays all-false and every CellTypes entry stays NodeTypeInvisible. After the
// fix the predictor re-extends to cover [drum.Offset, drum.Offset+Length) on
// the next Ensure call and cells populate.
func TestLongSessionPauseResumeRetainsVisibleGrid(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodesPerRow */)

	// Compressed retained window: cap 256 → 8 full slides over 2048 abs.
	g.engine.Predictor.SetWindowCap(256)
	g.StopBackgroundPredictorForTest()

	// Drive synthetic long-playback. Same harness pattern as
	// TestLongSessionHeapBoundedAtOneMillionAbs in long_session_canary_test.go.
	g.SetPlaying(true)
	const N = 2048
	for i := 1; i <= N; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		if i%64 == 0 {
			_ = g.Update()
		}
	}

	// Snapshot post-playback predictor state. windowStart must have advanced
	// past 0; otherwise the harness didn't trigger the slide path and the
	// repro is meaningless.
	winStart, winEnd, _ := g.engine.Predictor.WindowBoundsForTest()
	if winStart == 0 {
		t.Fatalf("predictor window did not slide (start=%d end=%d) — harness too short", winStart, winEnd)
	}
	if winEnd-winStart > 512 { // sanity: cap is 256, expect length ≈ 256
		t.Logf("predictor window=[%d,%d) length=%d (expected ~256)", winStart, winEnd, winEnd-winStart)
	}

	// Pause → idle → resume. This mirrors the screenshot user flow: pause then
	// re-play. The pause/resume itself is innocuous in the test harness (the
	// audio thread is stubbed) but exercises the Game.SetPlaying transition.
	g.SetPlaying(false)
	advanceFrames(g, 30)
	g.SetPlaying(true)
	advanceFrames(g, 4)

	// Force the visible window to an abs range that lies entirely below the
	// predictor's retained windowStart. This is the screenshot's failure
	// mode: the UI wants to display cells the predictor no longer has data
	// for, and the timeline archive only stores sparse Playback/Import
	// commits (gap-pads are dropped during migration), so the render path
	// has no source for "off but-rendered" cells → entire grid goes grey.
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

	// Each row's visible window must contain at least one non-Invisible cell
	// type. If every row reports CellTypes all NodeTypeInvisible the grid is
	// rendering as fully grey — that's the bug. The scene built by
	// buildSoakScene has Regular nodes at fixed J positions per row, so any
	// abs range covering a full loop period must include real beats.
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

	// Additional check: the predictor window must now cover the visible
	// window. The fix guarantees this; without it postStart > drum.Offset.
	if postStart > g.drum.Offset {
		t.Fatalf("predictor windowStart=%d > drum.Offset=%d — visible window is evicted (cells render grey)",
			postStart, g.drum.Offset)
	}
}
