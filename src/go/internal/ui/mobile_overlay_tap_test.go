//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newMobileTestDV creates a DrumView configured for mobile testing with
// small screen enabled and layout initialized.
func newMobileTestDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
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

// mobileInputAt sets input mocks at the given position with given press state.
// Returns a restore function.
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

// TestContextMenuTapInjectionWithOpenMenu verifies that removing the
// anyDropdownOpen() guard from tap injection allows taps inside an open
// context menu to fire buttons.
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

	// Suppress should be set from openContextMenu — clear it to simulate
	// a new touch after the open (user lifts finger, then taps again).
	suppressClicksUntilRelease = false

	// Frame 1: press inside the context menu at a button position.
	// The portal system now handles context menu input dispatch. Call
	// handleContextMenuInput directly to simulate the portal path.
	r := mobileInputAt(t, bx, by, true)
	dv.handleContextMenuInput(bx, by, true)
	r()

	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("deferred tap should be active after press inside context menu")
	}

	// Frame 2: release at (0,0) — simulating mobile touch end where coords
	// revert to origin. With portal capture, the context menu input handler
	// still fires on release because the deferred tap is active.
	r = mobileInputAt(t, 0, 0, false)
	handled := dv.handleContextMenuInput(0, 0, false)
	r()

	if !handled {
		t.Error("handleContextMenuInput should consume input when deferred tap is active on release")
	}
	// The deferred tap should have fired and closed the menu (the Rename
	// button closes the context menu when fired).
	if dv.contextMenuDeferredTap.Active() {
		t.Error("deferred tap should have fired on release")
	}
}

// TestContextMenuDeferredTapCapture verifies that when a DeferredTap is
// started on a context menu button, the portal system establishes capture
// so that the release frame (even at coords 0,0) routes to the correct
// overlay and fires the button.
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

	// Press inside menu -> portal dispatches to context menu overlay.
	// Call handleContextMenuInput directly to simulate the portal path.
	r := mobileInputAt(t, bx, by, true)
	handled := dv.handleContextMenuInput(bx, by, true)
	r()

	if !handled {
		t.Error("expected handleContextMenuInput to consume press (DeferredTap started)")
	}
	if !dv.contextMenuDeferredTap.Active() {
		t.Fatal("context menu deferred tap should be active after press")
	}

	// Release at (0,0) -- portal capture routes to context menu overlay
	// even though (0,0) is outside drum bounds because deferred tap is active.
	r = mobileInputAt(t, 0, 0, false)
	dv.handleContextMenuInput(0, 0, false)
	r()

	if dv.contextMenuDeferredTap.Active() {
		t.Error("deferred tap should have fired on release")
	}
}

// TestOverflowMenuTapInjectionWithOpenMenu verifies the same fix works for
// the overflow menu (Upload/Import/Export).
func TestOverflowMenuTapInjectionWithOpenMenu(t *testing.T) {
	dv := newMobileTestDV(t)

	// Open overflow menu.
	dv.openOverflowMenuPortal()
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		t.Skip("overflow popup rect empty")
	}

	// Get a position inside the popup.
	px := popupRect.Min.X + popupRect.Dx()/2
	py := popupRect.Min.Y + 5

	suppressClicksUntilRelease = false

	// Press inside the overflow menu. The portal system now handles
	// overflow menu input dispatch. Call handleOverflowMenuInput directly.
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

// TestTapOutsideOpenPopupClosesIt verifies that tapping outside an open
// popup closes it without click-through, confirming the Phase 1 change
// (removing anyDropdownOpen guard) is safe.
func TestTapOutsideOpenPopupClosesIt(t *testing.T) {
	dv := newMobileTestDV(t)

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	suppressClicksUntilRelease = false

	// Tap outside the context menu rect. Choose a point inside drum bounds
	// but clearly outside the menu.
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

// TestLongPressCancelsDeferredTap verifies that a GestureLongPress event
// cancels any active DeferredTap on the DrumView, preventing stale taps
// from firing when the finger lifts after a long press.
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

// TestFXPanelPortalDispatch verifies that the FX panel is dispatched via
// the portal system and responds to taps inside the panel rect.
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
