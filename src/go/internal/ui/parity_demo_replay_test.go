package ui

import "testing"

// Regression: starting/pausing/restarting the demo circuit repeatedly should
// not trigger parity panics. This mirrors the manual repro flow reported by
// the user with the bundled demo.
func TestDemoReplayParityStable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.Layout(1280, 720)

	// Build the demo circuit.
	g.buildDemo()
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.ClearParityMismatches()

	playPause := func(steps int) {
		pressPlay(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update while playing: %v", err)
		}
		advancePlaybackByAbs(g, steps)
		pressStop(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update on stop: %v", err)
		}
	}

	// Run a few play/stop cycles to stress parity.
	for i := 0; i < 3; i++ {
		playPause(g.grid.MaxDiv() * 2)
	}

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after demo replay cycles, got %d", got)
	}
}
