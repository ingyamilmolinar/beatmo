//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// TestLongSessionPauseDragResumeKeepsCellsPopulated exercises the user-reported
// bug captured in screenshot.png: after a long playback session the user
// presses Pause, drags the global-timeline ribbon a little, then presses Play
// again — at which point part of the DrumView cell grid goes blank (bars
// missing on the left of every row while the predictor still has data for
// the live playhead).
//
// The reproduction reuses the compressed-window technique from
// TestLongSessionPauseResumeRetainsVisibleGrid (windowCap=256, synthetic abs
// drive) but routes the pause/resume through the full Game.Update →
// PlayPressed transport flow instead of toggling Playing() directly. The
// pause → drag → resume cycle in the user's report sequences three observable
// state changes that the existing scroll-back test doesn't exercise together:
//
//   1. Pause goes through handlePlaybackTransition (engine.Stop, state.Pause).
//   2. While paused, drum.Offset is rewritten by the timeline-zone scrub
//      callback (see drumview_ctor.go's OnScrubPosition / OnOffsetChange).
//      refreshDrumRow runs at the dragged offset.
//   3. Resume goes through handlePlaybackTransition again (engine.Start,
//      state.Resume). On the very next frame DrumView.TrackBeat may snap
//      Offset back to follow the playhead, and refreshDrumRow must paint a
//      complete row window covering [Offset, Offset+Length).
//
// Invariant: for every abs i in [drum.Offset, drum.Offset+Length), Steps[i]
// must equal predictor.VisibleAt(rowIdx, i) UNLESS the timeline holds an
// immutable Playback/Import commit at i. Anything else is the screenshot
// symptom — preview.BuildRowWindow silently dropping cells.
//
// Coverage matrix:
//   - FollowPlayback=true with small backward / small forward / large
//     backward (past windowStart) / large forward (past windowEnd) drag.
//   - FollowPlayback=false with backward-past-windowStart / forward-far-ahead.
//     This is the harder case because TrackBeat no longer snaps Offset back,
//     so the dragged Offset persists across the resume frame and the
//     predictor's slide-clamp / visibleMinAbs anchor logic carries the load.
//
// NOTE: As of the commit that introduced this test, all sub-cases PASS. The
// invariant holds in the synthetic Go harness, which suggests the user's
// screenshot symptom lives in code paths this harness does not reach:
// WASM/browser timing, the row-sprite cache rendering layer, or the
// real-sequencer commit path. This test is therefore primarily a regression
// guard for the predictor / preview.BuildRowWindow contract; a future fix
// for the user-reported bug should make at least one of these sub-cases
// fail before it passes again.
func TestLongSessionPauseDragResumeKeepsCellsPopulated(t *testing.T) {
	cases := []struct {
		name      string
		dragDelta int  // added to the natural Offset (pausedBeats - frac*Length)
		follow    bool // FollowPlayback: when false, Offset stays at dragged value across resume
	}{
		// FollowPlayback=true (default): TrackBeat snaps Offset back to follow the playhead.
		{name: "follow_backward_small", dragDelta: -32, follow: true},
		{name: "follow_forward_small", dragDelta: 32, follow: true},
		{name: "follow_backward_large_past_window_start", dragDelta: -512, follow: true},
		{name: "follow_forward_large_past_window_end", dragDelta: 512, follow: true},
		// FollowPlayback=false: Offset stays at the dragged value during playback.
		// This exercises the slide-clamp path more thoroughly because the
		// predictor cannot rely on the snap-back to re-center visibleMinAbs.
		{name: "nofollow_backward_past_window_start", dragDelta: -512, follow: false},
		{name: "nofollow_forward_far_ahead", dragDelta: 128, follow: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runPauseDragResumeRepro(t, tc.dragDelta, tc.follow)
		})
	}
}

