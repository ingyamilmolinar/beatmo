package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Fix A contract: a path change during playback must bump parityGen, so the
// scan's parityGen filter (Fix B) excludes any decision/audio recorded under
// the old paths. Before this fix updateBeatInfos bumped only audioGen and
// hand-cleared the buffers with seqMu released — a window in which the
// background sequencer could record a decision under old paths that the
// post-SetPaths predictor would then contradict, panicking the app on a live
// graph edit.
func TestUpdateBeatInfosBumpsParityGenOnLiveEdit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Enter playback so the pathsChanged invalidation path runs.
	g.SetPlaying(true)
	g.nextBeatIdxs = []int{1}
	g.seqNextIdxs = []int{1}
	g.elapsedBeats = 0

	genBefore := g.parityGen.Load()

	// Change the path topology: reroute a->b->a into a->b->c->a.
	g.deleteEdge(b, a)
	g.addEdge(b, c)
	g.addEdge(c, a)
	g.updateBeatInfos()

	genAfter := g.parityGen.Load()
	if genAfter <= genBefore {
		t.Fatalf("expected parityGen to advance on live path edit during playback: before=%d after=%d", genBefore, genAfter)
	}
}
