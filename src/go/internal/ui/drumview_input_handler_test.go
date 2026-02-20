//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newInputHandlerTestDV creates a DrumView suitable for HandleInput/HandleWheel tests.
// The returned DrumView is fully initialized (tree, zones, scroll) so all
// InputHandler methods are safe to call.
func newInputHandlerTestDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, game_log.New(testLogOutput(), game_log.LevelError))
	if dv == nil {
		t.Fatal("NewDrumView returned nil")
	}
	if dv.rowScroll() == nil {
		t.Fatal("rowScroll() nil — zones not initialized")
	}
	return dv
}

// TestDrumViewInput_TouchFlickerGuard verifies that when the mouse is released
// (!pressed) but the row scroll's TouchActive is still true, mouseDownInBounds
// stays true. This guards against touch-event flicker on mobile where a fast
// swipe can momentarily drop the touch for one frame.
func TestDrumViewInput_TouchFlickerGuard(t *testing.T) {
	dv := newInputHandlerTestDV(t)

	// Simulate an initial press inside bounds to set mouseDownInBounds.
	cx, cy := dv.Bounds.Min.X+10, dv.Bounds.Min.Y+10
	result := dv.HandleInput(cx, cy, true)
	if result != InputCaptured {
		t.Fatalf("initial press should capture; got %v", result)
	}
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be true after press inside bounds")
	}

	// Simulate touch-scroll active: set TouchScroller.active directly.
	dv.rowScroll().TS.active = true

	// Now release (pressed=false). Because TouchActive is true, the flicker
	// guard should prevent mouseDownInBounds from being cleared.
	result = dv.HandleInput(cx, cy, false)
	// Capturing() returns true because mouseDownInBounds is still true.
	if result != InputCaptured {
		t.Fatalf("release with TouchActive should still capture; got %v", result)
	}
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should remain true when TouchActive during release")
	}

	// Clear TouchActive and release again — now mouseDownInBounds should clear.
	dv.rowScroll().TS.active = false
	result = dv.HandleInput(cx, cy, false)
	if dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be cleared when TouchActive is false on release")
	}
	// With mouseDownInBounds cleared, hover inside bounds with !pressed → InputIgnored.
	if result != InputIgnored {
		t.Fatalf("release with no active state should be ignored; got %v", result)
	}
}

// TestDrumViewInput_CapturePersistedOutOfBounds verifies that once a press
// starts inside the DrumView (mouseDownInBounds=true), subsequent HandleInput
// calls with pressed=true return InputCaptured even when the cursor drifts
// outside the bounds. This prevents the splitter from stealing mid-drag.
func TestDrumViewInput_CapturePersistedOutOfBounds(t *testing.T) {
	dv := newInputHandlerTestDV(t)

	// Press inside bounds.
	cx, cy := dv.Bounds.Min.X+50, dv.Bounds.Min.Y+50
	result := dv.HandleInput(cx, cy, true)
	if result != InputCaptured {
		t.Fatalf("press inside should capture; got %v", result)
	}
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be true")
	}

	// Drag outside bounds — still pressed.
	ox, oy := dv.Bounds.Max.X+100, dv.Bounds.Max.Y+100
	result = dv.HandleInput(ox, oy, true)
	if result != InputCaptured {
		t.Fatalf("drag outside bounds with mouseDownInBounds should capture; got %v", result)
	}
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should persist during drag outside")
	}
}

