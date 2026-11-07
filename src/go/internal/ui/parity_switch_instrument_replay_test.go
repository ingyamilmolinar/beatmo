package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Mirrors the clav-funk template: one cyclic row whose START node fires at abs=0 and loops to itself, with two
// silent nodes in the cycle.
const clavFunkCircuitJSON = `{
  "schema_version": "0.0.1",
  "subdiv": 16,
  "bpm": 104,
  "instruments": [
    {"name": "Guitar", "id": "guitar-electric", "kind": "builtin", "volume": 0.85, "origin": 0, "color": "#EE00DDFF"}
  ],
  "nodes": [
    {"id": 0, "i": 0, "j": 0, "type": "regular", "outputs": [1], "duration": 0.18},
    {"id": 1, "i": 12, "j": 0, "type": "regular", "outputs": [2], "duration": 0.18},
    {"id": 2, "i": 16, "j": 0, "type": "regular", "outputs": [3], "pitch": 3, "duration": 0.18},
    {"id": 3, "i": 24, "j": 0, "type": "regular", "outputs": [4], "duration": 0.18},
    {"id": 4, "i": 32, "j": 0, "type": "silent", "outputs": [5]},
    {"id": 5, "i": 32, "j": 8, "type": "regular", "outputs": [6], "pitch": 5, "duration": 0.18},
    {"id": 6, "i": 32, "j": 12, "type": "regular", "outputs": [7], "duration": 0.18},
    {"id": 7, "i": 32, "j": 24, "type": "regular", "outputs": [8], "pitch": 3, "duration": 0.18},
    {"id": 8, "i": 32, "j": 32, "type": "regular", "outputs": [9], "duration": 0.18},
    {"id": 9, "i": 20, "j": 32, "type": "regular", "outputs": [10], "duration": 0.18},
    {"id": 10, "i": 16, "j": 32, "type": "regular", "outputs": [11], "pitch": 7, "duration": 0.18},
    {"id": 11, "i": 8, "j": 32, "type": "regular", "outputs": [12], "pitch": 5, "duration": 0.18},
    {"id": 12, "i": 0, "j": 32, "type": "silent", "outputs": [13]},
    {"id": 13, "i": 0, "j": 24, "type": "regular", "outputs": [14], "pitch": 3, "duration": 0.18},
    {"id": 14, "i": 0, "j": 20, "type": "regular", "outputs": [15], "duration": 0.18},
    {"id": 15, "i": 0, "j": 8, "type": "regular", "outputs": [0], "pitch": 7, "duration": 0.18}
  ]
}`

// The start node fires at abs=0 every loop, so after stop → instrument switch → play the slate there must agree
// with the scheduler, not read as empty committed past.
func TestSwitchInstrumentAfterStopReplayParity(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)

	if err := g.Import([]byte(clavFunkCircuitJSON)); err != nil {
		t.Fatalf("import clav-funk: %v", err)
	}
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.refreshDrumRow()
	g.ClearParityMismatches()

	// A small window lets a brief playback evict abs=0, as a long session crosses the real cap.
	g.engine.Predictor.SetWindowCap(64)

	// Sanity: the start node is a real hit at abs=0.
	startInfo := g.beatInfoAtRow(0, 0)
	if !g.parityExpected(0, 0, startInfo) {
		t.Fatalf("precondition: expected start-node hit at abs=0 (info=%+v)", startInfo)
	}

	// First playback: play through the whole loop several times.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during first play: %v", err)
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*40)
	// abs=0 must have left the window: the state that corrupted the replay.
	if g.engine.Predictor.WindowStart() <= 0 {
		t.Fatalf("precondition: expected predictor window to have advanced past abs=0, windowStart=%d",
			g.engine.Predictor.WindowStart())
	}

	// Stop.
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop: %v", err)
	}

	// While stopped, as the user did.
	g.drum.selRow = 0
	g.drum.SetInstrument(roster.Flute)
	if !g.drum.IsInstrumentAvailable(roster.Flute) {
		if err := audio.RegisterWAV(roster.Flute, "test://flute.wav"); err != nil {
			t.Fatalf("register flute: %v", err)
		}
		g.drum.refreshInstruments()
	}
	if err := g.Update(); err != nil {
		t.Fatalf("update after instrument switch: %v", err)
	}

	// Replay: pressing play must not desync the slate at abs=0.
	g.ClearParityMismatches()
	pressPlay(t, g.drum)

	var panicked any
	func() {
		defer func() { panicked = recover() }()
		if err := g.Update(); err != nil {
			t.Fatalf("update during replay: %v", err)
		}
		advancePlaybackByAbs(g, g.grid.MaxDiv()*3)
	}()
	if panicked != nil {
		t.Fatalf("replay after instrument switch panicked (parity mismatch): %v", panicked)
	}

	slate, inWindow, off, length, _ := g.parityViewState(0, 0)
	info := g.beatInfoAtRow(0, 0)
	expected := g.parityExpected(0, 0, info)
	if inWindow && expected != slate {
		t.Fatalf("slate desync at abs=0 after instrument switch+replay: slate=%v expected=%v (offset=%d length=%d)",
			slate, expected, off, length)
	}
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after switch+replay, got %d", got)
	}
}
