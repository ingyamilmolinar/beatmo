package ui

import "testing"

// Regression: after stop, stale parity audio/highlights must be cleared so
// replaying does not trigger highlight_vs_audio panics.
func TestImportStopReplayClearsParity(t *testing.T) {
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

	// Play briefly
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during first play: %v", err)
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*3)

	// Stop -> parity buffers cleared
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop: %v", err)
	}

	// Replay
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during replay: %v", err)
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*3)

	// Pause again; no panic should have occurred. Parity ring should be empty.
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after stop/replay, got %d", got)
	}
}
