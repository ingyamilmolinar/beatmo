package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Deleting a linear in-between node should reconnect its predecessor to its successor
// preserving a straight path.
func TestDeleteNodeReconnectsLinear(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	// Set start so game computes path
	g.start = a
	a.Start = true
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	// Delete the middle node and expect reconnect a->c
	g.deleteNode(b)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, c.ID}]; !ok {
		t.Fatalf("expected reconnect edge a->c after deleting b")
	}
}

// Only reconnect orthogonal straight paths: if predecessor and successor do not share
// row or column, do not connect.
func TestDeleteCornerDoesNotReconnectDiagonal(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.start = a
	a.Start = true
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	g.deleteNode(b)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, c.ID}]; ok {
		t.Fatalf("did not expect diagonal reconnect a->c after deleting corner b")
	}
}

// When deleting a node with multiple successors, only valid orthogonal reconnections are created.
func TestDeleteNodeReconnectsOnlyAlignedSuccessors(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular) // aligned with a
	d := g.tryAddNode(1, 1, model.NodeTypeRegular) // not aligned with a
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(b, d)
	g.start = a
	a.Start = true
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	g.deleteNode(b)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, c.ID}]; !ok {
		t.Fatalf("expected reconnect a->c")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, d.ID}]; ok {
		t.Fatalf("did not expect diagonal reconnect a->d")
	}
}
