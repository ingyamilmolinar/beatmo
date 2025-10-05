package ui

import "testing"

// Ensure pulses and highlights from a paused state don't linger after resume.
func TestResumeClearsOrphanPulses(t *testing.T) {
	SetDefaultStartForTest(true)
	g := New(testLogger)
	defer SetDefaultStartForTest(false)
	g.Layout(640, 480)

	// Start playback
	g.drum.playPressed = true
	_ = g.Update()
	if len(g.activePulses) == 0 {
		t.Fatalf("no pulses after start")
	}
	// Pause playback
	g.drum.playPressed = true
	_ = g.Update()
	if !g.paused {
		t.Fatalf("expected paused state")
	}
	// Resume playback; orphan pulses should be cleared and respawned
	g.drum.playPressed = true
	_ = g.Update()
	if !g.playing {
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
