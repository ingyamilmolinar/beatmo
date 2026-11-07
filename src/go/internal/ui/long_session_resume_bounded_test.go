//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Resume resets frozenUpToByRow, and the next refresh's back-fill must stay inside the predictor's window:
// re-freezing the whole elapsed history per row stalls the frame for seconds on a long session. The work
// is bounded by windowCap + the row length, never by elapsedBeats.
func TestLongSessionResumeBackfillBounded(t *testing.T) {
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
	const rows = 6
	buildSoakScene(t, g, rows, 8 /* nodesPerRow */)
	g.drum.SetFollow(true)

	// Small enough that windowStart slides far past 0 while the playhead climbs.
	const windowCap = 256
	g.engine.Predictor.SetWindowCap(windowCap)
	g.StopBackgroundPredictorForTest()

	// Drive a long synthetic session through the real Game.Update transport flow.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update entering play: %v", err)
	}
	const N = 8192 // >> windowCap, so a full re-freeze is O(N) not O(window)
	for i := 1; i <= N; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		if i%64 == 0 {
			_ = g.Update()
		}
	}
	setPlayStartForAbs(g, N)
	_ = g.Update()

	winStart, winEnd, _ := g.engine.Predictor.WindowBoundsForTest()
	if winStart == 0 {
		t.Fatalf("predictor window did not slide (start=%d end=%d) — harness too short", winStart, winEnd)
	}
	if g.elapsedBeats < N/2 {
		t.Fatalf("elapsedBeats=%d did not advance near N=%d — harness broken", g.elapsedBeats, N)
	}

	// Pause via the real transport flow.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on pause: %v", err)
	}
	if !g.Paused() {
		t.Fatalf("expected Paused after pressPlay during playback")
	}
	advanceFrames(g, 2)

	// Resume — measure the safety-net back-fill work done by this single frame.
	before := g.FreezeBackfillIters()
	setPlayStartForAbs(g, N)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on resume: %v", err)
	}
	if !g.Playing() || g.Paused() {
		t.Fatalf("expected Playing after resume")
	}
	resumeIters := g.FreezeBackfillIters() - before

	// An unbounded back-fill does about elapsedBeats × rows iterations, far above this.
	bound := int64(rows) * int64(windowCap+g.drum.Length+64)
	t.Logf("resume back-fill iters=%d bound=%d (elapsedBeats=%d rows=%d windowCap=%d length=%d window=[%d,%d))",
		resumeIters, bound, g.elapsedBeats, rows, windowCap, g.drum.Length, winStart, winEnd)
	if resumeIters > bound {
		t.Fatalf("resume back-fill did O(session) work: %d iters > bound %d (elapsedBeats=%d). "+
			"Playing after a long pause re-freezes the entire history in one frame — the hang.",
			resumeIters, bound, g.elapsedBeats)
	}
}
