package ui

import (
	"image"
	"math"
	"testing"
)

// TestKnob_DragPixelsScalesWithRadius verifies that knob drag sensitivity
// scales with knob visual radius. On Spacious density (mobile), the knob
// diameter is ~88 px (radius ~44), so a full-range sweep requires ~176 px of
// finger travel — about 2× finer than the fixed-constant baseline (120 px).
// Small knobs (radius < 30) still use the 120 px baseline so desktop and
// Compact-density feel is unchanged.
func TestKnob_DragPixelsScalesWithRadius(t *testing.T) {
	// Spacious-density knob (88×88 → radius=44).
	k := NewKnob(0)
	k.SetRect(image.Rect(0, 0, 88, 88))
	k.HandleInputResult(44, 44, true) // press near centre
	// Drag 44 px right. With radius-scaled sweep of 176 px, expect value≈0.25
	// (44/176 = 0.25). Pre-fix this would have been 44/120 ≈ 0.367.
	k.HandleInputResult(88, 44, true)
	if k.Value < 0.22 || k.Value > 0.28 {
		t.Errorf("Spacious knob 44-px-right drag: Value=%v, want ~0.25 "+
			"(radius-scaled sweep should be ~176 px)", k.Value)
	}
	k.HandleInputResult(88, 44, false)

	// Compact-density knob (24×24 → radius=12). Below the radius*4 threshold,
	// so falls back to knobDragPixelsCoarse=120.
	k = NewKnob(0)
	k.SetRect(image.Rect(0, 0, 24, 24))
	k.HandleInputResult(12, 12, true)
	k.HandleInputResult(72, 12, true) // 60 px right
	if k.Value < 0.45 || k.Value > 0.55 {
		t.Errorf("Compact knob 60-px-right drag: Value=%v, want ~0.5 "+
			"(small knob should use 120 px baseline)", k.Value)
	}
	k.HandleInputResult(72, 12, false)
}

// TestKnob_ReleaseDoesNotRemutateOnStaleCoords reproduces the mobile
// touch-release bug: when the dispatcher hands OnRelease coordinates that
// are NOT the last drag coordinates (e.g., the pre-fix touch override
// deactivated mid-frame and the dispatcher read (0,0) from the underlying
// cursor), the knob must NOT recompute its value from those stale coords.
//
// Pre-fix behaviour: HandleInputResult(0, 0, false) called updateFromDrag(0,0),
// producing a huge upward delta from (pressX, pressY)=(300,400) and clamping
// the value to 1.0 (max).
//
// Post-fix behaviour: release is treated as a commit of the drag-end value.
// The knob clears `dragging` and leaves Value untouched.
//
// Note: this test does NOT drive globalTouchState — it isolates the knob's
// own contract. The end-to-end touch lifecycle is covered by
// synth_knob_touch_lifecycle_test.go and the browser test.
func TestKnob_ReleaseDoesNotRemutateOnStaleCoords(t *testing.T) {
	k := NewKnob(0.5)
	// Large rect so the press point (300, 400) is inside.
	k.SetRect(image.Rect(200, 300, 500, 600))

	// Press at (300, 400) — latches pressVal=0.5.
	if got := k.HandleInputResult(300, 400, true); got != InputCaptured {
		t.Fatalf("press: got %v want InputCaptured", got)
	}

	// Drag right 30 px → value rises by 30/knobDragPixels for the knob's radius.
	// Whatever the exact post-Change-4 sensitivity is, it must be a real
	// fractional value between 0.5 and 1.0 (drag right = increase).
	k.HandleInputResult(330, 400, true)
	dragEndValue := k.Value
	if dragEndValue <= 0.5 || dragEndValue >= 1.0 {
		t.Fatalf("after drag: Value=%v, want strictly between 0.5 and 1.0", dragEndValue)
	}

	// Simulate the buggy stale-coord release: dispatcher hands (0, 0)
	// because the touch override deactivated on the release frame.
	if got := k.HandleInputResult(0, 0, false); got != InputConsumed {
		t.Fatalf("release: got %v want InputConsumed", got)
	}
	if k.Capturing() {
		t.Error("Capturing() should be false after release")
	}

	// The bug is "value snaps to max on release". After the fix, Value must
	// equal the drag-end value, NOT 1.0.
	if math.Abs(k.Value-dragEndValue) > 1e-9 {
		t.Errorf("after stale-coord release: Value=%v, want unchanged %v "+
			"(pre-fix this snapped to ~1.0 because updateFromDrag(0,0) "+
			"computed a huge upward delta)", k.Value, dragEndValue)
	}
	if k.Value >= 1.0 {
		t.Errorf("regression: Value=%v reached max on stale-coord release — "+
			"the original mobile bug is back", k.Value)
	}
}

// TestKnob_ReleaseDoesNotRemutateOnDownwardStaleCoords mirrors the test above
// for a press in the upper portion of the screen where a (0, 0) stale release
// would push the value DOWN to 0 rather than up to 1. Covers the second-axis
// failure mode.
func TestKnob_ReleaseDoesNotRemutateOnDownwardStaleCoords(t *testing.T) {
	k := NewKnob(0.5)
	// Press near the top-left so (0,0) release looks like "left/up" drag.
	k.SetRect(image.Rect(50, 50, 150, 150))

	if got := k.HandleInputResult(100, 80, true); got != InputCaptured {
		t.Fatalf("press: got %v want InputCaptured", got)
	}
	// Drag left 20 px → value drops below 0.5 but stays > 0.
	k.HandleInputResult(80, 80, true)
	dragEndValue := k.Value
	if dragEndValue >= 0.5 || dragEndValue <= 0.0 {
		t.Fatalf("after leftward drag: Value=%v, want strictly between 0 and 0.5", dragEndValue)
	}

	// Buggy stale release: (0,0). Press was at (100, 80), so (0,0) reads
	// as "left and up by similar amounts"; with absDx==absDy ties going
	// to vertical, that's dy=+80 → pressVal + 80/pixels.
	k.HandleInputResult(0, 0, false)

	if math.Abs(k.Value-dragEndValue) > 1e-9 {
		t.Errorf("after stale-coord release: Value=%v, want unchanged %v",
			k.Value, dragEndValue)
	}
}
