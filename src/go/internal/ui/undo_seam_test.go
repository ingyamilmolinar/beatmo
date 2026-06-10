package ui

import "testing"

func TestUndoSeam_RoundTripPreservesDocument(t *testing.T) {
	g := newTestGameForUndo(t)
	before := g.undoCapture()

	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	after := g.undoCapture()
	if string(before) == string(after) {
		t.Fatal("expected document to change after AddRow")
	}

	g.undoManager.Undo()
	restored := g.undoCapture()
	if string(restored) != string(before) {
		t.Fatalf("undo did not restore byte-identical document\nbefore=%s\nrestored=%s", before, restored)
	}
}

func newTestGameForUndo(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()
	// Re-establish the committed baseline after Layout has built the default
	// start node so that subsequent record() calls compare against the fully
	// initialised document state, not the pre-Layout empty graph.
	g.undoManager.OnExternalLoad()
	return g
}
