package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Fire only if previous connected node fired.
func TestNodeLogicTriggerIfPrevTriggered(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	// Make A skip every 2nd trigger so B can observe both prev-triggered states.
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()

	g.engine.Predictor.Ensure(4)
	if got := g.engine.Predictor.AudibleAt(0, 0); !got {
		t.Fatalf("expected a audible at abs=0")
	}
	if got := g.engine.Predictor.AudibleAt(0, 1); !got {
		t.Fatalf("expected b audible at abs=1 when prev triggered")
	}
	if got := g.engine.Predictor.AudibleAt(0, 2); got {
		t.Fatalf("expected a suppressed at abs=2 (skip_every_n=2)")
	}
	if got := g.engine.Predictor.AudibleAt(0, 3); got {
		t.Fatalf("expected b suppressed at abs=3 when prev skipped")
	}
}

// Every N loops gate.
func TestNodeLogicEveryNTriggers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()

	g.engine.Predictor.Ensure(3)
	if got := g.engine.Predictor.AudibleAt(0, 0); got {
		t.Fatalf("expected a suppressed at abs=0 (every_n_triggers=2)")
	}
	if got := g.engine.Predictor.AudibleAt(0, 2); !got {
		t.Fatalf("expected a audible at abs=2 (every_n_triggers=2)")
	}
}

// Probability 0 and 1 extremes.
func TestNodeLogicProbabilityExtremes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()
	g.engine.Predictor.Ensure(1)
	if got := g.engine.Predictor.AudibleAt(0, 0); got {
		t.Fatalf("expected suppression at p=0")
	}
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicP = 1
		g.graph.SetNodeParams(a.ID, p)
	}
	g.engine.Predictor.Ensure(1)
	if got := g.engine.Predictor.AudibleAt(0, 0); !got {
		t.Fatalf("expected fire at p=1")
	}
}

// Skip every N via logic kind
func TestNodeLogicSkipEveryNLogic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()

	g.engine.Predictor.Ensure(3)
	if got := g.engine.Predictor.AudibleAt(0, 0); !got {
		t.Fatalf("expected a audible at abs=0 (skip_every_n=2)")
	}
	if got := g.engine.Predictor.AudibleAt(0, 2); got {
		t.Fatalf("expected a suppressed at abs=2 (skip_every_n=2)")
	}
}

// Previous-trigger dependent gating
func TestNodeLogicPrevTriggerGates(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	// Make A skip every 2nd trigger so B can observe both prev-triggered states.
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	// B triggers only if previous skipped.
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()

	g.engine.Predictor.Ensure(4)
	// B at abs=1 follows A at abs=0 (triggered) -> suppressed.
	if got := g.engine.Predictor.AudibleAt(0, 1); got {
		t.Fatalf("expected b suppressed at abs=1 when prev triggered")
	}
	// B at abs=3 follows A at abs=2 (skipped) -> triggers.
	if got := g.engine.Predictor.AudibleAt(0, 3); !got {
		t.Fatalf("expected b audible at abs=3 when prev skipped")
	}
}
