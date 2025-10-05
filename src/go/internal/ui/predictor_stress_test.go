package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// buildRectangleWithLogic creates a rectangular loop with invisible pass-throughs
// and mixed logic rules across audible nodes.
func buildRectangleWithLogic(g *Game) *uiNode {
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	inv1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	B := g.tryAddNode(2, 0, model.NodeTypeRegular)
	C := g.tryAddNode(3, 0, model.NodeTypeRegular)
	D := g.tryAddNode(3, 1, model.NodeTypeRegular)
	E := g.tryAddNode(2, 1, model.NodeTypeRegular)
	inv2 := g.tryAddNode(1, 1, model.NodeTypeInvisible)
	F := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(A, inv1)
	g.addEdge(inv1, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, E)
	g.addEdge(E, inv2)
	g.addEdge(inv2, F)
	g.addEdge(F, A)
	// Mix in logic rules
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(A.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(B.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(B.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(C.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.5
		g.graph.SetNodeParams(C.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(D.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(D.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(E.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(E.ID, p)
	}
	return A
}

// TestSeqBeatSchedulesUnderHeavyPrediction_Legacy verifies seqScheduleBeat keeps
// scheduling in the presence of heavy prediction work (legacy path), by gating
// from DrumView Steps exclusively.
func TestSeqBeatSchedulesUnderHeavyPrediction_Legacy(t *testing.T) {
	g := New(testLogger)
	g.Layout(1024, 768)
	start := buildRectangleWithLogic(g)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Length = 128
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	want := append([]bool(nil), g.drum.Rows[0].Steps...)
	got := make([]bool, len(want))
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {})

	// Hammer computePredictions in a goroutine to simulate heavy work.
	done := make(chan struct{}, 1)
	go func() {
		g.computePredictions(len(want) * 32)
		done <- struct{}{}
	}()

	// Drive scheduler from DrumView Steps (seqScheduleBeat) repeatedly.
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
	for i := 0; i < len(want); i++ {
		g.seqScheduleBeat()
		if i < len(got) {
			got[i] = (i < len(g.drum.Rows[0].Steps) && g.drum.Rows[0].Steps[i])
		}
	}
	<-done
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mismatch at %d: got=%v want=%v", i, got[i], want[i])
		}
	}
}
