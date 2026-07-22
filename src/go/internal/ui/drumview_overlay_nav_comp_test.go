//go:build test

package ui

import (
	"image"
	"testing"
)

// navMenuCompForTest builds an opened NavigationMenuComponent backed by mutable
// unit/amount state so tests can drive HandleInput like the tree would.
func navMenuCompForTest(t *testing.T, unitBeats *bool) *NavigationMenuComponent {
	t.Helper()
	panel := &NavigationPanel{}
	panel.SetCallbacks(NavPanelCallbacks{
		JumpAmount:    func() int { return 4 },
		JumpUnitBeats: func() bool { return *unitBeats },
		OnToggleUnit:  func() { *unitBeats = !*unitBeats },
		OnPrev:        func() {},
		OnNext:        func() {},
	})
	c := NewNavigationMenuComponent(panel)
	c.SetScreenBounds(image.Rect(0, 0, 640, 480))
	c.SetAnchor(image.Rect(300, 240, 400, 264))
	c.Open()
	return c
}

// TestNavMenuCloseButton_DesktopVsMobile pins the "add missing close button"
// deliverable: the nav menu shows a desktop close × (a press on it closes the
// menu) and none on mobile, where tap-away is the dismiss. It routes through the
// same shared newPopupCloseButton helper every other pop-up uses.
func TestNavMenuCloseButton_DesktopVsMobile(t *testing.T) {
	// Desktop: close × present and wired to Close via HandleInput's press path.
	withSmallScreen(t, false)
	unit := true
	c := navMenuCompForTest(t, &unit)
	btn := c.CloseBtn()
	if btn == nil {
		t.Fatal("desktop nav menu must have a close ×")
	}
	if btn.Icon != string(IconClose) {
		t.Fatalf("close icon = %q; want %q", btn.Icon, string(IconClose))
	}
	r := btn.Rect()
	if r.Empty() {
		t.Fatal("close button rect is empty")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if res := c.HandleInput(cx, cy, true); res != InputConsumed {
		t.Fatalf("close press result = %v; want InputConsumed", res)
	}
	if c.IsOpen() {
		t.Fatal("nav menu still open after pressing the close ×")
	}

	// Mobile: no × — the bottom sheet dismisses via tap-away.
	withSmallScreen(t, true)
	unit2 := true
	c2 := navMenuCompForTest(t, &unit2)
	if b := c2.CloseBtn(); b != nil {
		t.Fatalf("mobile nav menu must have no close ×; got rect %+v", b.Rect())
	}
}

// TestNavMenuComponentIsCompOverlayable is the compile-time + runtime proof that
// the nav menu is a first-class portal component (compOverlayable), so it flows
// through compPortalOverlay/compHitHandler and the tree's native input dispatch.
func TestNavMenuComponentIsCompOverlayable(t *testing.T) {
	unit := true
	var _ compOverlayable = navMenuCompForTest(t, &unit)
}

// TestNavMenuComponentCapturesControlPress verifies a press on a nav control
// returns InputCaptured — the framework-native way to receive OnDrag/OnRelease
// (via compHitHandler) so momentary buttons pop back and the segment latch
// resets. No bespoke overflowNavCapturing flag is involved.
func TestNavMenuComponentCapturesControlPress(t *testing.T) {
	unit := true
	c := navMenuCompForTest(t, &unit)
	seg := c.Panel().unitSeg
	sx, sy := center(seg.SegmentRect(1)) // Subdiv
	if res := c.HandleInput(sx, sy, true); res != InputCaptured {
		t.Fatalf("control press returned %v, want InputCaptured (so the tree delivers release)", res)
	}
	if !c.Capturing() {
		t.Fatal("Capturing() must be true while a control press is in flight")
	}
	c.HandleInput(sx, sy, false) // release, delivered natively by the tree
	if c.Capturing() {
		t.Fatal("Capturing() must clear on release")
	}
}

// TestNavMenuComponentSegmentTogglesBothWays reproduces the "can't click Beats
// after Subdiv" bug through the component's HandleInput (press+release both
// ways). With native release delivery the segArmed latch resets each time.
func TestNavMenuComponentSegmentTogglesBothWays(t *testing.T) {
	unit := true // Beats
	c := navMenuCompForTest(t, &unit)
	seg := c.Panel().unitSeg

	sx, sy := center(seg.SegmentRect(1)) // Subdiv
	c.HandleInput(sx, sy, true)
	c.HandleInput(sx, sy, false)
	if unit {
		t.Fatal("press+release on Subdiv did not switch to subdiv")
	}
	c.rebuild() // re-sync active from source of truth (Layout runs on rebuild)

	bx, by := center(seg.SegmentRect(0)) // Beats
	c.HandleInput(bx, by, true)
	c.HandleInput(bx, by, false)
	if !unit {
		t.Fatal("press+release on Beats did not switch back (segment latch stuck)")
	}
}

// TestNavMenuComponentClaimsKeyboardWhenEditing verifies the owned go-to editor
// links the text input to the parent tree/keyboard: ClaimsKeyboard is true only
// while the editor is active, and the editor rect is inside InputBounds so a
// click in it is not treated as an outside-click.
func TestNavMenuComponentClaimsKeyboardWhenEditing(t *testing.T) {
	unit := true
	c := navMenuCompForTest(t, &unit)
	if c.ClaimsKeyboard() {
		t.Fatal("must not claim keyboard before editing")
	}
	c.Editor().OpenValue(ValueOpen{
		Spec:   positionValueSpec(8, 128),
		Anchor: c.Panel().GotoBeatRect(),
		Clamp:  c.InputBounds(),
		Get:    func() float64 { return 0 },
		Set:    func(v float64) {},
	})
	if !c.Editor().Active() {
		t.Fatal("editor did not activate")
	}
	if !c.ClaimsKeyboard() {
		t.Fatal("must claim keyboard while the editor is active")
	}
	er := c.Editor().Rect()
	if !er.Empty() && !er.In(c.InputBounds()) {
		t.Fatalf("editor rect %v must lie inside InputBounds %v", er, c.InputBounds())
	}
}
