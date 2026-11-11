package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// When creating a node via origin selection, ensure we do not auto-stitch
// edges of other circuits that happen to be colinear at the clicked location.
func TestOriginSelectDoesNotStitchOtherCircuits(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Circuit A: a0 -> a1 across y=0
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a1 := g.tryAddNode(6, 0, model.NodeTypeRegular)
	g.addEdge(a0, a1)
	if _, ok := g.graph.Edges[[2]model.NodeID{a0.ID, a1.ID}]; !ok {
		t.Fatalf("expected edge a0->a1")
	}
	edgesBefore := len(g.graph.Edges)

	// Add Row B and select origin, then click at (3,0) which lies on A's edge.
	g.drum.AddRow()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	btn := g.drum.rowOriginBtns()[1]
	r := btn.Rect()
	_ = btn.HandleInputResult((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	_ = btn.HandleInputResult((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, false)
	_ = g.Update()
	// Directly create node at grid to avoid screen coord math.
	n := g.tryAddNode(3, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("new node not created")
	}
	if g.drum.Rows[1].Origin != n.ID {
		t.Fatalf("row1 origin not set to new node")
	}

	// Verify A's single edge remains (no stitching), and total graph edges didn't increase by split count.
	if _, ok := g.graph.Edges[[2]model.NodeID{a0.ID, a1.ID}]; !ok {
		t.Fatalf("edge a0->a1 missing (stitching occurred)")
	}
	if len(g.graph.Edges) != edgesBefore {
		t.Fatalf("edges changed by %d during origin-select placement", len(g.graph.Edges)-edgesBefore)
	}
}
