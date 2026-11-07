package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// While any text input is open, Left/Right reach the caret only and never pan the grid.

// A focused TextInput that is neither a tree-focused zone nor a numeric editor.
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

// The portal-hosted rename field.
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

// Laid out so the pinned search box exists, then focused.
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

// The overflow menu overlay claims the keyboard, so arrows move the caret, not the camera beneath.
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

// While open its overlay tree owns the keyboard, so the grid's shortcuts yield, as the instrument menu's do.
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
