package ui

import (
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// Regression: repeated play/pause/play cycles on a multi-row circuit with logic
// nodes (the stable test fixture, never the shipped startup demo) must not
// trigger parity panics.
func TestCircuitPauseResumeParityStable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch.Store(parityWatchPanic)
	g.Layout(1280, 720)

	if err := g.Import(assets_pkg.TestFixtureDemoJSON); err != nil {
		t.Fatalf("import test fixture: %v", err)
	}
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

	// Start
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
