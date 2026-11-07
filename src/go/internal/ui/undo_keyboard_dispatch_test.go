//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Unlike SetInputForTest it also covers isKeyJustPressed, which edge-triggered shortcuts read.
func stubAllInput(mx, my int, pressed map[ebiten.Key]bool, justPressed map[ebiten.Key]bool, mouseLeft bool) func() {
	oc, om, okp, okj := cursorPosition, isMouseButtonPressed, isKeyPressed, isKeyJustPressed
	cursorPosition = func() (int, int) { return mx, my }
	isMouseButtonPressed = func(b ebiten.MouseButton) bool { return mouseLeft && b == ebiten.MouseButtonLeft }
	isKeyPressed = func(k ebiten.Key) bool { return pressed[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return justPressed[k] }
	return func() {
		cursorPosition, isMouseButtonPressed, isKeyPressed, isKeyJustPressed = oc, om, okp, okj
	}
}

// A modal popup makes Update skip the editor path, so the shortcut is dispatched at the top of Update,
// independently of that gating.
func TestUndoKeyboardFiresWhilePopupOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.SetBPM(140)
	g.undoManager.record("change BPM")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected an undo step")
	}

	g.longPressPopup = true

	restore := stubAllInput(400, 300,
		map[ebiten.Key]bool{ebiten.KeyControlLeft: true},
		map[ebiten.Key]bool{ebiten.KeyZ: true},
		false)
	defer restore()

	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.undoManager.CanRedo() {
		t.Fatal("Ctrl+Z while a popup is open did not trigger undo (keyboard-gating regression)")
	}
}

// It must not also fire from handleEditor: one Ctrl+Z, one step.
func TestUndoKeyboardFiresOnceOverGrid(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.SetBPM(140)
	g.undoManager.record("bpm-1")
	g.drum.SetBPM(160)
	g.undoManager.record("bpm-2")
	depthBefore := len(g.undoManager.undo)
	if depthBefore < 2 {
		t.Fatalf("precondition: expected >=2 undo steps, got %d", depthBefore)
	}

	// A grid coordinate: top-left of the grid pane, definitely not over a panel.
	gx, gy := 10, gridTopOffset()+10
	if g.blocksAt(gx, gy) {
		t.Skip("chosen grid point unexpectedly blocks; layout-dependent")
	}

	restore := stubAllInput(gx, gy,
		map[ebiten.Key]bool{ebiten.KeyControlLeft: true},
		map[ebiten.Key]bool{ebiten.KeyZ: true},
		false)
	defer restore()

	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	consumed := depthBefore - len(g.undoManager.undo)
	if consumed != 1 {
		t.Fatalf("single Ctrl+Z consumed %d undo steps, want exactly 1 (double-dispatch?)", consumed)
	}
}
