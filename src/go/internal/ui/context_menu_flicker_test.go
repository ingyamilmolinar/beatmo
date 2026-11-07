//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Opening the menu sets the suppression; on the next frame the still-held press lands outside the menu's rect and
// must not close it, or the menu flashes for a frame.
func TestContextMenuStaysOpenAfterKebabTap(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: roster.Snare,
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

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("no row kebab buttons after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab rect empty — layout may differ in this configuration")
	}
	lx := kebabRect.Min.X + kebabRect.Dx()/2
	ly := kebabRect.Min.Y + kebabRect.Dy()/2

	// drum.Update runs after the InputDispatcher each frame; this frame opens the menu and sets the suppression.
	resetFrame0 := SetInputForTest(
		func() (int, int) { return lx, ly },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	// SetInputForTest clears the flag on teardown, but the touch is still down between real frames, so it is restored.
	savedSuppress := suppressClicksUntilRelease
	resetFrame0()
	suppressClicksUntilRelease = savedSuppress
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	if !dv.IsContextMenuOpen() {
		t.Fatal("expected context menu to open after tapping row label")
	}

	// The dispatcher runs with the press still down at the label, outside the menu.
	result := dv.HandleInput(lx, ly, true)
	_ = result // result is informational; the key assertion is below

	if !dv.IsContextMenuOpen() {
		menuRect := dv.contextMenuRect
		t.Errorf(
			"context menu was closed by HandleInput(left=true) at kebab pos (%d,%d) — flicker bug\n"+
				"  kebab rect:   %v\n"+
				"  menu rect:    %v\n"+
				"  kebab in menu: %v\n"+
				"  Fix: guard 'click outside to close' with !suppressClicksUntilRelease",
			lx, ly, kebabRect, menuRect,
			image.Pt(lx, ly).In(menuRect),
		)
	}
}

// Once the opening touch is released, an outside tap still closes it.
func TestContextMenuClosesOnOutsideTapAfterSuppressClears(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 600
	bounds := image.Rect(0, 0, W, H)
	dv := NewDrumView(bounds, nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: roster.Snare,
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

	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("no row kebab buttons after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab rect empty")
	}
	lx := kebabRect.Min.X + kebabRect.Dx()/2
	ly := kebabRect.Min.Y + kebabRect.Dy()/2

	// Open the menu via Update() (tap the kebab).
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

	if !dv.IsContextMenuOpen() {
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

	// Now tap outside the menu rect — tree's click-outside closes it.
	outsideX := dv.contextMenuRect.Min.X + 5
	outsideY := dv.contextMenuRect.Min.Y - 20 // above the menu
	if outsideY < dv.Bounds.Min.Y {
		outsideY = dv.contextMenuRect.Max.Y + 5 // flip below
	}

	resetOutside := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	resetOutside()

	if dv.IsContextMenuOpen() {
		t.Errorf("context menu should close when tapping outside after suppression cleared (tap pos: %d,%d, menu rect: %v)",
			outsideX, outsideY, dv.contextMenuRect)
	}
}
