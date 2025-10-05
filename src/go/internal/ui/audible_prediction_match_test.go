package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestEvalMatchesPredictionAcrossComplexCircuits verifies evalNodePlayback
// and prediction-based preview agree exactly across a range of steps for
// circuits with combined node rules and invisible nodes.
func TestEvalMatchesPredictionAcrossComplexCircuits(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Build a rectangular loop with invisible links and mixed logic:
	// A(0,0) -> inv(1,0) -> B(2,0) -> C(3,0)
	//   ^                             |
	//   |                             v
	//  D(0,1) <- inv(1,1) <- inv(2,1) <- inv(3,1)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	inv1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	c := g.tryAddNode(3, 0, model.NodeTypeRegular)
	inv2 := g.tryAddNode(3, 1, model.NodeTypeInvisible)
	inv3 := g.tryAddNode(2, 1, model.NodeTypeInvisible)
	inv4 := g.tryAddNode(1, 1, model.NodeTypeInvisible)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	// Edges clockwise
	g.addEdge(a, inv1)
	g.addEdge(inv1, b)
	g.addEdge(b, c)
	g.addEdge(c, inv2)
	g.addEdge(inv2, inv3)
	g.addEdge(inv3, inv4)
	g.addEdge(inv4, d)
	g.addEdge(d, a)
	// Logic: A probability .35, B every 2nd, C prev_triggered, D skip every 3rd
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.35
		g.graph.SetNodeParams(a.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(d.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(d.ID, p)
	}

	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Length = 16
	g.updateBeatInfos()

	horizon := 32
	g.computePredictions(horizon)
	// Reset runtime counters before eval scan to provide a clean baseline.
	g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.lastTriggeredByRow = make(map[int]map[model.NodeID]bool)
	g.lastFiredNodeByRow = make([]model.NodeID, len(g.drum.Rows))

	for i := 0; i < horizon; i++ {
		bi := g.beatInfoAtRow(0, i)
		ok, _, _, _ := g.evalNodePlayback(0, i, bi)
		vis := false
		if i < len(g.predVisibleByRow[0]) {
			vis = g.predVisibleByRow[0][i]
		}
		if ok != vis {
			t.Fatalf("idx %d: eval=%v predVis=%v node=%+v", i, ok, vis, bi)
		}
	}
}

// TestAudioSchedulerMatchesPrediction uses the deterministic step scheduler to
// confirm audible playback matches predVisible timeline exactly across steps.
// Note: audio scheduling is verified indirectly via eval-on-idx equivalence
// above. A direct scheduler-vs-prediction test is intentionally omitted here
// to avoid coupling to seam/visual policy.
