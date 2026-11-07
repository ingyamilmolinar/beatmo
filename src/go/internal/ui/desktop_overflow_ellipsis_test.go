//go:build test

package ui

import (
	"image"
	"testing"
)

// At 1280x720 the transport occupies the narrow left column.
func newDesktopGameForTransport(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	withSmallScreen(t, false) // desktop profile
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	g.Layout(1280, 720)
	return g
}

// Hidden inline; the folder button reaches them.
func TestDesktopFileOpsBehindFolderButton(t *testing.T) {
	g := newDesktopGameForTransport(t)
	dv := g.drum

	// The grid folder button (file-menu trigger) must be present on desktop.
	if got := g.gridFileButtonRect(); got.Empty() {
		t.Errorf("desktop folder button rect is empty — the file menu trigger is hidden")
	}

	if got := dv.uploadBtn().Rect(); !got.Empty() {
		t.Errorf("upload button rect = %v, want empty (moved behind file menu)", got)
	}
	if got := dv.importBtn().Rect(); !got.Empty() {
		t.Errorf("import button rect = %v, want empty (moved behind file menu)", got)
	}
	if got := dv.exportBtn().Rect(); !got.Empty() {
		t.Errorf("export button rect = %v, want empty (moved behind file menu)", got)
	}
}

// Icons, so a narrow cell never clips "Undo"/"Redo" to "...", which read as an overflow button.
func TestDesktopUndoRedoUseIconsNotEllipsisText(t *testing.T) {
	g := newDesktopGameForTransport(t)
	tz := g.drum.transportZone

	if tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons missing on desktop")
	}
	for name, b := range map[string]*Button{"undo": tz.undoBtn, "redo": tz.redoBtn} {
		if b.Rect().Empty() {
			t.Errorf("%s button rect is empty — should stay inline on desktop", name)
		}
		if b.Icon == "" {
			t.Errorf("%s button has no Icon — text label collapses to \"...\" in the narrow cell", name)
		}
		if b.Text != "" {
			t.Errorf("%s button still has Text=%q — icon buttons must not carry clippable text", name, b.Text)
		}
		// Never the bare "...".
		clipped := clipTextToWidth(b.Text, b.Rect().Dx())
		if clipped == "..." {
			t.Errorf("%s button still renders as \"...\" (rect width=%d)", name, b.Rect().Dx())
		}
	}
}

// Anchored under the folder button in the grid pane, so not confined to the drum pane.
func TestDesktopFileMenuOpensVisible(t *testing.T) {
	g := newDesktopGameForTransport(t)
	dv := g.drum

	g.openFileMenu()
	if !dv.IsOverflowMenuOpen() {
		t.Fatalf("openFileMenu did not open the file menu")
	}
	popup := dv.overflowPopupRect()
	if popup.Empty() {
		t.Errorf("file popup rect is empty after opening on desktop")
	}
	screen := image.Rect(0, 0, g.winW, g.winH)
	if !popup.In(screen) {
		t.Errorf("file popup rect %v not within screen %v", popup, screen)
	}
}
