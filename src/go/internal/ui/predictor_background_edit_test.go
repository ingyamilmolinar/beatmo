package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// buildComplexGraph creates a loop with multiple probability nodes and mixed logic rules.
func buildComplexGraph(g *Game) []*uiNode {
	// Rectangle with 6 regular nodes and 2 invisible links for diagonals
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(1, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 0, model.NodeTypeRegular)
	D := g.tryAddNode(2, 1, model.NodeTypeRegular)
	E := g.tryAddNode(1, 1, model.NodeTypeRegular)
	F := g.tryAddNode(0, 1, model.NodeTypeRegular)
	// Edges in a ring
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, E)
	g.addEdge(E, F)
	g.addEdge(F, A)
	// Add some invisible intermediate nodes that the graph will expand
	// Note: UI handles invisible intermediates; include few true invisibles too
	inv := g.tryAddNode(1, 2, model.NodeTypeInvisible)
	_ = inv
	// Mixed logic
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.41
		g.graph.SetNodeParams(A.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(B.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(B.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(C.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 4
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
	if n, ok := g.graph.GetNodeByID(F.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.27
		g.graph.SetNodeParams(F.ID, p)
	}
	return []*uiNode{A, B, C, D, E, F}
}

func TestComplexPredictionDeterministicAndAhead(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 768)
	nodes := buildComplexGraph(g)
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.updateBeatInfos()
	horizon := 512
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}
	g.engine.Predictor.Ensure(horizon)
	snap1 := make([]bool, horizon)
	for i := 0; i < horizon; i++ {
		snap1[i] = g.engine.Predictor.VisibleAt(0, i)
	}
	// Force a full recompute via UpdateNode using the same params; snapshot should remain identical.
	if n, ok := g.graph.GetNodeByID(nodes[0].ID); ok {
		g.engine.Predictor.UpdateNode(nodes[0].ID, n)
	}
	g.engine.Predictor.Ensure(horizon)
	snap2 := make([]bool, horizon)
	for i := 0; i < horizon; i++ {
		snap2[i] = g.engine.Predictor.VisibleAt(0, i)
	}
	if len(snap1) != len(snap2) {
		t.Fatalf("snapshot lengths differ: %d vs %d", len(snap1), len(snap2))
	}
	for i := range snap1 {
		if snap1[i] != snap2[i] {
			t.Fatalf("prediction not deterministic at %d: %v vs %v", i, snap1[i], snap2[i])
		}
	}
}

func TestLiveEditRecomputesPredictions(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Simple 2-node loop
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, A)
	// Start with A probability 0.0 (always skipped).
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.0
		g.graph.SetNodeParams(A.ID, p)
	}
	g.start = A
	g.graph.StartNodeID = A.ID
	g.updateBeatInfos()
	horizon := 128
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}
	g.engine.Predictor.Ensure(horizon)
	for i := 0; i < horizon; i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == A.ID && g.engine.Predictor.AudibleAt(0, i) {
			t.Fatalf("expected A silent before edit at %d (probability=0)", i)
		}
	}

	// Edit A to probability 1.0 and ensure predictions are recomputed.
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 1.0
		g.graph.SetNodeParams(A.ID, p)
	}
	g.engine.Predictor.Ensure(horizon)
	for i := 0; i < horizon; i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == A.ID && !g.engine.Predictor.AudibleAt(0, i) {
			t.Fatalf("expected A audible after edit at %d (probability=1)", i)
		}
	}
}
