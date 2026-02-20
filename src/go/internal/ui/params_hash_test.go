package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestParamsHash_StableAcrossCalls(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Populate enough nodes/params so map iteration order changes would matter.
	a := g.graph.AddNode(0, 0, model.NodeTypeRegular)
	b := g.graph.AddNode(1, 0, model.NodeTypeMute)
	c := g.graph.AddNode(2, 0, model.NodeTypeSilent)

	if n, ok := g.graph.GetNodeByID(a); ok {
		p := n.Params
		p.Volume = 0.8
		p.Duration = 1.25
		g.graph.SetNodeParams(a, p)
	}
	if n, ok := g.graph.GetNodeByID(b); ok {
		p := n.Params
		p.Pitch = 1.5
		p.LogicKind = "probability"
		p.LogicP = 0.25
		p.GrooveKind = "rush"
		p.GroovePct = 0.1
		g.graph.SetNodeParams(b, p)
	}
	if n, ok := g.graph.GetNodeByID(c); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(c, p)
	}

	want := g.paramsHash()
	for i := 0; i < 50; i++ {
		if got := g.paramsHash(); got != want {
			t.Fatalf("expected stable params hash across calls; iter=%d got=%d want=%d", i, got, want)
		}
	}
}

func TestParamsHash_ChangesOnParamEdit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	id := g.graph.AddNode(0, 0, model.NodeTypeRegular)
	before := g.paramsHash()

	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		t.Fatalf("node missing")
	}
	p := n.Params
	p.Volume = 0.5
	g.graph.SetNodeParams(id, p)

	after := g.paramsHash()
	if before == after {
		t.Fatalf("expected params hash to change after edit; hash=%d", after)
	}
}

func TestParamsHash_IgnoresInvisibleNodes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	_ = g.graph.AddNode(0, 0, model.NodeTypeRegular)
	before := g.paramsHash()

	_ = g.graph.AddNode(10, 10, model.NodeTypeInvisible)
	after := g.paramsHash()
	if before != after {
		t.Fatalf("expected params hash to ignore invisible nodes; before=%d after=%d", before, after)
	}
}
