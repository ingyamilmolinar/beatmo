package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestSchedulerVsDrumRow_MixedEverySkipRules builds a rectangular loop with
// invisible pass-through nodes and mixes multiple every/skip rules across
// distinct audible nodes. It asserts that what the scheduler plays over a
// horizon matches the drum view row Steps computed by the UI.
func TestSchedulerVsDrumRow_MixedEverySkipRules(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	// Build rectangle:
	// A(0,0)-> inv(1,0)-> B(2,0)-> C(3,0)
	//  ^                             |
	//  |                             v
	// F(0,1)<- inv(1,1)<- E(2,1)<- D(3,1)
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
	// Mixed rules
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
	horizon := 48
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	// Prepare drum preview at offset 0
	g.drum.Offset = 0
	g.refreshDrumRow()
	want := append([]bool(nil), g.drum.Rows[0].Steps...)
	if len(want) != horizon {
		t.Fatalf("unexpected steps len %d", len(want))
	}

	// Capture scheduler playback over the same horizon
	got := make([]bool, horizon)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {
		// Index is the just-scheduled absolute subdivision index for row 0
		idx := 0
		if len(g.seqNextIdxs) > 0 {
			idx = g.seqNextIdxs[0] - 1
		}
		if idx >= 0 && idx < len(got) {
			got[idx] = true
		}
	})
	g.SetPlaying(true)
	for abs := 0; abs < horizon; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	for i := 0; i < horizon; i++ {
		if got[i] != want[i] {
			t.Fatalf("scheduler vs drum mismatch at %d: got=%v want=%v\nwant=%v\ngot =%v", i, got[i], want[i], want, got)
		}
	}
}
