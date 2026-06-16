package ui

import (
	"image/color"
	"testing"
)

// TestTransportUndoRedoButtonsWired verifies the transport-bar Undo/Redo
// buttons exist and route through the wired callbacks into the Game's
// UndoManager: clicking Undo after a recorded action makes Redo available.
func TestTransportUndoRedoButtonsWired(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons not constructed")
	}
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected CanUndo after record")
	}

	// The buttons defer the restore via QueueAction (so it never runs under the
	// seqMu that Game.Update holds during dispatch); Game.Update drains the queue
	// after releasing the lock, which drainPendingActions reproduces here.
	tz.undoBtn.OnClick()
	g.drainPendingActions()
	if !g.undoManager.CanRedo() {
		t.Fatal("after undo-button click, redo should be available")
	}

	tz.redoBtn.OnClick()
	g.drainPendingActions()
	if !g.undoManager.CanUndo() {
		t.Fatal("after redo-button click, undo should be available again")
	}
}

// TestTransportUndoRedoDim verifies the labels dim when the respective stack
// is empty and re-light when it is non-empty, via syncUndoRedoVisual.
func TestTransportUndoRedoDim(t *testing.T) {
	g := newTestGameForUndo(t)
	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons not constructed")
	}

	dimmed := color.Color(colTextDisabled)
	lit := color.Color(colTextPrimary)

	// Fresh manager: both stacks empty -> both dim.
	tz.syncUndoRedoVisual()
	if tz.undoBtn.TextColor != dimmed {
		t.Fatalf("undo should be dim when stack empty, got %v", tz.undoBtn.TextColor)
	}
	if tz.redoBtn.TextColor != dimmed {
		t.Fatalf("redo should be dim when stack empty, got %v", tz.redoBtn.TextColor)
	}

	// Record an action -> undo lights up, redo still dim.
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")
	tz.syncUndoRedoVisual()
	if tz.undoBtn.TextColor != lit {
		t.Fatalf("undo should be lit when stack non-empty, got %v", tz.undoBtn.TextColor)
	}
	if tz.redoBtn.TextColor != dimmed {
		t.Fatalf("redo should still be dim, got %v", tz.redoBtn.TextColor)
	}

	// Undo -> redo lights up.
	g.undoManager.Undo()
	tz.syncUndoRedoVisual()
	if tz.redoBtn.TextColor != lit {
		t.Fatalf("redo should be lit after undo, got %v", tz.redoBtn.TextColor)
	}
}
