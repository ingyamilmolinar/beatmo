//go:build test

package ui

import (
	"image/color"
	"testing"
)

// Through the production Update then Draw, not syncUndoRedoVisual by hand, which cannot show a dim that is never
// applied in the real loop.
func TestUndoRedoButtonsDimViaRealFrame(t *testing.T) {
	g := newTestGameForUndo(t)
	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons not constructed")
	}
	dimmed := color.Color(colTextDisabled)
	lit := color.Color(colTextSecondary)

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

// Building the startup demo is not a user action, so the stack starts empty and both buttons greyed.
func TestStartupUndoRedoButtonsGreyed(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Inside the per-frame undo bracket, as startup does.
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

// nil falls back to the default the Draw path uses.
func iconColorOf(b *Button) color.Color {
	if b.IconColor == nil {
		return color.Color(colButtonBorder)
	}
	return b.IconColor
}
