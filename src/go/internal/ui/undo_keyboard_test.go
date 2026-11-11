package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestUndoKeyboardShortcut(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyControlLeft: true}, map[ebiten.Key]bool{ebiten.KeyZ: true})
	defer restore()

	if !g.handleUndoRedoKeys() {
		t.Fatal("Ctrl+Z chord should have been consumed by handleUndoRedoKeys")
	}
	if !g.undoManager.CanRedo() {
		t.Fatal("Ctrl+Z should have triggered undo (redo now available)")
	}
}

func TestRedoKeyboardShortcut(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.undoManager.record("add-row")
	g.undoManager.Undo()
	if !g.undoManager.CanRedo() {
		t.Fatal("setup: expected redo available after undo")
	}

	// Ctrl+Shift+Z should redo.
	restore := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyControlLeft: true, ebiten.KeyShiftLeft: true},
		map[ebiten.Key]bool{ebiten.KeyZ: true},
	)
	if !g.handleUndoRedoKeys() {
		restore()
		t.Fatal("Ctrl+Shift+Z chord should have been consumed")
	}
	if g.undoManager.CanRedo() {
		restore()
		t.Fatal("Ctrl+Shift+Z should have triggered redo (redo stack now empty)")
	}
	restore()

	// Undo again, then Ctrl+Y should redo.
	g.undoManager.Undo()
	if !g.undoManager.CanRedo() {
		t.Fatal("setup: expected redo available after second undo")
	}
	restore2 := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyControlLeft: true},
		map[ebiten.Key]bool{ebiten.KeyY: true},
	)
	defer restore2()
	if !g.handleUndoRedoKeys() {
		t.Fatal("Ctrl+Y chord should have been consumed")
	}
	if g.undoManager.CanRedo() {
		t.Fatal("Ctrl+Y should have triggered redo (redo stack now empty)")
	}
}

func stubKeys(pressed, justPressed map[ebiten.Key]bool) func() {
	oldP, oldJ := isKeyPressed, isKeyJustPressed
	isKeyPressed = func(k ebiten.Key) bool { return pressed[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return justPressed[k] }
	return func() { isKeyPressed, isKeyJustPressed = oldP, oldJ }
}
