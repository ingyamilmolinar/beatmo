//go:build test

package ui

import "testing"

// Mute/solo are session state, absent from the exported document, so snapshot undo cannot restore them: they
// change no bytes and record no step. Revisit together if they ever join the export schema.
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
