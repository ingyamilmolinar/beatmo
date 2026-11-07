package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestUndoRoundTripPerAction(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(g *Game)
		label  string
	}{
		{"add-row", func(g *Game) { g.drum.AddRow() }, "add row"},
		{"bpm", func(g *Game) { g.drum.SetBPM(140) }, "change BPM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGameForUndo(t)
			before := g.undoCapture()
			tc.mutate(g)
			g.updateBeatInfos()
			g.undoManager.record(tc.label)
			after := g.undoCapture()
			if string(after) == string(before) {
				t.Fatalf("%s did not change the document", tc.name)
			}
			g.undoManager.Undo()
			if got := g.undoCapture(); string(got) != string(before) {
				t.Fatalf("%s: undo not byte-identical", tc.name)
			}
			g.undoManager.Redo()
			if got := g.undoCapture(); string(got) != string(after) {
				t.Fatalf("%s: redo not byte-identical", tc.name)
			}
		})
	}
}

// Through the real delete path, which drops the incident edge from both g.edges and g.graph.Edges.
func TestUndoNodeDeleteCascade(t *testing.T) {
	g := newTestGameForUndo(t)

	// Clear of the default start node, so these are fresh nodes.
	a := g.tryAddNode(5, 5, model.NodeTypeRegular)
	b := g.tryAddNode(6, 5, model.NodeTypeRegular)
	if a == nil || b == nil {
		t.Fatalf("failed to create nodes: a=%v b=%v", a, b)
	}
	g.addEdge(a, b)
	g.updateBeatInfos()
	g.undoManager.record("build A->B")

	// Sanity: the edge exists before delete.
	edgeKey := [2]model.NodeID{a.ID, b.ID}
	if _, ok := g.graph.Edges[edgeKey]; !ok {
		t.Fatalf("precondition: edge A->B (%d->%d) missing before delete", a.ID, b.ID)
	}
	if _, ok := g.graph.GetNodeByID(a.ID); !ok {
		t.Fatalf("precondition: node A (%d) missing before delete", a.ID)
	}

	// Delete node A through the real UI path; this cascades the incident edge.
	aID := a.ID
	g.deleteNode(a)
	g.undoManager.record("delete node")

	// Confirm the cascade actually removed both node and edge.
	if _, ok := g.graph.GetNodeByID(aID); ok {
		t.Fatalf("delete did not remove node A (%d)", aID)
	}
	if _, ok := g.graph.Edges[edgeKey]; ok {
		t.Fatalf("delete did not cascade-remove edge A->B (%d->%d)", aID, b.ID)
	}

	// Undo the delete: node A and edge A->B must both be back.
	g.undoManager.Undo()

	if _, ok := g.graph.GetNodeByID(aID); !ok {
		t.Fatalf("undo did not restore node A (%d)", aID)
	}
	if _, ok := g.graph.Edges[edgeKey]; !ok {
		t.Fatalf("undo did not restore cascaded edge A->B (%d->%d)", aID, b.ID)
	}
}

// The restore re-seeks to the captured beat; ±1 subdivision allows a stub tick between reads.
func TestUndoPreservesPlayheadDuringPlayback(t *testing.T) {
	g := newTestGameForUndo(t)
	g.SetPlaying(true)
	g.Seek(3)
	g.drum.SetBPM(150)
	g.updateBeatInfos()
	g.undoManager.record("change BPM")

	div := max1(g.grid.MaxDiv())
	beatBefore := g.playheadAbsSubdiv() / div
	g.undoManager.Undo()
	beatAfter := g.playheadAbsSubdiv() / div

	if beatBefore == 0 {
		t.Fatalf("precondition: expected non-zero playhead beat after Seek(3), got 0")
	}
	delta := beatAfter - beatBefore
	if delta < 0 {
		delta = -delta
	}
	if delta > 1 {
		t.Fatalf("playhead beat moved across undo: before=%d after=%d (tolerance ±1 beat)", beatBefore, beatAfter)
	}
	if beatAfter == 0 {
		t.Fatalf("playhead reset to beat 0 across undo: before=%d after=%d", beatBefore, beatAfter)
	}
}
