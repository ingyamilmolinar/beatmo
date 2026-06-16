//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestUndoRedoButtonsDimViaRealFrame drives the PRODUCTION frame loop (g.Update
// then g.Draw) — not syncUndoRedoVisual directly — and asserts the transport
// undo/redo icons are greyed when their stack is empty and lit when it is not.
// The pre-existing TestTransportUndoRedoDim calls syncUndoRedoVisual() by hand,
// so it can't catch the real-app case where the dim never gets applied/refreshed
// through the actual update+draw path.
func TestUndoRedoButtonsDimViaRealFrame(t *testing.T) {
	g := newTestGameForUndo(t)
	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons not constructed")
	}
	dimmed := color.Color(colTextDisabled)
	lit := color.Color(colTextPrimary)

	img := newTrackedImage("test.undobtns", 1280, 720)
	defer releaseImage(img)

	frame := func() {
		g.Update()
		g.Draw(img)
	}

	// Beginning: nothing to undo/redo → both greyed.
	frame()
	if got := iconColorOf(tz.undoBtn); got != dimmed {
		t.Fatalf("at start undo icon must be greyed (%v), got %v", dimmed, got)
	}
	if got := iconColorOf(tz.redoBtn); got != dimmed {
		t.Fatalf("at start redo icon must be greyed (%v), got %v", dimmed, got)
	}

	// Record an action → undo lights (whiter), redo still greyed.
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")
	frame()
	if got := iconColorOf(tz.undoBtn); got != lit {
		t.Fatalf("after an action undo icon must be lit (%v), got %v", lit, got)
	}
	if got := iconColorOf(tz.redoBtn); got != dimmed {
		t.Fatalf("redo icon must still be greyed (%v), got %v", dimmed, got)
	}

	// Undo it → redo lights, undo greyed again.
	g.undoManager.Undo()
	frame()
	if got := iconColorOf(tz.redoBtn); got != lit {
		t.Fatalf("after undo, redo icon must be lit (%v), got %v", lit, got)
	}
	if got := iconColorOf(tz.undoBtn); got != dimmed {
		t.Fatalf("after undo, undo icon must be greyed again (%v), got %v", dimmed, got)
	}
}

// TestStartupUndoRedoButtonsGreyed reproduces the real-app bug: at launch the
// undo button appeared enabled (whiter) even though the user had done nothing.
// Building the initial demo recorded an undo step (the demo-vs-empty diff), so
// CanUndo was true at startup. The initial demo is not a user action — the undo
// stack must be empty and both buttons greyed until the user edits something.
func TestStartupUndoRedoButtonsGreyed(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build the demo exactly as startup does — inside the per-frame undo bracket
	// that game_update.go wraps every Update() in.
	g.undoManager.beginGroup("")
	g.buildDemo()
	g.undoManager.endGroup()

	if g.undoManager.CanUndo() {
		t.Fatal("initial demo must not count as a user action — undo stack must be empty at launch")
	}
	if g.undoManager.CanRedo() {
		t.Fatal("redo stack must be empty at launch")
	}

	tz := g.drum.transportZone
	img := newTrackedImage("test.startupundo", 1280, 720)
	defer releaseImage(img)
	g.Update()
	g.Draw(img)

	dimmed := color.Color(colTextDisabled)
	if got := iconColorOf(tz.undoBtn); got != dimmed {
		t.Fatalf("undo icon must be greyed at launch (%v), got %v", dimmed, got)
	}
	if got := iconColorOf(tz.redoBtn); got != dimmed {
		t.Fatalf("redo icon must be greyed at launch (%v), got %v", dimmed, got)
	}
}

// iconColorOf returns a button's effective icon color (nil falls back to the
// same default the Draw path uses).
func iconColorOf(b *Button) color.Color {
	if b.IconColor == nil {
		return color.Color(colButtonBorder)
	}
	return b.IconColor
}
