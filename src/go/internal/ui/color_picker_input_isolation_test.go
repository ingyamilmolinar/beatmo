//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// As a real tap landing on it.
func fireContextMenuItemByIcon(t *testing.T, dv *DrumView, row int, icon string) {
	t.Helper()
	for _, it := range dv.ContextMenuItemsForTest(row) {
		if it.icon == icon {
			if it.onClick != nil {
				it.onClick()
			}
			return
		}
	}
	t.Fatalf("context menu item with icon %q not found", icon)
}

// No native-input trigger left over from the context menu overlaps the open picker, and the picker owns a
// full-bounds hit region, so taps inside it never pass through.
func TestMobileColorPickerIsolatesInput(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)

	// The user's path (⋯ → Color) through a full dv.Update, which also syncs the native gestures.
	dv.OpenContextMenu(0)
	dv.Update()
	fireContextMenuItemByIcon(t, dv, 0, "color")
	dv.Update()

	if !dv.IsColorMenuOpen() {
		t.Fatal("color picker should be open after tapping Color")
	}
	if dv.IsContextMenuOpen() {
		t.Fatal("context menu should be closed once the color picker opens")
	}

	pickerRect := dv.colorWheelComp.WheelRect()
	if pickerRect.Empty() {
		t.Fatal("color picker bounds empty — no hit region to isolate input")
	}

	// Its portal hit area covers it, so taps are consumed.
	if got := dv.colorWheelComp.InputBounds(); got != pickerRect {
		t.Errorf("color picker InputBounds = %v, want full panel %v", got, pickerRect)
	}

	// Otherwise the JS touchend handler opens a hidden text input behind the swatches.
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel != NativeTextInput {
			continue
		}
		if r.Rect.Overlaps(pickerRect) {
			t.Errorf("native-input trigger %q rect %v overlaps open color picker %v — input not isolated", r.Intent.ID, r.Rect, pickerRect)
		}
		// In particular the rename trigger from the dismissed context menu must be gone.
		if strings.HasPrefix(r.Intent.ID, "rename-") {
			t.Errorf("stale rename trigger %q still armed while color picker is open", r.Intent.ID)
		}
	}
}
