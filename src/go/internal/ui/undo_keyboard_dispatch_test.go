//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// stubAllInput overrides cursor, mouse, key-pressed and key-just-pressed for the
// duration of the test and returns a restore func. Unlike SetInputForTest it
// also covers isKeyJustPressed (needed for edge-triggered shortcuts).
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

// TestUndoKeyboardFiresWhilePopupOpen is the regression guard for the keyboard
// gating bug: when a long-press popup (or any modal that routes input away from
// the editor) is open, Update short-circuits before handleEditor — so Ctrl+Z
// used to do nothing. The shortcut must be dispatched unconditionally at the top
// of Update, independent of the editor/dispatcher gating.
func TestUndoKeyboardFiresWhilePopupOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.SetBPM(140)
	g.undoManager.record("change BPM")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected an undo step")
	}

	// Open the long-press popup: Update takes the popup branch and skips the
	// entire editor path (where the shortcut used to live).
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

// TestUndoKeyboardFiresOnceOverGrid guards against double-dispatch: with the
// shortcut handled in Update, it must NOT also fire from handleEditor when the
// cursor is over the grid. A single Ctrl+Z must consume exactly one undo step.
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
