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

// openTemplateMenuSearchFocused opens the overflow menu on the template page
// (page 1), lays it out so the pinned search box is created, and focuses it.
func openTemplateMenuSearchFocused(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	dv.overflowPopupBtns(dv.overflowPopupRect()) // creates templateSearchBox + rect
	if dv.templateSearchBox == nil {
		t.Fatal("setup: template search box not created on overflow page 1")
	}
	dv.templateSearchBox.SetText("bach")
	dv.templateSearchBox.SetFocus(true)
}

// Arrows must not pan the grid while the template-menu search box (overflow
// page 1) is open + focused — the caret must move, the camera must not.
// Regression: the overflow menu overlay (dvOverlayPortal) did not implement
// KeyboardClaimant, so the grid keyboard gate never yielded to it and Left/Right
// panned the grid under the open menu (a sibling-tree menu leaking into the grid).
func TestArrowKeysYieldToTemplateMenuSearch(t *testing.T) {
	g := newTestGameForUndo(t)
	openTemplateMenuSearchFocused(t, g)
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the template-menu search box is open (OffsetX=%v); keys must reach the search caret", g.cam.OffsetX)
	}
}

// TestArrowKeysYieldToTemplateMenuSearchRight is the Right-arrow mirror.
func TestArrowKeysYieldToTemplateMenuSearchRight(t *testing.T) {
	g := newTestGameForUndo(t)
	openTemplateMenuSearchFocused(t, g)
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowRight: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowRight must NOT pan the grid while the template-menu search box is open (OffsetX=%v)", g.cam.OffsetX)
	}
}

// TestOverflowMenuClaimsKeyboardWhileOpen pins the per-tree isolation invariant
// at the contract level: while the overflow (template) menu is open, its subtree
// (overlayTree) owns the keyboard so DrumView.KeyboardClaimed() is true and the
// sibling GridTree's keyboard shortcuts yield entirely. Mirrors the instrument
// menu's claim-while-open semantics (InstrumentMenuComponent.ClaimsKeyboard).
func TestOverflowMenuClaimsKeyboardWhileOpen(t *testing.T) {
	g := newTestGameForUndo(t)
	dv := g.drum
	if dv.KeyboardClaimed() {
		t.Fatal("precondition: keyboard must be unclaimed with no popup open")
	}
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	if !dv.KeyboardClaimed() {
		t.Fatal("overflow menu open: KeyboardClaimed() must be true so the grid keyboard shortcuts yield to the menu")
	}
	dv.closeOverflowMenu()
	if dv.KeyboardClaimed() {
		t.Fatal("overflow menu closed: keyboard ownership must be released back to the grid")
	}
}
