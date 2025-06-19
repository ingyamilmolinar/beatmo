package model

import "testing"

func TestAddNodeTogglesRow(t *testing.T) {
	g := NewGraph()
	g.AddNode(2, 0)
	if !g.Row[2] {
		t.Fatal("step 2 should be ON after adding node at (2,0)")
	}
}

func TestToggleStepAddsOrRemovesNode(t *testing.T) {
	g := NewGraph()
	g.ToggleStep(1)
	if _, ok := findNode(g, 1, 0); !ok {
		t.Fatal("expected node at (1,0)")
	}
	g.ToggleStep(1)
	if _, ok := findNode(g, 1, 0); ok {
		t.Fatal("node should be gone after toggling off")
	}
}

func TestRemoveNodeClearsRow(t *testing.T) {
	g := NewGraph()
	id := g.AddNode(0, 0)
	g.RemoveNode(id)
	if len(g.Row) > 0 && g.Row[0] {
		t.Fatal("step 0 should be OFF after removing node")
	}
}

func TestEnsureRowLengthExpands(t *testing.T) {
	g := NewGraph()
	g.AddNode(5, 0)
	if len(g.Row) <= 5 {
		t.Fatalf("row not expanded; len=%d", len(g.Row))
	}
}

func findNode(g *Graph, i, j int) (NodeID, bool) {
	for id, n := range g.Nodes {
		if n.I == i && n.J == j {
			return id, true
		}
	}
	return 0, false
}
