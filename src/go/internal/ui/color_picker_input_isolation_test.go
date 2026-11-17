//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// fireContextMenuItemByIcon finds the open context menu item whose leading icon
// matches `icon` and fires its onClick (mirrors a real tap landing on it).
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

// TestMobileColorPickerIsolatesInput verifies that once the color picker is open
// on mobile, input is isolated on the z-axis: there is no lingering native-input
// TRIGGER (e.g. the rename trigger from the context menu it replaced) overlapping
// the picker, and the picker owns a full-bounds hit region so taps inside it are
// consumed rather than passing through to controls beneath.
func TestMobileColorPickerIsolatesInput(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)

	// Open the context menu, then tap "Color" — this is the exact path the user
	// follows (overflow ⋯ → Color). It closes the context menu and opens the
	// swatch picker. dv.Update() (not recalcButtons alone) drives the full frame
	// — recalcButtons, rootTree.Update (z-order gating), and syncNativeGestures
	// (nativeInputCandidates → dv.lastNativeRects) — matching the real loop.
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

	// The picker must own a full-bounds input region (its portal hit area) so
	// taps inside it are consumed, not passed through to lower-z controls.
	if got := dv.colorWheelComp.InputBounds(); got != pickerRect {
		t.Errorf("color picker InputBounds = %v, want full panel %v", got, pickerRect)
	}

	// No native-input trigger may overlap the open picker — otherwise the JS
	// touchend handler would open a hidden text input behind the swatches.
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
