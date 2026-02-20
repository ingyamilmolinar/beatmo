//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// setupOverlayTestDV creates a DrumView with one row and runs a warm-up frame.
func setupOverlayTestDV(t *testing.T, w, h int) *DrumView {
	t.Helper()
	bounds := image.Rect(0, 0, w, h)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame so layout is fully initialised.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	restore()
	return dv
}

// TestEditButtonSuppressesAdjacentButtons verifies that clicking the edit
// (pencil) button sets suppressClicksUntilRelease so adjacent buttons (label)
// cannot fire on subsequent frames of the same press.
func TestEditButtonSuppressesAdjacentButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := setupOverlayTestDV(t, 390, 600)

	if len(dv.rowEditBtns) == 0 {
		t.Fatal("no edit buttons after layout")
	}

	// Directly fire the edit button's OnClick.
	dv.rowEditBtns[0].OnClick()

	// Rename should be open.
	if dv.renameRow != 0 {
		t.Fatalf("renameRow=%d, want 0", dv.renameRow)
	}

	// SuppressClicksUntilMouseUp should have been called.
	if !suppressClicksUntilRelease {
		t.Fatal("suppressClicksUntilRelease should be true after edit OnClick")
	}

	// Context menu must NOT be open.
	if dv.contextMenuOpen {
		t.Fatal("context menu should NOT be open — edit button should suppress adjacent buttons")
	}
}

// TestEditButtonClosesContextMenu verifies that when the context menu is open,
// firing the edit button's OnClick closes it before opening rename.
func TestEditButtonClosesContextMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := setupOverlayTestDV(t, 390, 600)

	// Open context menu first.
	dv.OpenContextMenuForTest(0)
	if !dv.contextMenuOpen {
		t.Fatal("context menu should be open after OpenContextMenuForTest")
	}

	// Fire edit button — should close context menu and open rename.
	dv.rowEditBtns[0].OnClick()

	if dv.contextMenuOpen {
		t.Fatal("context menu should be CLOSED after edit button OnClick")
	}
	if dv.renameRow != 0 {
		t.Fatalf("renameRow=%d, want 0 (rename should be open)", dv.renameRow)
	}
}

// TestRenameOverlayBlocksRowButtons verifies that while the rename overlay is
// open, clicking the label area does NOT fire the label button (which would
// open the context menu on mobile).
func TestRenameOverlayBlocksRowButtons(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := setupOverlayTestDV(t, 390, 600)

	// Open rename via edit button.
	dv.rowEditBtns[0].OnClick()
	if dv.renameBox == nil && (dv.renameComp == nil || !dv.renameComp.IsOpen()) {
		t.Fatal("rename should be open after edit button OnClick")
	}

	// Clear suppress so we can test the overlay guard independently.
	suppressClicksUntilRelease = false

	// Click the label area — should be blocked by overlay guard.
	lblRect := dv.rowLabels[0].Rect()
	if lblRect.Empty() {
		t.Skip("label rect empty")
	}
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 600 },
	)
	dv.Update()
	restore()

	if dv.contextMenuOpen {
		t.Fatal("context menu should NOT open while rename overlay is active")
	}
}

// TestContextMenuRenameOpensCleanly verifies that tapping the Rename button in
// the context menu closes the context menu AND opens rename — not both at once.
func TestContextMenuRenameOpensCleanly(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := setupOverlayTestDV(t, 390, 600)

	// Open context menu.
	dv.OpenContextMenuForTest(0)
	if !dv.contextMenuOpen {
		t.Fatal("context menu should be open")
	}

	// Find the Rename button in context menu buttons.
	var renameBtnFound bool
	for _, btn := range dv.contextMenuBtns {
		if btn.Text == "Rename" && btn.OnClick != nil {
			btn.OnClick()
			renameBtnFound = true
			break
		}
	}
	if !renameBtnFound {
		t.Skip("no Rename button found in context menu")
	}

	// Context menu should be closed, rename should be open.
	if dv.contextMenuOpen {
		t.Fatal("context menu should be CLOSED after tapping Rename")
	}
	if dv.renameRow < 0 {
		t.Fatal("rename should be open after tapping Rename in context menu")
	}
}

// TestOverlayBlocksToolbarButtons verifies that when an overlay is open,
// toolbar buttons (play, stop, etc.) don't fire.
func TestOverlayBlocksToolbarButtons(t *testing.T) {
	assertDefaultParityState(t)

	dv := setupOverlayTestDV(t, 800, 600)

	// Open inst menu to create an open overlay.
	dv.openInstMenuForRow(0)
	if !dv.isInstMenuOpen() {
		t.Fatal("inst menu should be open")
	}

	// Record whether play button fires.
	playFired := false
	origOnClick := dv.playBtn.OnClick
	dv.playBtn.OnClick = func() {
		playFired = true
		if origOnClick != nil {
			origOnClick()
		}
	}

	// Click on the play button area.
	playRect := dv.playBtn.Rect()
	if playRect.Empty() {
		t.Skip("play button rect empty")
	}
	px := playRect.Min.X + playRect.Dx()/2
	py := playRect.Min.Y + playRect.Dy()/2

	// Clear suppress so we're testing the overlay guard only.
	suppressClicksUntilRelease = false

	clickDrumView(t, dv, px, py)

	if playFired {
		t.Fatal("play button should NOT fire while inst menu overlay is open")
	}
}

// TestInstMenuToggleViaOverlay verifies that clicking the label while the inst
// menu is open toggles the menu closed via the overlay HandleInput (not via
// the button in Update, which is now blocked by the overlay guard).
func TestInstMenuToggleViaOverlay(t *testing.T) {
	assertDefaultParityState(t)

	dv := setupOverlayTestDV(t, 800, 600)

	// Open inst menu.
	dv.openInstMenuForRow(0)
	if !dv.isInstMenuOpen() {
		t.Fatal("inst menu should be open")
	}

	// Clear suppress so we can click.
	suppressClicksUntilRelease = false

	lblRect := dv.rowLabels[0].Rect()
	if lblRect.Empty() {
		t.Fatal("label rect empty")
	}
	cx := lblRect.Min.X + lblRect.Dx()/2
	cy := lblRect.Min.Y + lblRect.Dy()/2

	// Simulate full input flow: HandleInput (overlay stack) + Update.
	// In the real Game loop, HandleInput runs before Update each frame.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.HandleInput(cx, cy, true)
	dv.Update()
	restore()

	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.HandleInput(cx, cy, false)
	dv.Update()
	restore()

	if dv.isInstMenuOpen() {
		t.Fatal("inst menu should be CLOSED after clicking label while it was open")
	}
}
