package ui

import "testing"

// Regression: stopping from a paused state must clear pulses/highlights even
// when the playing flag does not transition (paused -> stopped).
func TestStopFromPausedClearsPulsesAndHighlights(t *testing.T) {
	withDefaultStart(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Start playback (spawns pulses).
	pressPlay(t, g.drum)
	_ = g.Update()
	if !g.Playing() {
		t.Fatalf("expected playing after start")
	}
	if len(g.activePulses) == 0 {
		t.Fatalf("expected pulses after start")
	}

	// Pause (keeps pulses/highlights around).
	pressPlay(t, g.drum)
	_ = g.Update()
	if !g.Paused() {
		t.Fatalf("expected paused after pause toggle")
	}
	if len(g.activePulses) == 0 {
		t.Fatalf("expected pulses to remain while paused")
	}

	// Seed a highlight to ensure Stop clears highlight state deterministically.
	g.highlightedBeats[makeBeatKey(0, 0)] = 123

	// Stop from paused.
	pressStop(t, g.drum)
	_ = g.Update()
	if g.Playing() || g.Paused() {
		t.Fatalf("expected stopped state after stop")
	}
	if len(g.activePulses) != 0 || g.activePulse != nil {
		t.Fatalf("expected no active pulses after stop; pulses=%d active=%v", len(g.activePulses), g.activePulse)
	}
	if len(g.highlightedBeats) != 0 {
		t.Fatalf("expected highlights cleared after stop; got=%v", g.highlightedBeats)
	}
}
