package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Regression: when a live edit changes beat paths during playback, we may bump
// audioGen and clear parity buffers. If the sequencer counters were ahead of
// the UI playhead boundary (nextBeatIdxs), leaving them "future" can cause
// reschedules to skip the true next beat for the row (and parity can miss the
// resulting mismatch because history/past masking remains authoritative).
//
// Ensure updateBeatInfos clamps seqNextIdxs back to the next-beat boundary on
// path changes while playing.
func TestUpdateBeatInfos_PathChangeClampsSequencerAheadOfView(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlaying(true)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Drive playback to abs=4 so elapsedBeats=4 and seqNextIdxs=5.
	scheduleAbsForMuteTest(g, 4)
	if len(g.seqNextIdxs) == 0 {
		t.Fatalf("seqNextIdxs empty after scheduling")
	}
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs empty after scheduling")
	}
	beforeNext := g.nextBeatIdxs[0]
	if beforeNext <= 0 {
		t.Fatalf("unexpected nextBeatIdxs=%d", beforeNext)
	}

	// Simulate a transient race where the sequencer got ahead of the UI's
	// next-beat boundary (e.g., highlights haven't applied yet).
	g.seqMu.Lock()
	g.seqNextIdxs[0] = beforeNext + 5
	g.seqMu.Unlock()

	// Delete a node to force a path change while playback runs.
	g.deleteNode(b)

	if len(g.seqNextIdxs) == 0 {
		t.Fatalf("seqNextIdxs empty after path change")
	}
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs empty after path change")
	}
	if got := g.seqNextIdxs[0]; got > g.nextBeatIdxs[0] {
		t.Fatalf("expected seqNextIdxs clamped to nextBeatIdxs after path change: seqNext=%d nextBeat=%d", got, g.nextBeatIdxs[0])
	}
}
