package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestKnobInitialState verifies NewKnob clamps to [0,1] and SetRect / Rect
// round-trip the bounding rectangle.
func TestKnobInitialState(t *testing.T) {
	k := NewKnob(0.5)
	if k.Value != 0.5 {
		t.Errorf("Value=%v want 0.5", k.Value)
	}
	if got := NewKnob(-1).Value; got != 0 {
		t.Errorf("NewKnob(-1).Value=%v want 0 (clamp)", got)
	}
	if got := NewKnob(2).Value; got != 1 {
		t.Errorf("NewKnob(2).Value=%v want 1 (clamp)", got)
	}
	r := image.Rect(10, 20, 60, 80)
	k.SetRect(r)
	if k.Rect() != r {
		t.Errorf("Rect()=%v want %v", k.Rect(), r)
	}
}

// TestKnobPressInsideRectStartsDrag verifies a press inside the rect starts
// a drag (InputCaptured) and a press outside is ignored.
func TestKnobPressInsideRectStartsDrag(t *testing.T) {
	k := NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))
	if got := k.HandleInputResult(30, 30, true); got != InputCaptured {
		t.Errorf("press inside: got %v want InputCaptured", got)
	}
	if !k.Capturing() {
		t.Error("Capturing() should be true mid-drag")
	}
	// Release with no movement → InputConsumed, drag ends.
	if got := k.HandleInputResult(30, 30, false); got != InputConsumed {
		t.Errorf("release: got %v want InputConsumed", got)
	}
	if k.Capturing() {
		t.Error("Capturing() should be false after release")
	}

	// Press outside the rect is ignored.
	k.SetRect(image.Rect(0, 0, 60, 60))
	if got := k.HandleInputResult(100, 100, true); got != InputIgnored {
		t.Errorf("press outside: got %v want InputIgnored", got)
	}
}

// TestKnobVerticalDragDoesNotChangeValue verifies the knob is horizontal-only:
// pure up/down motion leaves the value untouched so it never fights the panel's
// vertical scroll.
func TestKnobVerticalDragDoesNotChangeValue(t *testing.T) {
	k := NewKnob(0.4)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true) // press at x=30
	// Move 60 px up (x unchanged) → no value change.
	k.HandleInputResult(30, -30, true)
	if k.Value != 0.4 {
		t.Errorf("after 60-px-up drag: Value=%v want 0.4 (vertical must not change value)", k.Value)
	}
	// Move 200 px down (x unchanged) → still no change.
	k.HandleInputResult(30, 230, true)
	if k.Value != 0.4 {
		t.Errorf("after 200-px-down drag: Value=%v want 0.4 (vertical must not change value)", k.Value)
	}
	k.HandleInputResult(30, 0, false)
}

// TestKnobHorizontalDragSweepAndClamp verifies coarse-mode horizontal
// sensitivity: a full knobDragPixelsCoarse rightward sweep covers the 0→1 range
// and over-drag clamps.
func TestKnobHorizontalDragSweepAndClamp(t *testing.T) {
	k := NewKnob(0)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true) // press; pressX=30, pressVal=0
	// Move 60 px right → +0.5 in coarse mode (120 px sweep).
	k.HandleInputResult(90, 30, true)
	if k.Value < 0.49 || k.Value > 0.51 {
		t.Errorf("after 60-px-right drag: Value=%v want ~0.5", k.Value)
	}
	// Move further right (150 px total) → clamps at 1.0.
	k.HandleInputResult(180, 30, true)
	if k.Value != 1 {
		t.Errorf("clamp high: Value=%v want 1.0", k.Value)
	}
	// Move way left → clamps at 0.
	k.HandleInputResult(-300, 30, true)
	if k.Value != 0 {
		t.Errorf("clamp low: Value=%v want 0.0", k.Value)
	}
	k.HandleInputResult(0, 30, false) // release
}

// TestKnobFineDragIsLessSensitive verifies FineDrag triples the
// pixels-per-sweep, so the same horizontal movement produces a smaller value
// delta when fine mode is engaged.
func TestKnobFineDragIsLessSensitive(t *testing.T) {
	k := NewKnob(0.5)
	k.FineDrag = true
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true) // press
	// Coarse mode = 120 px/sweep → 60 px = 0.5 change. Fine mode = 360 px
	// → 60 px = 0.167 change. Expect Value ≈ 0.667.
	k.HandleInputResult(90, 30, true)
	if k.Value < 0.65 || k.Value > 0.69 {
		t.Errorf("fine drag 60-px-right: Value=%v want ~0.667", k.Value)
	}
	k.HandleInputResult(90, 30, false)
}

// TestKnobDragSequenceIsAbsoluteFromPressPoint verifies that two
// equal-distance drags from the press point produce identical value
// outcomes — the knob latches the press's initial value and never
// accumulates frame-to-frame deltas (which would drift under jitter).
func TestKnobDragSequenceIsAbsoluteFromPressPoint(t *testing.T) {
	k := NewKnob(0.3)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true)
	k.HandleInputResult(54, 30, true) // 24 px right
	first := k.Value
	// Drag to a different intermediate point …
	k.HandleInputResult(42, 30, true)
	// … then back to the same target. Value should match `first`.
	k.HandleInputResult(54, 30, true)
	if k.Value != first {
		t.Errorf("absolute-from-press invariant broken: first=%v, after re-drag=%v", first, k.Value)
	}
	k.HandleInputResult(30, 30, false)
}

