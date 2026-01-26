package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestSchedulerVsDrumRow_MultiRowMixedRules verifies both rows stay in lockstep
// with the scheduler across a horizon for different circuits and mixed rules.
func TestSchedulerVsDrumRow_MultiRowMixedRules(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)
	// Row 0 rectangle (same as previous test but tweak rules a bit)
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
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(C.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(D.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(D.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(E.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 4
		g.graph.SetNodeParams(E.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(F.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 4
		g.graph.SetNodeParams(F.ID, p)
	}

	g.start = A
	g.graph.StartNodeID = A.ID

	// Row 1 triangle loop
	G := g.tryAddNode(6, 0, model.NodeTypeRegular)
	H := g.tryAddNode(7, 0, model.NodeTypeRegular)
	I := g.tryAddNode(7, 1, model.NodeTypeRegular)
	g.addEdge(G, H)
	g.addEdge(H, I)
	g.addEdge(I, G)
	if n, ok := g.graph.GetNodeByID(G.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(G.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(H.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(H.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(I.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(I.ID, p)
	}

	// Add row 1 and point origin to G
	g.drum.AddRow()
	if len(g.drum.rowLabels) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	g.drum.rowLabels[1].OnClick()
	g.drum.SetInstrument("row2") // distinct id for capture
	g.drum.Rows[1].Origin = G.ID
	g.drum.Rows[1].Node = g.nodeByID(G.ID)

	horizon := 60
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	want0 := append([]bool(nil), g.drum.Rows[0].Steps...)
	want1 := append([]bool(nil), g.drum.Rows[1].Steps...)

	got0 := make([]bool, horizon)
	got1 := make([]bool, horizon)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {
		if id == g.drum.Rows[0].Instrument {
			if len(g.seqNextIdxs) < 1 {
				return
			}
			idx := g.seqNextIdxs[0] - 1
			if idx >= 0 && idx < len(got0) {
				got0[idx] = true
			}
		} else if id == g.drum.Rows[1].Instrument {
			if len(g.seqNextIdxs) < 2 {
				return
			}
			idx := g.seqNextIdxs[1] - 1
			if idx >= 0 && idx < len(got1) {
				got1[idx] = true
			}
		}
	})
	g.SetPlaying(true)
	for abs := 0; abs < horizon; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	for i := 0; i < horizon; i++ {
		if got0[i] != want0[i] {
			t.Fatalf("row0 mismatch at %d: got=%v want=%v", i, got0[i], want0[i])
		}
		if got1[i] != want1[i] {
			t.Fatalf("row1 mismatch at %d: got=%v want=%v", i, got1[i], want1[i])
		}
	}
}
