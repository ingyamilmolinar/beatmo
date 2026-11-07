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

func TestImportClearsUndoHistory(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected undo available")
	}
	snap := g.undoCapture()
	if err := g.Import(snap); err != nil {
		t.Fatalf("import: %v", err)
	}
	if g.undoManager.CanUndo() || g.undoManager.CanRedo() {
		t.Fatal("real Import must clear undo history")
	}
}

func newTestGameForUndo(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()
	// After Layout built the default start node, so record() compares against the initialised document, not an empty
	// graph.
	g.undoManager.OnExternalLoad()
	return g
}
