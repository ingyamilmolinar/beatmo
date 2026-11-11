package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: if the UI playhead (nextBeatIdxs) lags the global playhead,
// late-but-ordered highlights should still advance the UI instead of being
// dropped by a past-boundary check that is too aggressive.
func TestApplySequencerHighlight_AllowsCatchUpWhenUIBehind(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(start, next)
	g.addEdge(next, start)

	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start

	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlaying(true)
	row := 0
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}

	// Simulate a lagging UI playhead with the sequencer/global clock ahead.
	g.nextBeatIdxs[row] = 4
	g.seqNextIdxs[row] = 8
	g.elapsedBeats = 10

	idx := g.nextBeatIdxs[row]
	info := g.beatInfoAtRow(row, idx)
	g.applySequencerHighlight(row, idx, info)

	if got := g.nextBeatIdxs[row]; got != idx+1 {
		t.Fatalf("expected nextBeatIdxs to advance from %d to %d, got %d (seqNext=%v elapsed=%d)", idx, idx+1, got, g.seqNextIdxs, g.elapsedBeats)
	}
}
