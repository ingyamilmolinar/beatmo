package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// clavFunkCircuitJSON mirrors internal/assets/templates/clav-funk.json: a single
// cyclic row whose START node (id 0 at i=0) fires at abs=0 and loops back to
// itself (node 15 -> node 0), with two silent nodes in the cycle. This is the
// exact template the user loaded before the crash.
const clavFunkCircuitJSON = `{
  "version": 1,
  "subdiv": 16,
  "bpm": 104,
  "instruments": [
    {"name": "Guitar", "id": "guitar-electric", "kind": "builtin", "volume": 0.85, "origin": 0, "color": "#EE00DDFF", "reverb_send": 0.12, "synth_params": {"pitch": 0}}
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

// TestSwitchInstrumentAfterStopReplayParity reproduces the user's crash:
// load a template, play it, stop, switch the row's instrument, then press
// play again. The start node fires at abs=0 every loop, so the rendered slate
// MUST agree with the scheduler/predictor there. The instrument switch marks
// the row path-changed and dirties paths; on replay the start-node hit at
// abs=0 was being masked as empty committed-past, tripping the
// scheduler-vs-DrumView parity watchdog (panic on desktop).
func TestSwitchInstrumentAfterStopReplayParity(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.Layout(1024, 720)

	if err := g.Import([]byte(clavFunkCircuitJSON)); err != nil {
		t.Fatalf("import clav-funk: %v", err)
	}
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.refreshDrumRow()
	g.ClearParityMismatches()

	// Force a small predictor window so a brief playback evicts abs=0 from the
	// predictor's sliding window (mirrors the real 4-minute session crossing the
	// 4096-subdivision cap).
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
	// Precondition: the long playback evicted abs=0 from the predictor's
	// sliding window — the exact state that used to corrupt the replay.
	if g.engine.Predictor.WindowStart() <= 0 {
		t.Fatalf("precondition: expected predictor window to have advanced past abs=0, windowStart=%d",
			g.engine.Predictor.WindowStart())
	}

	// Stop.
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop: %v", err)
	}

	// Switch row 0's instrument while stopped (guitar-electric -> flute), exactly as
	// the user did. flute is a builtin synth instrument.
	g.drum.selRow = 0
	g.drum.SetInstrument("flute")
	if !g.drum.IsInstrumentAvailable("flute") {
		if err := audio.RegisterWAV("flute", "test://flute.wav"); err != nil {
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

	// Root-cause assertion: the rendered slate at abs=0 must agree with the
	// scheduler/predictor truth for the always-firing start node.
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
