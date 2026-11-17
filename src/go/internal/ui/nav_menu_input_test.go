//go:build test

package ui

import (
	"image"
	"testing"
)

// navPanelForTest builds a laid-out NavigationPanel backed by a mutable unit
// flag + jump amount, so tests can drive HandleInput and observe toggles.
func navPanelForTest(unitBeats *bool, amount *int, prev, next *int) *NavigationPanel {
	p := &NavigationPanel{}
	p.SetCallbacks(NavPanelCallbacks{
		JumpAmount:    func() int { return *amount },
		JumpUnitBeats: func() bool { return *unitBeats },
		OnToggleUnit:  func() { *unitBeats = !*unitBeats },
		OnPrev:        func() { *prev++ },
		OnNext:        func() { *next++ },
	})
	p.Layout(image.Rect(0, 0, 240, 5*touchMinTargetPx))
	return p
}

func center(r image.Rectangle) (int, int) { return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2 }

// TestNavPanelSegmentTogglesBothWays reproduces the "can't click Beats after
// Subdiv" bug: a full press+release on Subdiv must switch to subdiv, and a
// following press+release on Beats must switch back. The segArmed latch must
// reset on the release edge so the second tap fires.
func TestNavPanelSegmentTogglesBothWays(t *testing.T) {
	unit := true // start on Beats
	amount, prev, next := 4, 0, 0
	p := navPanelForTest(&unit, &amount, &prev, &next)

	sx, sy := center(p.unitSeg.SegmentRect(1)) // Subdiv
	p.HandleInput(sx, sy, true)
	p.HandleInput(sx, sy, false)
	if unit {
		t.Fatal("press+release on Subdiv did not switch unit to subdiv")
	}
	p.Layout(image.Rect(0, 0, 240, 5*touchMinTargetPx)) // re-sync active from source of truth

	bx, by := center(p.unitSeg.SegmentRect(0)) // Beats
	p.HandleInput(bx, by, true)
	p.HandleInput(bx, by, false)
	if !unit {
		t.Fatal("press+release on Beats did not switch unit back to beats (segArmed latch stuck)")
	}
}

// TestNavPanelSegmentNoDoubleFireWhileHeld verifies a HELD press (pressed==true
// re-delivered every frame) fires the segment toggle exactly once, not on every
// frame — the segArmed latch's original purpose.
func TestNavPanelSegmentNoDoubleFireWhileHeld(t *testing.T) {
	unit := true
	amount, prev, next := 4, 0, 0
	p := navPanelForTest(&unit, &amount, &prev, &next)

	sx, sy := center(p.unitSeg.SegmentRect(1)) // Subdiv
	p.HandleInput(sx, sy, true)
	p.HandleInput(sx, sy, true) // held — must NOT re-toggle
	p.HandleInput(sx, sy, true)
	if unit {
		t.Fatal("held press never fired the toggle")
	}
	// unit flipped once (true->false). A double-fire would flip it back to true.
	if unit != false {
		t.Fatalf("held press double-fired the toggle; unit=%v want false", unit)
	}
}

// TestNavPanelJogButtonMomentary verifies the prev/next jog buttons are
// momentary: OnPrev fires once on the press edge, and the button is NOT left
// stuck in the pressed state after the release edge (the "buttons toggle" bug).
func TestNavPanelJogButtonMomentary(t *testing.T) {
	unit := true
	amount, prev, next := 4, 0, 0
	p := navPanelForTest(&unit, &amount, &prev, &next)

	px, py := center(p.prevBtn.Rect())
	p.HandleInput(px, py, true)
	if prev != 1 {
		t.Fatalf("prev press fired OnPrev %d times, want 1", prev)
	}
	p.HandleInput(px, py, false) // release edge
	if p.prevBtn.pressed {
		t.Fatal("prev jog button stuck in pressed state after release (looks toggled)")
	}
	if prev != 1 {
		t.Fatalf("OnPrev fired again on release; count=%d want 1", prev)
	}
}

// The bespoke overflowNavCapturing integration guard was removed: the nav menu
// is now a first-class portal component (NavigationMenuComponent), so release
// delivery is handled natively by the tree's compHitHandler. The replacement
// guards live in drumview_overlay_nav_comp_test.go
// (TestNavMenuComponentCapturesControlPress / SegmentTogglesBothWays).
