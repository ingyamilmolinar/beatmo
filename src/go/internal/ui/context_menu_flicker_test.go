//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestContextMenuStaysOpenAfterLabelTap reproduces the kebab-popup flicker bug.
//
// Root cause: on mobile the row label button opens the context menu inside
// drum.Update() (which runs AFTER the InputDispatcher each frame). On the very
// next frame the InputDispatcher dispatches to handleContextMenuInput() with
// left=true at the label position. The label is NOT inside the context-menu
// bottom-sheet rect, so the "click outside to close" check fires and immediately
// closes the menu — even though suppressClicksUntilRelease=true was set by
// openContextMenu(). The visible result is a 1-frame flash.
//
// The fix: honor suppressClicksUntilRelease in the "click outside to close"
// path of handleContextMenuInput, the same way DeferredTap.Begin() does.
func TestContextMenuStaysOpenAfterLabelTap(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: "snare",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame so layout is fully initialised.
	resetWarm := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetWarm()

	if len(dv.rowLabels) == 0 {
		t.Fatal("no row labels after warm-up")
	}
	labelRect := dv.rowLabels[0].Rect()
	if labelRect.Empty() {
		t.Skip("label rect empty — layout may differ in this configuration")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// ── Frame 0: touch down at label position (touch override active) ──
	// drum.Update() fires AFTER InputDispatcher each real frame.
	// We call Update() directly to simulate the drum-update half of the frame.
	// This should open the context menu and set suppressClicksUntilRelease.
	resetFrame0 := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	// Save the suppression state openContextMenu() set, then tear down the
	// input mock. SetInputForTest clears suppressClicksUntilRelease on both
	// call and teardown, but in the real app the touch hasn't been released so
	// the flag stays true between frames. We restore it after the reset to
	// accurately model the inter-frame state.
	savedSuppress := suppressClicksUntilRelease
	resetFrame0()
	suppressClicksUntilRelease = savedSuppress
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	if !dv.contextMenuOpen {
		t.Fatal("expected context menu to open after tapping row label")
	}

	// ── Frame 1: InputDispatcher runs with left=true at same label position ──
	// suppressClicksUntilRelease is still true (touch not released yet).
	// The dispatcher calls dv.HandleInput → OverlayStack.HandleInput.
	// The label position is NOT inside the context-menu bottom-sheet rect
	// (it opened below the label), so the "click outside to close" logic fires.
	//
	// Expected: menu stays open (OverlayStack guards on suppressClicksUntilRelease).
	// Actual before fix: contextMenuOpen = false (menu closes = 1-frame flicker).
	result := dv.HandleInput(lx, ly, true)
	_ = result // result is informational; the key assertion is below

	if !dv.contextMenuOpen {
		menuRect := dv.contextMenuRect
		t.Errorf(
			"context menu was closed by HandleInput(left=true) at label pos (%d,%d) — flicker bug\n"+
				"  label rect:   %v\n"+
				"  menu rect:    %v\n"+
				"  label in menu: %v\n"+
				"  Fix: guard 'click outside to close' with !suppressClicksUntilRelease",
			lx, ly, labelRect, menuRect,
			image.Pt(lx, ly).In(menuRect),
		)
	}
}

// TestContextMenuClosesOnOutsideTapAfterSuppressClears verifies that the menu
// DOES close when the user taps outside AFTER the suppression window ends
// (i.e., once the touch that opened the menu has been released).
// This ensures the fix doesn't break the normal "tap outside to close" flow.
func TestContextMenuClosesOnOutsideTapAfterSuppressClears(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: "snare",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up.
	resetWarm := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetWarm()

	if len(dv.rowLabels) == 0 {
		t.Fatal("no row labels after warm-up")
	}
	labelRect := dv.rowLabels[0].Rect()
	if labelRect.Empty() {
		t.Skip("label rect empty")
	}
	lx := labelRect.Min.X + labelRect.Dx()/2
	ly := labelRect.Min.Y + labelRect.Dy()/2

	// Open the menu via Update().
	resetOpen := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOpen()

	if !dv.contextMenuOpen {
		t.Fatal("context menu must be open before testing close")
	}

	// Simulate touch release — clears suppressClicksUntilRelease.
	resetRelease := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return false }, // left released
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update() // clears suppressClicksUntilRelease (left=false)
	resetRelease()

	// Now tap outside the menu rect — should close.
	outsideX := dv.contextMenuRect.Min.X + 5
	outsideY := dv.contextMenuRect.Min.Y - 20 // above the menu
	if outsideY < dv.Bounds.Min.Y {
		outsideY = dv.contextMenuRect.Max.Y + 5 // flip below
	}

	dv.HandleInput(outsideX, outsideY, true)

	if dv.contextMenuOpen {
		t.Errorf("context menu should close when tapping outside after suppression cleared (tap pos: %d,%d, menu rect: %v)",
			outsideX, outsideY, dv.contextMenuRect)
	}
}
