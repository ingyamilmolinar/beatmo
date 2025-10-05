package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Build a small loop: s0 -> a -> s1 -> b -> s2 -> s0
func buildPrevChain(g *Game) (s0, a, b *uiNode) {
	s0 = g.tryAddNode(0, 0, model.NodeTypeSilent)
	a = g.tryAddNode(1, 0, model.NodeTypeRegular)
	s1 := g.tryAddNode(2, 0, model.NodeTypeSilent)
	b = g.tryAddNode(3, 0, model.NodeTypeRegular)
	s2 := g.tryAddNode(4, 0, model.NodeTypeSilent)
	g.addEdge(s0, a)
	g.addEdge(a, s1)
	g.addEdge(s1, b)
	g.addEdge(b, s2)
	g.addEdge(s2, s0)
	g.updateBeatInfos()
	return s0, a, b
}

func TestRuntimeTriggerIfPrevTriggered(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	s0, a, b := buildPrevChain(g)
	g.start = s0
	g.graph.StartNodeID = s0.ID
	// b triggers only if previous triggered
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(b.ID, p)
	}
	// Find absolute indices for a and b
	idxA, idxB := -1, -1
	for i := 0; i < 16; i++ {
		info := g.beatInfoAtRow(0, i)
		if info.NodeID == a.ID {
			idxA = i
		}
		if info.NodeID == b.ID {
			idxB = i
		}
	}
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing a/b in beat path")
	}
	// Simulate eval for A (triggered)
	ok, _, _, _ := g.evalNodePlayback(0, idxA, g.beatInfoAtRow(0, idxA))
	if !ok {
		t.Fatalf("expected a to trigger")
	}
	if g.lastTriggeredByRow[0] == nil {
		g.lastTriggeredByRow[0] = map[model.NodeID]bool{}
	}
	g.lastTriggeredByRow[0][a.ID] = true
	// Now B should trigger
	ok, _, _, _ = g.evalNodePlayback(0, idxB, g.beatInfoAtRow(0, idxB))
	if !ok {
		t.Fatalf("expected b to trigger when prev a triggered")
	}
	// Now force a to skip and ensure b suppresses
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 1
		g.graph.SetNodeParams(a.ID, p)
	}
	ok, _, _, _ = g.evalNodePlayback(0, idxA, g.beatInfoAtRow(0, idxA))
	if ok {
		t.Fatalf("expected a to skip")
	}
	g.lastTriggeredByRow[0][a.ID] = false
	ok, _, _, _ = g.evalNodePlayback(0, idxB, g.beatInfoAtRow(0, idxB))
	if ok {
		t.Fatalf("expected b suppressed when prev a skipped")
	}
}

func TestRuntimeTriggerIfPrevSkipped(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	s0, a, b := buildPrevChain(g)
	g.start = s0
	g.graph.StartNodeID = s0.ID
	// b triggers only if previous skipped
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(b.ID, p)
	}
	// Indices
	idxA, idxB := -1, -1
	for i := 0; i < 16; i++ {
		info := g.beatInfoAtRow(0, i)
		if info.NodeID == a.ID {
			idxA = i
		}
		if info.NodeID == b.ID {
			idxB = i
		}
	}
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing a/b in beat path")
	}
	// A triggers -> B should suppress
	ok, _, _, _ := g.evalNodePlayback(0, idxA, g.beatInfoAtRow(0, idxA))
	if !ok {
		t.Fatalf("expected a to trigger initially")
	}
	if g.lastTriggeredByRow[0] == nil {
		g.lastTriggeredByRow[0] = map[model.NodeID]bool{}
	}
	g.lastTriggeredByRow[0][a.ID] = true
	ok, _, _, _ = g.evalNodePlayback(0, idxB, g.beatInfoAtRow(0, idxB))
	if ok {
		t.Fatalf("expected b suppressed when prev a triggered")
	}
	// Now force a to skip -> B should trigger
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 1
		g.graph.SetNodeParams(a.ID, p)
	}
	ok, _, _, _ = g.evalNodePlayback(0, idxA, g.beatInfoAtRow(0, idxA))
	if ok {
		t.Fatalf("expected a to skip with skip_every_n=1")
	}
	g.lastTriggeredByRow[0][a.ID] = false
	ok, _, _, _ = g.evalNodePlayback(0, idxB, g.beatInfoAtRow(0, idxB))
	if !ok {
		t.Fatalf("expected b to trigger when prev a skipped")
	}
}
