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

// TestSeqScheduleTimeAdvancesUnderHeavyPrediction verifies the time-based
// sequencer keeps advancing even when predictor Ensure() is busy.
func TestSeqScheduleTimeAdvancesUnderHeavyPrediction(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 768)
	start := buildRectangleWithLogic(g)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.SetLength(128)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {})

	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}

	// Hammer Ensure() in a goroutine to simulate heavy work.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 32; i++ {
			g.engine.Predictor.Ensure(len(g.drum.Rows[0].Steps) * 32)
		}
		close(done)
	}()

	g.SetPlaying(true)
	steps := len(g.drum.Rows[0].Steps)
	for abs := 0; abs < steps; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	waitForChan(t, done, 10000)
	if len(g.seqNextIdxs) == 0 || g.seqNextIdxs[0] < steps {
		t.Fatalf("sequencer did not advance: got=%v want>=%d", g.seqNextIdxs, steps)
	}
}
