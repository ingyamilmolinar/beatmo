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
	g := New(testLogger)
	g.Layout(1024, 768)
	nodes := buildComplexGraph(g)
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.updateBeatInfos()
	horizon := 512
	g.drum.Length = 64
	g.computePredictions(horizon)
	snap1 := append([]bool(nil), g.predVisibleByRow[0]...)
	// Recompute again; snapshot should remain identical
	g.computePredictions(horizon)
	snap2 := append([]bool(nil), g.predVisibleByRow[0]...)
	if len(snap1) != len(snap2) {
		t.Fatalf("snapshot lengths differ: %d vs %d", len(snap1), len(snap2))
	}
	for i := range snap1 {
		if snap1[i] != snap2[i] {
			t.Fatalf("prediction not deterministic at %d: %v vs %v", i, snap1[i], snap2[i])
		}
	}
}

func TestLiveEditClearsFutureCache(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)
	// Simple 2-node loop
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, A)
	// Start with A probability 0.25
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.25
		g.graph.SetNodeParams(A.ID, p)
	}
	g.start = A
	g.graph.StartNodeID = A.ID
	g.updateBeatInfos()
	horizon := 256
	g.computePredictions(horizon)
	before := append([]bool(nil), g.predAudibleByRow[0]...)

	// Simulate playback halfway and then edit A to probability 1.0
	cur := 128
	g.elapsedBeats = cur
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.RebaseAt(cur)
	}
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 1.0
		g.graph.SetNodeParams(A.ID, p)
	}
	// Recompute
	g.computePredictions(horizon)
	after := append([]bool(nil), g.predAudibleByRow[0]...)
	// Prefix before cur should be unchanged
	for i := 0; i < cur && i < len(before) && i < len(after); i++ {
		if before[i] != after[i] {
			t.Fatalf("edit affected past at %d: %v -> %v", i, before[i], after[i])
		}
	}
	// After cur, all occurrences of A should now be audible.
	for i := cur; i < len(after); i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == A.ID && !after[i] {
			t.Fatalf("expected A audible after edit at %d", i)
		}
	}
}
