package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

func TestDeleteNodeClearsAttachedEdges(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	if len(g.edges) == 0 {
		t.Fatal("expected edge after add")
	}

	g.edgesDirty = false // simulate cached edge layer already built

	g.deleteNode(a)

	if len(g.edges) != 0 {
		t.Fatalf("expected no edges after node deletion, got %d", len(g.edges))
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; ok {
		t.Fatalf("graph edge still present after deleting node")
	}
	if !g.edgesDirty {
		t.Fatalf("edgesDirty not set after deleting node")
	}
}
