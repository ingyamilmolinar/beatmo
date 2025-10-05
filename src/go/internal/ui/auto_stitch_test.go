package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// helper: find edge presence between two grid coords
func hasEdge(g *Game, ai, aj, bi, bj int) bool {
	var aNode, bNode *uiNode
	for _, n := range g.nodes {
		if n.I == ai && n.J == aj {
			aNode = n
		}
		if n.I == bi && n.J == bj {
			bNode = n
		}
	}
	if aNode == nil || bNode == nil {
		return false
	}
	for _, e := range g.edges {
		if (e.A == aNode && e.B == bNode) || (e.A == bNode && e.B == aNode) {
			return true
		}
	}
	return false
}

func TestAutoStitchHorizontal(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	if !hasEdge(g, 0, 0, 3, 0) {
		t.Fatalf("missing initial edge")
	}
	// Insert a node in the middle; should split into two edges.
	mid := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if hasEdge(g, 0, 0, 3, 0) {
		t.Fatalf("old edge not removed")
	}
	if !hasEdge(g, 0, 0, 1, 0) || !hasEdge(g, 1, 0, 3, 0) {
		t.Fatalf("split edges not present")
	}
	// Ensure beat path now includes the mid node as a waypoint by checking graph edges map.
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, mid.ID}]; !ok {
		t.Fatalf("graph missing A-mid edge")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{mid.ID, b.ID}]; !ok {
		t.Fatalf("graph missing mid-B edge")
	}
}

func TestAutoStitchVertical(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 4, model.NodeTypeRegular)
	g.addEdge(a, b)
	if !hasEdge(g, 0, 0, 0, 4) {
		t.Fatalf("missing initial edge")
	}
	// Insert vertical mid
	mid := g.tryAddNode(0, 2, model.NodeTypeRegular)
	if hasEdge(g, 0, 0, 0, 4) {
		t.Fatalf("old edge not removed")
	}
	if !hasEdge(g, 0, 0, 0, 2) || !hasEdge(g, 0, 2, 0, 4) {
		t.Fatalf("split edges not present")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, mid.ID}]; !ok {
		t.Fatalf("graph missing A-mid edge")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{mid.ID, b.ID}]; !ok {
		t.Fatalf("graph missing mid-B edge")
	}
}

func TestAutoStitchCrossingBothAxes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Horizontal A -- B
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	// Vertical C -- D crossing at (1,0)
	c := g.tryAddNode(1, -2, model.NodeTypeRegular)
	d := g.tryAddNode(1, 2, model.NodeTypeRegular)
	g.addEdge(c, d)
	// Insert node at (1,0) → should split both horizontal and vertical edges
	m := g.tryAddNode(1, 0, model.NodeTypeRegular)
	_ = m
	if hasEdge(g, 0, 0, 3, 0) {
		t.Fatalf("old horizontal edge intact")
	}
	if hasEdge(g, 1, -2, 1, 2) {
		t.Fatalf("old vertical edge intact")
	}
	if !hasEdge(g, 0, 0, 1, 0) || !hasEdge(g, 1, 0, 3, 0) {
		t.Fatalf("horizontal splits missing")
	}
	if !hasEdge(g, 1, -2, 1, 0) || !hasEdge(g, 1, 0, 1, 2) {
		t.Fatalf("vertical splits missing")
	}
}
