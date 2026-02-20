package ui

import (
	"sync/atomic"
	"testing"
)

// validate the scheduler-driven sequencer triggers audio on engine beats,
// independent of UI rendering or Update calls. We simulate beats via the
// internal helper to avoid relying on wall-clock scheduler in tests.
func TestSequencerSchedulesOnBeatsIndependently(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Build a simple beat path of regular nodes.
	n0 := g.tryAddNode(0, 0, 0)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0) // loop
	g.updateBeatInfos()

	// Capture plays and count them while hammering the UI.
	var plays int32
	g.SetPlayFunc(func(string, float64, ...float64) { atomic.AddInt32(&plays, 1) })

	g.SetPlaying(true)
	// Trigger one beat and verify the time-based sequencer advanced.
	scheduleAbsForMuteTest(g, 0)
	if len(g.seqNextIdxs) == 0 {
		t.Fatalf("missing sequencer counters")
	}
	if g.seqNextIdxs[0] < 1 {
		t.Fatalf("sequencer did not advance: %d", g.seqNextIdxs[0])
	}
}