// TestKnobReleaseWithoutPriorPressIsIgnored verifies a stray release event
// (no preceding press) is ignored — the knob does not enter a drag.
func TestKnobReleaseWithoutPriorPressIsIgnored(t *testing.T) {
	k := NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))
	if got := k.HandleInputResult(30, 30, false); got != InputIgnored {
		t.Errorf("release without press: got %v want InputIgnored", got)
	}
	if k.Capturing() {
		t.Error("Capturing() should be false after stray release")
	}
}

// TestKnobDrawDoesNotPanicOnEmptyRect verifies Draw is safe for a knob
// with zero-size or tiny bounds (early-return rather than divide-by-zero).
func TestKnobDrawDoesNotPanicOnEmptyRect(t *testing.T) {
	dst := ebiten.NewImage(2, 2)
	k := NewKnob(0.5)
	// Empty rect.
	k.Draw(dst)
	// Tiny rect (radius < 4 → early return).
	k.SetRect(image.Rect(0, 0, 4, 4))
	k.Draw(dst)
}

// TestKnobHorizontalDragChangesValue verifies a press + RIGHT drag also
// increases the value (most users instinctively try both axes; vertical-
// only would feel unresponsive on a horizontal drag).
func TestKnobHorizontalDragChangesValue(t *testing.T) {
	k := NewKnob(0)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true)
	// Move 60 px right → +0.5 value delta (coarse 120 px/sweep).
	k.HandleInputResult(90, 30, true)
	if k.Value < 0.45 || k.Value > 0.55 {
		t.Errorf("after 60-px-right drag: Value=%v want ~0.5", k.Value)
	}
	k.HandleInputResult(90, 30, false)

	k = NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true)
	// Move 60 px LEFT → -0.5 value delta.
	k.HandleInputResult(-30, 30, true)
	if k.Value < 0 || k.Value > 0.05 {
		t.Errorf("after 60-px-left drag: Value=%v want ~0.0", k.Value)
	}
	k.HandleInputResult(-30, 30, false)
}

// TestKnobDragUsesHorizontalComponentOnly verifies that only the horizontal
// displacement drives the value — vertical movement in a diagonal drag is
// ignored entirely.
func TestKnobDragUsesHorizontalComponentOnly(t *testing.T) {
	// Big vertical, tiny horizontal → only the 12 px right counts (+0.1).
	k := NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleInputResult(30, 30, true)
	k.HandleInputResult(42, -90, true) // 12 px right, 120 px up
	if k.Value < 0.59 || k.Value > 0.61 {
		t.Errorf("diagonal drag: Value=%v want ~0.6 (horizontal 12px only, vertical ignored)", k.Value)
	}
	k.HandleInputResult(42, -90, false)

	// Same horizontal displacement, opposite/zero vertical → identical result.
	k2 := NewKnob(0.5)
	k2.SetRect(image.Rect(0, 0, 60, 60))
	k2.HandleInputResult(30, 30, true)
	k2.HandleInputResult(42, 200, true) // 12 px right, 170 px down
	if k2.Value != k.Value {
		t.Errorf("vertical direction affected value: up-diagonal=%v down-diagonal=%v", k.Value, k2.Value)
	}
	k2.HandleInputResult(42, 200, false)
}

// TestKnobWheelChangesValue verifies mouse-wheel notches adjust the value.
func TestKnobWheelChangesValue(t *testing.T) {
	k := NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))

	// Wheel +1 inside rect → value increases by knobWheelStep (0.025).
	if got := k.HandleWheel(30, 30, 1); got != InputConsumed {
		t.Errorf("wheel inside rect: got %v want InputConsumed", got)
	}
	if k.Value < 0.524 || k.Value > 0.526 {
		t.Errorf("after wheel +1: Value=%v want ~0.525", k.Value)
	}

	// Wheel +10 → +0.25 (10 notches × 0.025).
	for i := 0; i < 10; i++ {
		k.HandleWheel(30, 30, 1)
	}
	if k.Value < 0.77 || k.Value > 0.78 {
		t.Errorf("after wheel +10 (extra): Value=%v want ~0.775", k.Value)
	}

	// Wheel outside rect → InputIgnored, no value change.
	v := k.Value
	if got := k.HandleWheel(200, 200, 5); got != InputIgnored {
		t.Errorf("wheel outside rect: got %v want InputIgnored", got)
	}
	if k.Value != v {
		t.Errorf("wheel outside rect changed value (was %v now %v)", v, k.Value)
	}
}

// TestKnobWheelFineMode verifies FineDrag shrinks the per-notch delta.
func TestKnobWheelFineMode(t *testing.T) {
	k := NewKnob(0.5)
	k.FineDrag = true
	k.SetRect(image.Rect(0, 0, 60, 60))
	k.HandleWheel(30, 30, 1)
	if k.Value < 0.5045 || k.Value > 0.5055 {
		t.Errorf("fine wheel +1: Value=%v want ~0.505 (knobWheelStepFine=0.005)", k.Value)
	}
}

// TestKnobDrawAtTypicalSize verifies the draw path completes for a
// realistic synth-tab knob size (44×60: knob + caption space).
func TestKnobDrawAtTypicalSize(t *testing.T) {
	dst := ebiten.NewImage(64, 80)
	k := NewKnob(0.7)
	k.SetRect(image.Rect(10, 10, 54, 70))
	k.Draw(dst)
	// Drag-state visual: pure-white indicator. Just ensure it doesn't panic.
	k.HandleInputResult(32, 30, true)
	k.Draw(dst)
	k.HandleInputResult(32, 30, false)
}
