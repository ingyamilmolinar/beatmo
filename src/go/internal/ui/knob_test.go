package ui

import (
	"image"
	"testing"
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
