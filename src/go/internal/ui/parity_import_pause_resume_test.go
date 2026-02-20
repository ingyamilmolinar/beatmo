package ui

import "testing"

// Regression: import the real project, play, pause, resume, and keep playing
// without triggering parity panics.
func TestImportPauseResumeParityStable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.Layout(1024, 720)

	if err := g.Import([]byte(beatmoProjectJSON)); err != nil {
		t.Fatalf("import beatmo project: %v", err)
	}
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.refreshDrumRow()
	g.ClearParityMismatches()

	playFor := func(steps int) {
		pressPlay(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update while playing: %v", err)
		}
		advancePlaybackByAbs(g, steps)
	}

	// Start and play a bit.
	playFor(g.grid.MaxDiv() * 3)

	// Pause.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on pause: %v", err)
	}

	// Resume and play longer to surface potential mismatches.
	playFor(g.grid.MaxDiv() * 6)

	// Pause again to end the cycle.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on final pause: %v", err)
	}

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after import pause/resume cycle, got %d", got)
	}
}
