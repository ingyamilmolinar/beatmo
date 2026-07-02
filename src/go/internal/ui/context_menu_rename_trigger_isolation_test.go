//go:build test

package ui

import (
	"image"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// contextMenuBtnRectByIcon returns the rect of the open context menu's button
// whose leading icon matches `icon` (e.g. "pencil" for Rename, "color" for
// Color). The icons slice is parallel to ContextMenuBtns (close button has no
// icon entry).
func contextMenuBtnRectByIcon(dv *DrumView, icon string) (image.Rectangle, bool) {
	btns := dv.ContextMenuBtns()
	icons := dv.ContextMenuIconsForTest()
	for i, ic := range icons {
		if ic == icon && i < len(btns) {
			return btns[i].Rect(), true
		}
	}
	return image.Rectangle{}, false
}

// TestMobileColorButtonDoesNotTriggerRename pins the mobile input-isolation
// contract: the "rename-N" native-input TRIGGER must be registered on the
// Rename menu item's rect — never on the adjacent Color item. The original bug
// hard-coded contextMenuBtns[1] as "Rename", but dividers are skipped when
// buttons are built, so index 1 is actually the Color item. Tapping Color then
// opened the rename text input.
func TestMobileColorButtonDoesNotTriggerRename(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	// Capture the registered trigger rects.
	testMobileInputTriggerRegistered = map[string]bool{}
	testMobileInputTriggerRect = map[string]image.Rectangle{}
	testMobileInputRect = map[string]image.Rectangle{}
	t.Cleanup(func() {
		testMobileInputTriggerRegistered = nil
		testMobileInputTriggerRect = nil
		testMobileInputRect = nil
	})

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)

	// Open the row context menu (bottom sheet on mobile) and lay out controls so
	// the mobile native-input rects get registered.
	dv.OpenContextMenu(0)
	dv.recalcButtons()

	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	renameRect, ok := contextMenuBtnRectByIcon(dv, "pencil")
	if !ok || renameRect.Empty() {
		t.Fatalf("could not find Rename (pencil) menu item rect; icons=%v", dv.ContextMenuIconsForTest())
	}
	colorRect, ok := contextMenuBtnRectByIcon(dv, "color")
	if !ok || colorRect.Empty() {
		t.Fatalf("could not find Color menu item rect; icons=%v", dv.ContextMenuIconsForTest())
	}

	trigRect, ok := testMobileInputTriggerRect["rename-0"]
	if !ok {
		t.Fatalf("rename-0 trigger was not registered; registered=%v", testMobileInputTriggerRegistered)
	}

	// The trigger must sit on the Rename item, NOT the Color item.
	if trigRect != renameRect {
		t.Errorf("rename trigger rect = %v, want Rename item rect %v", trigRect, renameRect)
	}
	if trigRect == colorRect {
		t.Errorf("rename trigger rect = %v is the COLOR item rect — tapping Color would open rename", trigRect)
	}
	if trigRect.Overlaps(colorRect) {
		t.Errorf("rename trigger rect %v overlaps Color item rect %v — adjacent controls not isolated", trigRect, colorRect)
	}
}
