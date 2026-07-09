package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the invariant the user asked for: while ANY text input is
// open, Left/Right arrows must reach the text caret only and must NOT trigger
// main-grid behaviours such as camera panning. They complement the existing
// TestArrowKeysYieldToSynthValueEditor / TestArrowKeysYieldToEQValueEditor
// (which cover the shared inline numeric editor) by covering the text-input
// surfaces the global-shortcut gate currently misses.

// TestArrowKeysYieldToSaveAsDialog covers the Synth/Sampler "Save As" dialog.
// It is a focused TextInput but is neither a tree-focused zone nor the shared
// numeric editor, so handleGlobalShortcuts' gate does not yield to it — Left
// pans the grid while the user is typing a preset name.
func TestArrowKeysYieldToSaveAsDialog(t *testing.T) {
	g := newTestGameForUndo(t)
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetText("kick (saved)")
	ti.SetFocus(true)
	g.drum.saveAsDialog = &synthSaveAsDialog{textInput: ti}
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the Save As dialog text input is open (OffsetX=%v); keys must reach the dialog caret", g.cam.OffsetX)
	}
}

// TestArrowKeysYieldToSaveAsDialogRight is the Right-arrow mirror.
func TestArrowKeysYieldToSaveAsDialogRight(t *testing.T) {
	g := newTestGameForUndo(t)
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetText("kick (saved)")
	ti.SetFocus(true)
	g.drum.saveAsDialog = &synthSaveAsDialog{textInput: ti}
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowRight: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowRight must NOT pan the grid while the Save As dialog text input is open (OffsetX=%v)", g.cam.OffsetX)
	}
}

// TestArrowKeysYieldToRenameTextInput covers the portal-hosted instrument
// rename field. While the rename portal is open the user is typing into a
// TextInput, yet the global-shortcut gate has no portal check, so Left/Right
// pan the grid underneath the dialog.
func TestArrowKeysYieldToRenameTextInput(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	rc.Open()
	g.drum.openRenamePortal()
	if !g.drum.portal().IsOpen() {
		t.Fatal("setup: rename portal should be open")
	}
	if !rc.IsOpen() {
		t.Skip("rename component did not open (no anchor in this environment)")
	}
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the rename text input is open (OffsetX=%v)", g.cam.OffsetX)
	}
}

// Arrows must not pan while the instrument menu (search box) is open.
func TestArrowKeysYieldToInstrumentMenu(t *testing.T) {
	g := newTestGameForUndo(t)
	m := g.drum.instMenuComp
	if m == nil {
		t.Skip("no instrument menu component")
	}
	m.Open()
	g.drum.openInstMenuPortal()
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the instrument menu is open (OffsetX=%v)", g.cam.OffsetX)
	}
}

// Arrows must not pan while the WAV-naming box is focused.
func TestArrowKeysYieldToNamingBox(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.nameBox = NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	g.drum.nameBox.SetText("loop")
	g.drum.nameBox.SetFocus(true)
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowRight: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowRight must NOT pan the grid while the WAV-naming box is focused (OffsetX=%v)", g.cam.OffsetX)
	}
}
