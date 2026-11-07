package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A sequencer left ahead of the UI boundary after a live path change would skip the row's true next beat, and
// parity could miss it; updateBeatInfos clamps seqNextIdxs back while playing.
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
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

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

	// A transient race: highlights not yet applied.
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