func runPauseDragResumeRepro(t *testing.T, dragDelta int, followPlayback bool) {
	t.Helper()
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
	// FollowPlayback stays enabled during the long-playback prefix so the
	// predictor's windowStart slides forward with the playhead — that's
	// the actual repro precondition (windowCap plateau reached). It is
	// flipped (if the case wants nofollow) at pause time, just before
	// the drag, so the dragged Offset persists across the resume.
	g.drum.SetFollow(true)

	// Compressed retained window — 256 subdivisions vs production 4096.
	// At div=8 / N=1500 this still exercises ~5 full window slides so
	// windowStart is well past 0 by the time we pause.
	const windowCap = 256
	g.engine.Predictor.SetWindowCap(windowCap)
	g.StopBackgroundPredictorForTest()

	// Drive synthetic long playback through the proper Game.Update flow so
	// PlayPressed → handlePlaybackTransition → state.Resume reaches the
	// production state machine when we later pause/resume.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update entering play: %v", err)
	}
	const N = 1500
	for i := 1; i <= N; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		if i%32 == 0 {
			_ = g.Update()
		}
	}
	// Final Update at abs=N so elapsedBeats catches up to the predictor.
	setPlayStartForAbs(g, N)
	_ = g.Update()

	winStart, winEnd, _ := g.engine.Predictor.WindowBoundsForTest()
	if winStart == 0 {
		t.Fatalf("predictor window did not slide (start=%d end=%d) — harness too short", winStart, winEnd)
	}

	naturalOffset := g.drum.Offset
	pausedAbs := g.elapsedBeats
	t.Logf("post-playback: elapsedBeats=%d drum.Offset=%d Length=%d window=[%d,%d) cap=%d",
		pausedAbs, naturalOffset, g.drum.Length, winStart, winEnd, windowCap)

	// 1) Pause via the proper transport flow.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on pause: %v", err)
	}
	if !g.Paused() {
		t.Fatalf("expected Paused after pressPlay during playback (Playing=%v Paused=%v)", g.Playing(), g.Paused())
	}
	// Optionally disable auto-scroll BEFORE the drag so the dragged
	// drum.Offset persists across resume (without this TrackBeat snaps
	// Offset back to follow the playhead on the first resume frame).
	g.drum.SetFollow(followPlayback)
	advanceFrames(g, 4)

	// 2) Simulate a timeline ribbon scrub — this is exactly what the
	//    OnScrubPosition / OnOffsetChange callbacks do in drumview_ctor.go
	//    (dv.Offset = newOffset; dv.offsetChanged = true).
	dragged := naturalOffset + dragDelta
	if dragged < 0 {
		dragged = 0
	}
	g.drum.Offset = dragged
	g.drum.offsetChanged = true
	advanceFrames(g, 4)
	t.Logf("post-drag refresh: drum.Offset=%d (delta=%+d)", g.drum.Offset, dragDelta)

	// 3) Resume via the proper transport flow. The resume frame runs:
	//    PlayPressed → state.Resume → handlePlaybackTransition (engine.Start)
	//    → updateDrumTracking (TrackBeat may snap Offset) → refreshDrumRow.
	//    The bug — if present — surfaces on the row state PUBLISHED by this
	//    very Update call, before any subsequent refresh can self-heal. So
	//    we assert RIGHT after this Update returns.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on resume: %v", err)
	}
	if !g.Playing() || g.Paused() {
		t.Fatalf("expected Playing after resume (Playing=%v Paused=%v)", g.Playing(), g.Paused())
	}
	dumpRowDiagnostics(t, g)
	assertCellsMatchPredictor(t, g, 0)

	// Continue synthetic playback for a few more frames; the bug should
	// either still be present (predictor genuinely lost the visible
	// region) or have self-healed (preview pipeline catching up).
	for step := 1; step <= 8; step++ {
		setPlayStartForAbs(g, pausedAbs+step)
		_ = g.Update()
		assertCellsMatchPredictor(t, g, step)
	}
}

