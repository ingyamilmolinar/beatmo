package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Fire only if previous connected node fired.
func TestNodeLogicPrevFired(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(b.ID, p)
	}
	// Simulate that A just triggered previously
	g.lastTriggeredByRow = map[int]map[model.NodeID]bool{0: {a.ID: true}}
	ok, _, _, _ := g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: b.ID, NodeType: model.NodeTypeRegular})
	if !ok {
		t.Fatalf("expected B to fire when previous fired")
	}
	// Simulate previous did not trigger
	g.lastTriggeredByRow[0][a.ID] = false
	ok, _, _, _ = g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: b.ID, NodeType: model.NodeTypeRegular})
	if ok {
		t.Fatalf("expected B to be suppressed when previous did not fire")
	}
}

// Every N loops gate.
func TestNodeLogicEveryNTriggers(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	r1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if n, ok := g.graph.GetNodeByID(r1.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(r1.ID, p)
	}
	ok, _, _, _ := g.evalNodePlayback(0, 0, model.BeatInfo{NodeID: r1.ID, NodeType: model.NodeTypeRegular})
	if ok {
		t.Fatalf("expected first trigger to be suppressed for N=2 (fire every 2nd)")
	}
	ok, _, _, _ = g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: r1.ID, NodeType: model.NodeTypeRegular})
	if !ok {
		t.Fatalf("expected second trigger to fire for N=2")
	}
}

// Probability 0 and 1 extremes.
func TestNodeLogicProbabilityExtremes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0
		g.graph.SetNodeParams(a.ID, p)
	}
	ok, _, _, _ := g.evalNodePlayback(0, 0, model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular})
	if ok {
		t.Fatalf("expected suppression at p=0")
	}
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicP = 1
		g.graph.SetNodeParams(a.ID, p)
	}
	ok, _, _, _ = g.evalNodePlayback(0, 0, model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular})
	if !ok {
		t.Fatalf("expected fire at p=1")
	}
}

// Skip every N via logic kind
func TestNodeLogicSkipEveryN(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	r := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n, ok := g.graph.GetNodeByID(r.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(r.ID, p)
	}
	// 1st should fire
	ok, _, _, _ := g.evalNodePlayback(0, 0, model.BeatInfo{NodeID: r.ID, NodeType: model.NodeTypeRegular})
	if !ok {
		t.Fatalf("expected first to fire")
	}
	// 2nd should skip
	ok, _, _, _ = g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: r.ID, NodeType: model.NodeTypeRegular})
	if ok {
		t.Fatalf("expected second to skip")
	}
}

// Previous-trigger dependent gating
func TestNodeLogicPrevTriggerGates(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	// Seed last state: a triggered last time
	g.lastTriggeredByRow = map[int]map[model.NodeID]bool{0: {a.ID: true}}
	// b triggers only if prev skipped
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(b.ID, p)
	}
	ok, _, _, _ := g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: b.ID, NodeType: model.NodeTypeRegular})
	if ok {
		t.Fatalf("expected b suppressed when prev triggered")
	}
	// Now mark prev skipped and expect b to trigger
	g.lastTriggeredByRow[0][a.ID] = false
	ok, _, _, _ = g.evalNodePlayback(0, 1, model.BeatInfo{NodeID: b.ID, NodeType: model.NodeTypeRegular})
	if !ok {
		t.Fatalf("expected b to trigger when prev skipped")
	}
}
