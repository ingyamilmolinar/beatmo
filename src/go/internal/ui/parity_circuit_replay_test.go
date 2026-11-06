package ui

import (
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// Regression: starting/stopping/restarting a multi-row circuit with logic nodes
// (the stable test fixture, never the shipped startup demo) repeatedly must not
// trigger parity panics. This mirrors the manual repro flow first reported on
// the bundled demo.
func TestCircuitReplayParityStable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.Layout(1280, 720)

	if err := g.Import(assets_pkg.TestFixtureDemoJSON); err != nil {
		t.Fatalf("import test fixture: %v", err)
	}
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
		t.Fatalf("expected no parity mismatches after replay cycles, got %d", got)
	}
}
