//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Pause, a view-offset drag, then Play, through the real transport flow: every visible cell must render its
// played snapshot if played, else the predictor unless an immutable Playback/Import commit exists. Without
// follow the dragged Offset persists across the resume, so the predictor's anchors carry the load.
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
		// Without follow there is no snap-back to re-centre visibleMinAbs.
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
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodesPerRow */)
	// Follow stays on during the long prefix so windowStart slides with the playhead; a nofollow case flips it at
	// pause, before the drag.
	g.drum.SetFollow(true)

	// A compressed window still slides several times, so windowStart is well past 0 at pause.
	const windowCap = 256
	g.engine.Predictor.SetWindowCap(windowCap)
	g.StopBackgroundPredictorForTest()

	// Through Game.Update, so the later pause/resume reaches the production state machine.
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
	// Before the drag, or TrackBeat snaps Offset back on the first resume frame.
	g.drum.SetFollow(followPlayback)
	advanceFrames(g, 4)

	// What OnOffsetChange does for a grid drag or two-finger pan.
	dragged := naturalOffset + dragDelta
	if dragged < 0 {
		dragged = 0
	}
	g.drum.Offset = dragged
	g.drum.offsetChanged = true
	advanceFrames(g, 4)
	t.Logf("post-drag refresh: drum.Offset=%d (delta=%+d)", g.drum.Offset, dragDelta)

	// Asserted right after the resume Update, on the state it PUBLISHED, before a later refresh could self-heal.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on resume: %v", err)
	}
	if !g.Playing() || g.Paused() {
		t.Fatalf("expected Playing after resume (Playing=%v Paused=%v)", g.Playing(), g.Paused())
	}
	dumpRowDiagnostics(t, g)
	assertCellsMatchPredictor(t, g, 0)

	for step := 1; step <= 8; step++ {
		setPlayStartForAbs(g, pausedAbs+step)
		_ = g.Update()
		assertCellsMatchPredictor(t, g, step)
	}
}

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

// A drag that changes Offset without warming the preview leaves past cells with no commit and no preserved
// steps, which renders them blank although the predictor still has the data.
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
			// A played cell renders exactly its snapshot, never the live predictor, which no longer holds the played past
			// after a pause.
			if pv, pt, ok := g.timelineService().Played(rowIdx, abs); ok && abs < g.globalPastExclusive() {
				if r.Steps[i] != pv || r.CellTypes[i] != pt {
					fail.mismatch++
				}
				continue
			}
			// Any other divergence from the predictor means the preview dropped the row.
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
