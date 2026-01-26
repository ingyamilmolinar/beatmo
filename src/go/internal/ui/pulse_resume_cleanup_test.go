package ui

import "testing"

// Ensure pulses and highlights from a paused state don't linger after resume.
func TestResumeClearsOrphanPulses(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Start playback
	pressPlay(t, g.drum)
	_ = g.Update()
	if len(g.activePulses) == 0 {
		t.Fatalf("no pulses after start")
	}
	// Pause playback
	pressPlay(t, g.drum)
	_ = g.Update()
	if !g.Paused() {
		t.Fatalf("expected paused state")
	}
	// Resume playback; orphan pulses should be cleared and respawned
	pressPlay(t, g.drum)
	_ = g.Update()
	if !g.Playing() {
		t.Fatalf("expected playing state after resume")
	}
	// Should have at most one pulse per row
	seen := map[int]int{}
	for _, p := range g.activePulses {
		seen[p.row]++
	}
	for row, c := range seen {
		if c > 1 {
			t.Fatalf("row %d has duplicate pulses: %d", row, c)
		}
	}
}
