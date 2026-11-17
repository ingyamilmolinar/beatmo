//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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

// TestNavMenuComponentClaimsOnlyTheWheelArrows replaces the go-to text
// editor's TOTAL keyboard claim: the inline go-to wheel is a PARTIAL claimant —
// it owns the arrows while the menu is open (so they step the wheel, not the
// camera) and nothing else (Space still plays). A closed menu claims nothing.
func TestNavMenuComponentClaimsOnlyTheWheelArrows(t *testing.T) {
	unit := true
	c := navMenuCompForTest(t, &unit)
	if _, total := any(c).(KeyboardClaimant); total {
		t.Fatal("the nav menu must not take the whole keyboard (no text entry left)")
	}
	keys := c.ClaimedKeys()
	if len(keys) == 0 {
		t.Fatal("an open nav menu must claim the go-to wheel's arrows")
	}
	for _, k := range keys {
		switch k {
		case ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyEscape:
			t.Fatalf("the wheel must not claim %v", k)
		}
	}
	c.Close()
	if len(c.ClaimedKeys()) != 0 {
		t.Fatal("a closed nav menu must claim no keys")
	}
}
