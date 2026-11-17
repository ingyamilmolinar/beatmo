//go:build test

package ui

import "testing"

// TestMuteSoloNotUndoableByDesign documents and pins the deliberate decision
// that row mute/solo are NOT on the undo stack: they are session state, absent
// from the exported document, so a snapshot-based undo cannot restore them.
// Toggling them must (a) leave exportBytes unchanged and (b) produce no undo
// step. If mute/solo are ever added to the export schema, this test should be
// updated alongside re-adding them to the recorded-set.
func TestMuteSoloNotUndoableByDesign(t *testing.T) {
	g := newTestGameForUndo(t)
	if len(g.drum.Rows) < 1 {
		t.Skip("need at least one row")
	}
	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depthBefore := len(g.undoManager.undo)

	g.drum.toggleMute(0)
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("mute toggle changed the exported document (it should be session-only state)")
	}
	if len(g.undoManager.undo) != depthBefore {
		t.Fatalf("mute toggle recorded an undo step (%d->%d); mute is not snapshot-undoable",
			depthBefore, len(g.undoManager.undo))
	}

	g.drum.toggleSolo(0)
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("solo toggle changed the exported document (it should be session-only state)")
	}
	if len(g.undoManager.undo) != depthBefore {
		t.Fatalf("solo toggle recorded an undo step (%d->%d); solo is not snapshot-undoable",
			depthBefore, len(g.undoManager.undo))
	}
}
