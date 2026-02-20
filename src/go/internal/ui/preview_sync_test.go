package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func pastExclusiveForTest(g *Game, row int) int {
	past := 0
	if row >= 0 && row < len(g.nextBeatIdxs) {
		past = g.nextBeatIdxs[row]
	}
	if row >= 0 && row < len(g.seqNextIdxs) && g.seqNextIdxs[row] > past {
		past = g.seqNextIdxs[row]
	}
	return past
}

func assertPreviewMatchesPredictor(t *testing.T, g *Game, row, base, n int) {
	t.Helper()
	g.drum.Offset = base
	g.refreshDrumRow()
	got := g.drum.Rows[row].Steps
	if n > len(got) {
		n = len(got)
	}
	g.engine.Predictor.Ensure(base + n)
	for i := 0; i < n; i++ {
		want := g.engine.Predictor.VisibleAt(row, base+i)
		if got[i] != want {
			t.Fatalf("mismatch at abs=%d (+%d base=%d): got %v want %v", base+i, i, base, got[i], want)
		}
	}
}

func driveSequencerToAbsForTest(t *testing.T, g *Game, abs int) {
	t.Helper()
	for i := 0; i < 128; i++ {
		scheduleAbsForMuteTest(g, abs)
		if len(g.seqNextIdxs) > 0 && g.seqNextIdxs[0] > abs {
			return
		}
	}
	t.Fatalf("sequencer did not reach abs %d (seqNextIdxs=%v)", abs, g.seqNextIdxs)
}

func TestPreviewSyncMatchesPlayback_EveryN(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.SetPlaying(true)
	driveSequencerToAbsForTest(t, g, 24)
	base := pastExclusiveForTest(g, 0)
	assertPreviewMatchesPredictor(t, g, 0, base, 6)
}

func TestPreviewSyncMatchesPlayback_PrevTriggered(t *testing.T) {
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
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.SetPlaying(true)
	driveSequencerToAbsForTest(t, g, 24)
	base := pastExclusiveForTest(g, 0)
	assertPreviewMatchesPredictor(t, g, 0, base, 6)
}

func TestPreviewSyncMatchesPlayback_ComplexLogic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Build loop A->B->C->D->A
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	// Rules:
	// A: every 3rd trigger
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(a.ID, p)
	}
	// B: skip every 2nd trigger
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	// C: trigger if previous triggered
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	// D: trigger if previous skipped
	if n, ok := g.graph.GetNodeByID(d.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(d.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.SetPlaying(true)
	driveSequencerToAbsForTest(t, g, 64)
	base := pastExclusiveForTest(g, 0)
	// Check three different preview alignments (all in the future window).
	for _, delta := range []int{0, 3, 8} {
		assertPreviewMatchesPredictor(t, g, 0, base+delta, 12)
	}
}
