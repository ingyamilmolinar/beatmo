//go:build test

package ui

import (
	"math"
	"testing"
)

func knobRadDeg(d float64) float32 { return float32(d * math.Pi / 180) }

func knobApproxEq(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

// knobValAngle is the absolute arc angle (radians) for a normalized value.
func knobValAngle(v float64) float32 {
	return knobRadDeg(knobArcStartDeg + v*knobArcSweepDeg)
}

// TestKnobValueArcSpanUnipolar pins the default (unipolar) behaviour: the value
// arc grows from the track start to the value angle; value 0 draws nothing.
func TestKnobValueArcSpanUnipolar(t *testing.T) {
	if _, _, draw := knobValueArcSpan(0, 0.5, false); draw {
		t.Errorf("unipolar value=0 should draw no value arc")
	}
	from, to, draw := knobValueArcSpan(1, 0.5, false)
	if !draw {
		t.Fatalf("unipolar value=1 should draw a full value arc")
	}
	if !knobApproxEq(from, knobValAngle(0)) {
		t.Errorf("unipolar from = %v, want start %v", from, knobValAngle(0))
	}
	if !knobApproxEq(to, knobValAngle(1)) {
		t.Errorf("unipolar to = %v, want end %v", to, knobValAngle(1))
	}
}

// TestKnobValueArcSpanBipolarSymmetric pins symmetric bipolar params (detune
// ±cents, transpose ±st): zero sits at the 12-o'clock centre (zeroFrac 0.5),
// value 0.5 draws nothing, and the fill grows symmetrically from centre.
func TestKnobValueArcSpanBipolarSymmetric(t *testing.T) {
	if _, _, draw := knobValueArcSpan(0.5, 0.5, true); draw {
		t.Errorf("bipolar value=0.5 (centre) should draw no value arc")
	}
	from, to, draw := knobValueArcSpan(0.75, 0.5, true)
	if !draw {
		t.Fatalf("bipolar value=0.75 should draw a value arc")
	}
	if !knobApproxEq(from, knobValAngle(0.5)) || !knobApproxEq(to, knobValAngle(0.75)) {
		t.Errorf("bipolar(0.75) span = [%v,%v], want [centre,0.75]", from, to)
	}
	from, to, draw = knobValueArcSpan(0.25, 0.5, true)
	if !draw || !knobApproxEq(from, knobValAngle(0.25)) || !knobApproxEq(to, knobValAngle(0.5)) {
		t.Errorf("bipolar(0.25) span = [%v,%v], want [0.25,centre]", from, to)
	}
}

// TestKnobValueArcSpanBipolarAsymmetric pins an asymmetric bipolar param such
// as gain (-24..+6 dB, so 0 dB is at zeroFrac 0.8). The neutral point is the
// param's zero, NOT the geometric centre: value 0.8 draws nothing; above/below
// fill toward 0.8. This is the audit's "Gain +0 dB renders ~85% ring" bug.
func TestKnobValueArcSpanBipolarAsymmetric(t *testing.T) {
	const zero = 0.8 // (0 - (-24)) / (6 - (-24))
	if _, _, draw := knobValueArcSpan(zero, zero, true); draw {
		t.Errorf("asymmetric bipolar value at zeroFrac %v should draw no value arc", zero)
	}
	from, to, draw := knobValueArcSpan(0.9, zero, true)
	if !draw || !knobApproxEq(from, knobValAngle(zero)) || !knobApproxEq(to, knobValAngle(0.9)) {
		t.Errorf("gain(0.9) span = [%v,%v], want [zero(%v),0.9]", from, to, zero)
	}
	from, to, draw = knobValueArcSpan(0.5, zero, true)
	if !draw || !knobApproxEq(from, knobValAngle(0.5)) || !knobApproxEq(to, knobValAngle(zero)) {
		t.Errorf("gain(0.5) span = [%v,%v], want [0.5,zero(%v)]", from, to, zero)
	}
}
