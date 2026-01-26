package ui

import "testing"

// Regression: repeated play/pause/play cycles on the demo circuit must not
// trigger parity panics.
func TestDemoPauseResumeParityStable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.Layout(1280, 720)

	g.buildDemo()
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.ClearParityMismatches()

	playFor := func(steps int) {
		pressPlay(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update while starting: %v", err)
		}
		advancePlaybackByAbs(g, steps)
	}

	// Start demo
	playFor(g.grid.MaxDiv() * 2)

	// Pause
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on pause: %v", err)
	}

	// Resume and run longer to mimic the reported crash timing.
	playFor(g.grid.MaxDiv() * 4)

	// Pause again
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on pause 2: %v", err)
	}

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after pause/resume cycles, got %d", got)
	}
}
