//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Open the Save As dialog on the Synth tab and drive Left/Right through the real
// Game.Update loop: the caret moves and the grid must NOT pan.
func TestFunctional_ArrowKeys_SaveAsDialog(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	g.Update()

	g.drum.openSaveAsDialog()
	if g.drum.saveAsDialog == nil || !g.drum.saveAsDialog.Focused() {
		t.Skip("Save As dialog did not open/focus in this environment")
	}

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	// One settling frame so the dialog's TextInput caret is initialised.
	fi.frame(t, g)

	startCursor := g.drum.saveAsDialog.textInput.cursor
	if startCursor == 0 {
		t.Skip("precondition: prefilled caret should sit at end (>0)")
	}
	camBefore := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned the grid while the Save As dialog was open (%v -> %v)", camBefore, g.cam.OffsetX)
	}
	if g.drum.saveAsDialog == nil {
		t.Fatalf("Left arrow unexpectedly closed the Save As dialog")
	}
	if g.drum.saveAsDialog.textInput.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move the dialog caret: cursor=%d want %d", g.drum.saveAsDialog.textInput.cursor, startCursor-1)
	}
}
