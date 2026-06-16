package ui

import (
	"image"
	"testing"
)

func newEndlessKnob() *Knob {
	k := NewKnob(0) // value 0 => real == Min
	k.SetRect(image.Rect(0, 0, 100, 100))
	k.Scale = KnobScale{Min: 20, Max: 20000, Unit: "Hz"}
	k.Endless = true
	k.StepMul = 100 // 100 Hz per notch
	return k
}

func realOf(k *Knob) float64 {
	return k.Scale.Min + k.Value*(k.Scale.Max-k.Scale.Min)
}

func TestEndlessKnobAccumulatesInRealUnits(t *testing.T) {
	k := newEndlessKnob()
	k.HandleInputResult(50, 50, true) // press at x=50, real=20
	k.HandleInputResult(58, 50, true) // +8px = 1 notch = +100Hz
	if got := realOf(k); got < 119.999 || got > 120.001 {
		t.Fatalf("after +1 notch real=%v want ~120", got)
	}
	k.HandleInputResult(138, 50, true) // +80px = 10 notches = +1000Hz
	if got := realOf(k); got < 1119.99 || got > 1120.01 {
		t.Fatalf("after +11 notches real=%v want ~1120", got)
	}
}

func TestEndlessKnobClampsThenReversesImmediately(t *testing.T) {
	k := newEndlessKnob()
	k.HandleInputResult(50, 50, true)
	k.HandleInputResult(-5000, 50, true) // far left => clamp at Min
	if realOf(k) > 20.0001 {
		t.Fatalf("expected clamp at Min, got %v", realOf(k))
	}
	k.HandleInputResult(-4992, 50, true) // +8px => +100Hz, no dead zone
	if got := realOf(k); got < 119.99 || got > 120.01 {
		t.Fatalf("reverse-off-clamp real=%v want ~120", got)
	}
}

func TestEndlessKnobWheelStepsByStepMul(t *testing.T) {
	k := newEndlessKnob()
	if r := k.HandleWheel(50, 50, 1); r != InputConsumed {
		t.Fatalf("wheel result=%v want consumed", r)
	}
	if got := realOf(k); got < 119.99 || got > 120.01 {
		t.Fatalf("wheel +1 real=%v want ~120", got)
	}
}

func TestDiscreteKnobWheelStepsOneIndex(t *testing.T) {
	k := newDiscreteKnob([]string{"a", "b", "c", "d"}) // 4 detents; helper from knob_discrete_test.go
	if r := k.HandleWheel(50, 50, 1); r != InputConsumed {
		t.Fatalf("wheel result=%v want consumed", r)
	}
	if !nearAnyDetent(k.Value, 4) || k.Value < 0.33 || k.Value > 0.34 {
		t.Fatalf("wheel +1 idx value=%v want 1/3", k.Value)
	}
}

// TestKnobStepValueByWheelIsDebounced verifies the two-finger value wheel is
// clicky/slow: sub-threshold notches don't move the value, the threshold moves
// it by exactly one StepMul, and a single large-magnitude event is one notch.
func TestKnobStepValueByWheelIsDebounced(t *testing.T) {
	k := newEndlessKnob() // Min 20, Max 20000, StepMul 100, Value 0 (real=20)
	start := k.Value
	for i := 0; i < knobValueWheelNotchesPerStep-1; i++ {
		if k.StepValueByWheel(1) {
			t.Fatalf("value moved before threshold (notch %d)", i+1)
		}
	}
	if k.Value != start {
		t.Fatalf("value changed before notch threshold")
	}
	if !k.StepValueByWheel(1) {
		t.Fatalf("value should step once the notch threshold is crossed")
	}
	if d := realOf(k) - (k.Scale.Min + start*(k.Scale.Max-k.Scale.Min)); d < 99.9 || d > 100.1 {
		t.Fatalf("one wheel step should move by StepMul (100), moved %v", d)
	}
	// A single high-magnitude event counts as ONE notch, never a big jump.
	k2 := newEndlessKnob()
	if k2.StepValueByWheel(50) {
		t.Fatalf("a single high-magnitude wheel event must not jump the value")
	}
}
