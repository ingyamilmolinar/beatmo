//go:build test

package ui

import (
	"testing"
)

// TestLongSessionResumeBackfillBounded reproduces the user-reported bug:
// "after a long playback session, pausing and playing again hangs for a few
// seconds before audio resumes." The hang is O(session-length): on resume the
// transport path resets frozenUpToByRow to -1 (game_update.go), and the very
// next refreshDrumRow re-freezes the ENTIRE elapsed history [0, playhead] in a
// single UI frame via the safety-net back-fill loop — per row. Because the
// playhead grows unbounded with session length, that one frame stalls for
// seconds on a long session, blocking the UI/sequencer before audio restarts.
//
// The fix bounds the back-fill loop to the predictor's retained window (the
// same guard syncUIToTime already uses at game_sync_ui.go). Indices below
// windowStart are already immutable past AND evicted from the predictor, so
// re-freezing them is both pointless and the source of the O(session) stall.
//
// Invariant under test: the work done by the safety-net back-fill on the resume
// frame must be bounded by the predictor window (windowCap) + the visible row
// length, NOT by elapsedBeats. We drive a synthetic long session with a small
// windowCap so windowStart is far above 0, pause, then resume through the real
// transport flow and assert the resume frame's back-fill iteration count is
// bounded — regardless of how long the session played.
func TestLongSessionResumeBackfillBounded(t *testing.T) {
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
	const rows = 6
	buildSoakScene(t, g, rows, 8 /* nodesPerRow */)
	g.drum.SetFollow(true)

	// Compressed retained window so windowStart slides far past 0 while the
	// synthetic playhead climbs to N — exactly the long-session precondition.
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

	// Bound: a small multiple of the predictor window + visible length, per row.
	// The buggy path does ~= elapsedBeats * rows iterations (>> this bound); the
	// fixed path re-freezes only the retained window.
	bound := int64(rows) * int64(windowCap+g.drum.Length+64)
	t.Logf("resume back-fill iters=%d bound=%d (elapsedBeats=%d rows=%d windowCap=%d length=%d window=[%d,%d))",
		resumeIters, bound, g.elapsedBeats, rows, windowCap, g.drum.Length, winStart, winEnd)
	if resumeIters > bound {
		t.Fatalf("resume back-fill did O(session) work: %d iters > bound %d (elapsedBeats=%d). "+
			"Playing after a long pause re-freezes the entire history in one frame — the hang.",
			resumeIters, bound, g.elapsedBeats)
	}
}