// TestDrumViewInput_HandleWheelPassthrough verifies that HandleWheel returns
// InputIgnored for both in-bounds and out-of-bounds wheel events. The DrumView
// defers wheel handling to Update() rather than consuming it in HandleWheel.
func TestDrumViewInput_HandleWheelPassthrough(t *testing.T) {
	dv := newInputHandlerTestDV(t)

	// Wheel inside bounds with nonzero steps.
	cx, cy := dv.Bounds.Min.X+10, dv.Bounds.Min.Y+10
	result := dv.HandleWheel(cx, cy, 3)
	if result != InputIgnored {
		t.Fatalf("in-bounds wheel should return InputIgnored; got %v", result)
	}

	// Wheel outside bounds.
	ox, oy := dv.Bounds.Max.X+50, dv.Bounds.Max.Y+50
	result = dv.HandleWheel(ox, oy, -2)
	if result != InputIgnored {
		t.Fatalf("out-of-bounds wheel should return InputIgnored; got %v", result)
	}

	// Zero steps should also be ignored (early return).
	result = dv.HandleWheel(cx, cy, 0)
	if result != InputIgnored {
		t.Fatalf("zero-step wheel should return InputIgnored; got %v", result)
	}
}

// TestDrumViewInput_ReleaseWithActiveDrag verifies the interaction between
// anyDragActive() and the release path. When anyDragActive is true during
// release, mouseDownInBounds is preserved (drag still owns capture). When
// the drag ends, the next release clears mouseDownInBounds.
func TestDrumViewInput_ReleaseWithActiveDrag(t *testing.T) {
	dv := newInputHandlerTestDV(t)

	// Press inside bounds to set mouseDownInBounds.
	cx, cy := dv.Bounds.Min.X+10, dv.Bounds.Min.Y+10
	dv.HandleInput(cx, cy, true)
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be set after press")
	}

	// Simulate an active drag via the dragging field (used by anyDragActive).
	dv.dragging = true

	// Release: anyDragActive() is true, so mouseDownInBounds should NOT clear.
	result := dv.HandleInput(cx, cy, false)
	if !dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should persist when anyDragActive is true during release")
	}
	// Capturing() includes mouseDownInBounds and anyDragActive — should be captured.
	if result != InputCaptured {
		t.Fatalf("release with active drag should still capture; got %v", result)
	}

	// End the drag.
	dv.dragging = false

	// Next release: neither TouchActive nor anyDragActive — clears mouseDownInBounds.
	result = dv.HandleInput(cx, cy, false)
	if dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should clear after drag ends and release")
	}
	if result != InputIgnored {
		t.Fatalf("release with no active state should be ignored; got %v", result)
	}
}

// TestDrumViewInput_PressOutsideBoundsIgnored verifies that pressing outside
// the DrumView bounds returns InputIgnored and does NOT set mouseDownInBounds.
func TestDrumViewInput_PressOutsideBoundsIgnored(t *testing.T) {
	dv := newInputHandlerTestDV(t)

	// Verify mouseDownInBounds starts false.
	if dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should be false initially")
	}

	// Press well outside bounds.
	ox, oy := dv.Bounds.Max.X+200, dv.Bounds.Max.Y+200
	result := dv.HandleInput(ox, oy, true)
	if result != InputIgnored {
		t.Fatalf("press outside bounds should be ignored; got %v", result)
	}
	if dv.mouseDownInBounds {
		t.Fatal("mouseDownInBounds should remain false after press outside bounds")
	}

	// Also test a point just outside each edge.
	edges := []struct {
		name string
		x, y int
	}{
		{"left", dv.Bounds.Min.X - 1, dv.Bounds.Min.Y + 10},
		{"top", dv.Bounds.Min.X + 10, dv.Bounds.Min.Y - 1},
		{"right", dv.Bounds.Max.X, dv.Bounds.Min.Y + 10},   // Max.X is exclusive
		{"bottom", dv.Bounds.Min.X + 10, dv.Bounds.Max.Y},   // Max.Y is exclusive
	}
	for _, e := range edges {
		result = dv.HandleInput(e.x, e.y, true)
		if result != InputIgnored {
			t.Errorf("press at %s edge (%d,%d) should be ignored; got %v", e.name, e.x, e.y, result)
		}
		if dv.mouseDownInBounds {
			t.Errorf("mouseDownInBounds should be false after press at %s edge", e.name)
		}
	}
}
