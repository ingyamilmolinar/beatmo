package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// With auto-stitching enabled, placing a node on a pass-through intersection
// (where an edge runs) should split that edge so the new node becomes part of
// the traversal.
func TestPlaceNodeOnPassThroughSplitsEdge(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Build a 2x2 square loop: (0,0)->(2,0)->(2,2)->(0,2)->(0,0)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 2, model.NodeTypeRegular)
	d := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)

	// Place a node on a pass-through intersection along edge (0,0)->(2,0): at (1,0).
	mid := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if mid == nil {
		t.Fatalf("failed to insert mid node")
	}
	// Verify the original edge is replaced by two edges through the new node.
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; ok {
		t.Fatalf("original edge still present in graph")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, mid.ID}]; !ok {
		t.Fatalf("missing A-mid edge in graph")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{mid.ID, b.ID}]; !ok {
		t.Fatalf("missing mid-B edge in graph")
	}
}
