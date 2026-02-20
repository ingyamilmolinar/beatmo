//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestVolumePopupClickOutsideCloses verifies that clicking outside a volume
// popup causes the portal to close it, and the sync in Update() closes the
// underlying SliderPopup.
func TestVolumePopupClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.tree == nil {
		t.Skip("tree not initialized")
	}
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect")
	}

	// Open the volume popup.
	dv.openVolumePopup(0)
	if !dv.volPopup.IsOpen() {
		t.Fatal("volPopup should be open after openVolumePopup")
	}
	if !dv.tree.Portal().Has("volume-popup") {
		t.Fatal("portal should have 'volume-popup' entry")
	}

	// Click far outside the popup (top-left corner).
	restore := SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Release frame.
	restore = SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Portal entry should be removed.
	if dv.tree.Portal().Has("volume-popup") {
		t.Fatal("portal should not have 'volume-popup' after click-outside")
	}

	// SliderPopup should also be closed via sync.
	if dv.volPopup.IsOpen() {
		t.Fatal("volPopup.IsOpen() should be false after click-outside dismissal")
	}
}

// TestMasterVolumePopupClickOutsideCloses verifies click-outside closes
// the master volume popup.
func TestMasterVolumePopupClickOutsideCloses(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.tree == nil {
		t.Skip("tree not initialized")
	}

	// Set a non-empty icon rect so the popup can open.
	dv.mainVolIconRect = image.Rect(10, 10, 30, 30)
	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("masterVolPopup should be open")
	}
	if !dv.tree.Portal().Has("master-volume-popup") {
		t.Fatal("portal should have 'master-volume-popup' entry")
	}

	// Click outside.
	restore := SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Release frame.
	restore = SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	if dv.tree.Portal().Has("master-volume-popup") {
		t.Fatal("portal should not have 'master-volume-popup' after click-outside")
	}
	if dv.masterVolPopup.IsOpen() {
		t.Fatal("masterVolPopup.IsOpen() should be false after click-outside")
	}
}

// TestSliderPopupClickOutsideUnblocksInput verifies that after a popup is
// dismissed via click-outside, the input guard at drumview_update.go:584
// no longer blocks.
func TestSliderPopupClickOutsideUnblocksInput(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.tree == nil {
		t.Skip("tree not initialized")
	}
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("rowVolSliders[0] has empty rect")
	}

	// Open the volume popup.
	dv.openVolumePopup(0)
	if !dv.volPopup.IsOpen() {
		t.Fatal("volPopup should be open")
	}

	// Verify the input guard blocks — the guard checks volPopup.IsOpen().
	blocked := dv.volPopup.IsOpen()
	if !blocked {
		t.Fatal("input guard should block while popup is open")
	}

	// Click outside to dismiss.
	restore := SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// Release.
	restore = SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	// After click-outside, the popup should be closed and input unblocked.
	if dv.volPopup.IsOpen() {
		t.Fatal("volPopup should be closed after click-outside")
	}
}
