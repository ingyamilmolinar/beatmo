package ui

import (
	"math"
	"testing"
)

func newEndlessKnobForTest(min, max, stepMul float64) *Knob {
	k := &Knob{Endless: true, StepMul: stepMul}
	k.Scale = KnobScale{Min: min, Max: max}
	k.Value = 0.5 // midpoint
	return k
}

func realValue(k *Knob) float64 {
	return k.Scale.Min + k.Value*(k.Scale.Max-k.Scale.Min)
}

func TestNudgeEndless_IncreasesByStep(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	start := realValue(k) // 100
	k.NudgeEndless(knobEndlessPxPerNotch * 5)
	got := realValue(k)
	if math.Abs(got-(start+5)) > 1e-6 {
		t.Fatalf("NudgeEndless(+5 notches): got %v want %v", got, start+5)
	}
}

func TestNudgeEndless_NegativeDecreases(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	start := realValue(k)
	k.NudgeEndless(-knobEndlessPxPerNotch * 3)
	got := realValue(k)
	if math.Abs(got-(start-3)) > 1e-6 {
		t.Fatalf("NudgeEndless(-3 notches): got %v want %v", got, start-3)
	}
}

func TestNudgeEndless_ClampsAtMax(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	k.NudgeEndless(knobEndlessPxPerNotch * 10000)
	if got := realValue(k); math.Abs(got-10) > 1e-6 {
		t.Fatalf("clamp at max: got %v want 10", got)
	}
}

func TestNudgeEndless_ClampsAtMin(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	k.NudgeEndless(-knobEndlessPxPerNotch * 10000)
	if got := realValue(k); math.Abs(got-0) > 1e-6 {
		t.Fatalf("clamp at min: got %v want 0", got)
	}
}

func TestMobileWheelDensityTokensPresent(t *testing.T) {
	dvals := Profile().DensityValues()
	if dvals.MobileWheelW <= 0 || dvals.MobileWheelH <= 0 ||
		dvals.MobileWheelTickGap <= 0 || dvals.MobileWheelResStripW <= 0 ||
		dvals.MobileWheelPillH <= 0 {
		t.Fatalf("wheel density tokens must be positive, got %+v", dvals)
	}
}