// dumpRowDiagnostics prints per-row Steps vs predictor vs timeline-commit
// state for the current visible window. Used to debug why the bug doesn't
// reproduce in the synthetic harness.
func dumpRowDiagnostics(t *testing.T, g *Game) {
	t.Helper()
	winStart, winEnd, cap := g.engine.Predictor.WindowBoundsForTest()
	t.Logf("DIAG: predictor window=[%d,%d) cap=%d drum.Offset=%d Length=%d elapsedBeats=%d lastStepsOffset=%d",
		winStart, winEnd, cap, g.drum.Offset, g.drum.Length, g.elapsedBeats, g.lastStepsOffset)
	for rowIdx, r := range g.drum.Rows {
		steps := 0
		predTrue := 0
		commits := 0
		releasedCommits := 0
		for i := 0; i < len(r.Steps); i++ {
			abs := g.drum.Offset + i
			if r.Steps[i] {
				steps++
			}
			if g.engine.Predictor.VisibleAt(rowIdx, abs) {
				predTrue++
			}
			if _, _, kind, ok := g.timelineCommittedWithKind(rowIdx, abs); ok {
				if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport {
					commits++
				} else if kind == timeline.CommitKindReleased {
					releasedCommits++
				}
			}
		}
		t.Logf("  row=%d Steps_true=%d/%d predictor_true=%d immutable_commits=%d released_commits=%d",
			rowIdx, steps, len(r.Steps), predTrue, commits, releasedCommits)
	}
}

// assertCellsMatchPredictor is the core invariant: after refresh, every row's
// Steps slice (what the user sees as bars) must reflect either an immutable
// timeline commit (Playback/Import) or — if no such commit exists for that
// abs — the predictor's VisibleAt for the same abs.
//
// The bug manifests when a drag changes drum.Offset without warming the
// preview pipeline: past cells in the new window have no immutable commit
// AND prev-steps preservation fails (canPreserveSteps requires
// PrevStepsOffset==Offset), so finalStep collapses to false and the user
// sees "blank cells" even though the predictor still has the right data.
func assertCellsMatchPredictor(t *testing.T, g *Game, frame int) {
	t.Helper()
	postStart, postEnd, _ := g.engine.Predictor.WindowBoundsForTest()

	totalExpectedTrue := 0
	type rowFail struct {
		row      int
		visTrue  int // predictor says true at this abs (in window)
		stepTrue int // refresh published Steps[i]=true
		typeNon  int // CellTypes[i] != Invisible
		mismatch int // Steps[i] != predictor.VisibleAt(abs) AND no immutable commit overrides
	}
	var failures []rowFail
	for rowIdx, r := range g.drum.Rows {
		fail := rowFail{row: rowIdx}
		for i := 0; i < len(r.Steps) && i < len(r.CellTypes); i++ {
			abs := g.drum.Offset + i
			predVis := g.engine.Predictor.VisibleAt(rowIdx, abs)
			if predVis {
				fail.visTrue++
				totalExpectedTrue++
			}
			if r.Steps[i] {
				fail.stepTrue++
			}
			if r.CellTypes[i] != model.NodeTypeInvisible {
				fail.typeNon++
			}
			// Only an immutable past commit is allowed to diverge from
			// the predictor (Playback/Import). For every other abs in
			// the visible window, Steps must equal predictor.VisibleAt
			// — a divergence means the preview pipeline silently
			// dropped the row.
			if r.Steps[i] != predVis {
				_, _, kind, ok := g.timelineCommittedWithKind(rowIdx, abs)
				immutable := ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport)
				if !immutable {
					fail.mismatch++
				}
			}
		}
		if (fail.visTrue > 0 && fail.stepTrue == 0) || fail.typeNon == 0 || fail.mismatch > 0 {
			failures = append(failures, fail)
		}
	}
	if len(failures) > 0 {
		t.Fatalf("frame %d: blank-row bug reproduced — drum.Offset=%d Length=%d elapsedBeats=%d predictor=[%d,%d) failures=%+v",
			frame, g.drum.Offset, g.drum.Length, g.elapsedBeats, postStart, postEnd, failures)
	}
	if totalExpectedTrue == 0 {
		t.Fatalf("frame %d: predictor reports zero VisibleAt across all rows in window [%d,%d) — predictor evicted visible region",
			frame, g.drum.Offset, g.drum.Offset+g.drum.Length)
	}
}
