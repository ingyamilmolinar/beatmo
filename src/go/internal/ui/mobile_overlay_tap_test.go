//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newMobileTestDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up frame for layout init.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	return dv
}

func mobileInputAt(t *testing.T, x, y int, pressed bool) func() {
	t.Helper()
	return SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
}

// Taps inside an open context menu reach its buttons; tap injection is not blocked by an open dropdown.
func TestContextMenuTapInjectionWithOpenMenu(t *testing.T) {
	dv := newMobileTestDV(t)

	// Open context menu for row 0.
	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	// Find the first menu button (e.g., "Rename").
	if len(dv.contextMenuBtns) == 0 {
		t.Fatal("no context menu buttons")
	}
	btnRect := dv.contextMenuBtns[0].Rect()
	if btnRect.Empty() {
		t.Fatal("first context menu button has empty rect")
	}
	bx := btnRect.Min.X + btnRect.Dx()/2
	by := btnRect.Min.Y + btnRect.Dy()/2

	// Opening sets the suppression; clear it to model a fresh touch after the finger lifts.
	suppressClicksUntilRelease = false

	// Called directly, as the portal dispatches it.
	r := mobileInputAt(t, bx, by, true)
	dv.handleContextMenuInput(bx, by, true)
	r()

	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("deferred tap should be active after press inside context menu")
	}

	// A mobile touch end reverts to (0,0); the active deferred tap still fires on release.
	r = mobileInputAt(t, 0, 0, false)
	handled := dv.handleContextMenuInput(0, 0, false)
	r()

	if !handled {
		t.Error("handleContextMenuInput should consume input when deferred tap is active on release")
	}
	// Rename closes the menu when it fires.
	if dv.contextMenuDeferredTap.Active() {
		t.Error("deferred tap should have fired on release")
	}
}

// A deferred tap on a button holds capture, so the release (even at 0,0) reaches the right overlay.
func TestContextMenuDeferredTapCapture(t *testing.T) {
	dv := newMobileTestDV(t)

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	// Get a button inside the menu.
	if len(dv.contextMenuBtns) < 2 {
		t.Fatal("need at least 2 context menu buttons")
	}
	// Use the "Rename" button (index 1) to avoid inst menu side effects.
	btnRect := dv.contextMenuBtns[1].Rect()
	bx := btnRect.Min.X + btnRect.Dx()/2
	by := btnRect.Min.Y + btnRect.Dy()/2

	suppressClicksUntilRelease = false

	// Called directly, as the portal dispatches it.
	r := mobileInputAt(t, bx, by, true)
	handled := dv.handleContextMenuInput(bx, by, true)
	r()

	if !handled {
		t.Error("expected handleContextMenuInput to consume press (DeferredTap started)")
	}
	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("context menu deferred tap should be active after press")
	}

	// (0,0) is outside the drum bounds, but the active deferred tap holds capture.
	r = mobileInputAt(t, 0, 0, false)
	dv.handleContextMenuInput(0, 0, false)
	r()

	if dv.contextMenuDeferredTap.Active() {
		t.Error("deferred tap should have fired on release")
	}
}

func TestOverflowMenuTapInjectionWithOpenMenu(t *testing.T) {
	dv := newMobileTestDV(t)

	// Open overflow menu.
	dv.openOverflowMenuPortal()
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Skip("overflow popup rect empty")
	}

	// On a row that routes through the shared MenuScroll deferred tap.
	px := popupRect.Min.X + popupRect.Dx()/2
	py := popupRect.Min.Y + 5

	suppressClicksUntilRelease = false

	// Called directly, as the portal dispatches it.
	r := mobileInputAt(t, px, py, true)
	handled := dv.handleOverflowMenuInput(px, py, true)
	r()

	// The shared MenuScroll now owns the overflow menu's deferred tap.
	if dv.overflowMenuScroll == nil || !dv.overflowMenuScroll.TapActive() {
		t.Fatal("overflow deferred tap should be active after press inside menu")
	}
	if !handled {
		t.Error("expected handleOverflowMenuInput to consume press")
	}

	// Release at (0,0).
	r = mobileInputAt(t, 0, 0, false)
	dv.handleOverflowMenuInput(0, 0, false)
	r()

	if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.TapActive() {
		t.Error("overflow deferred tap should have fired on release")
	}
}

// An outside tap closes the popup without clicking through.
func TestTapOutsideOpenPopupClosesIt(t *testing.T) {
	dv := newMobileTestDV(t)

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	suppressClicksUntilRelease = false

	outsideX := dv.Bounds.Min.X + 5
	outsideY := dv.Bounds.Max.Y - 5
	// Ensure it's outside the context menu rect.
	if image.Pt(outsideX, outsideY).In(dv.contextMenuRect) {
		outsideX = dv.Bounds.Max.X - 5
	}

	// Click-outside closing is handled by the tree. Go through Update.
	r := mobileInputAt(t, outsideX, outsideY, true)
	dv.Update()
	r()

	if dv.IsContextMenuOpen() {
		t.Error("context menu should be closed after tapping outside")
	}
}

// A long press cancels any active deferred tap, so lifting the finger fires nothing stale.
func TestLongPressCancelsDeferredTap(t *testing.T) {
	dv := newMobileTestDV(t)

	dv.OpenContextMenu(0)
	suppressClicksUntilRelease = false

	// Manually start a deferred tap (simulating a press inside the menu).
	dv.contextMenuDeferredTap.Begin(100, 100)
	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("deferred tap should be active")
	}

	// CancelAllDeferredTaps should clear it.
	dv.CancelAllDeferredTaps()

	if dv.contextMenuDeferredTap.Active() {
		t.Error("context menu deferred tap should be cancelled")
	}
	if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.TapActive() {
		t.Error("overflow deferred tap should be cancelled")
	}
	if dv.fxPanelDeferredTap.Active() {
		t.Error("fx panel deferred tap should be cancelled")
	}
}

func TestFXPanelPortalDispatch(t *testing.T) {
	dv := newMobileTestDV(t)

	if len(dv.rowFXBtns()) == 0 {
		t.Skip("no FX buttons")
	}

	// Verify portal/tree exists.
	if dv.tree == nil {
		t.Fatal("DrumViewTree should be initialized")
	}

	// Open the FX panel manually.
	dv.openFXPanelPortal()
	dv.fxPanelRow = 0
	dv.fxPanelRect = image.Rect(100, 100, 300, 400)

	// fxPanelOpen should report true.
	if !dv.IsFXPanelOpen() {
		t.Fatal("fxPanelOpen should be true")
	}

	suppressClicksUntilRelease = false

	// Press inside the FX panel rect via HandleInput (portal path).
	px := dv.fxPanelRect.Min.X + 10
	py := dv.fxPanelRect.Min.Y + 10
	r := mobileInputAt(t, px, py, true)
	result := dv.HandleInput(px, py, true)
	r()

	if result == InputIgnored {
		t.Error("HandleInput should consume input inside open FX panel via portal")
	}
}
