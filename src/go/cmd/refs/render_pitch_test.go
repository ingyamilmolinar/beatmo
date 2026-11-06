//go:build !js

package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// render_pitch_test.go — the synth must be rendered at the reference's ACTUAL
// pitch, not at the rounded semitone.
//
// measured.semis_from_a3 is an integer, so rendering from it detunes the synth
// against the very recording it is being A/B'd with. Across the committed
// corpus the median error is 11 cents, fifteen references exceed a quarter-tone
// and six exceed 40 cents -- audible as "out of tune" to a listener, and a
// systematic bias on every frequency-scaled diagnostic axis. F0Hz was already
// stored; only its fractional part was discarded.

func TestRenderPitch_UsesExactF0WhenReliable(t *testing.T) {
	// singing-bowl-freesound: 100.00 Hz, stored as -14 semis (+35.0 cents).
	m := wavref.Measured{F0Hz: 100.0, SemisFromA3: -14}
	got := renderPitch(m, true)
	want := 12 * math.Log2(100.0/220.0) // == -13.65
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("renderPitch = %v, want the exact %v", got, want)
	}
	cents := (got - float64(m.SemisFromA3)) * 100
	if math.Abs(cents-35.0) > 0.5 {
		t.Errorf("expected the fix to recover ~35 cents, recovered %.1f", cents)
	}
	// And it must actually differ from the old behaviour.
	if got == float64(m.SemisFromA3) {
		t.Error("renderPitch still returns the rounded semitone")
	}
}

func TestRenderPitch_FallsBackWhenF0Unreliable(t *testing.T) {
	// hihat-vcsl: 56.52 Hz detected at confidence 0.00 -- an artifact of the
	// octave guard, not a pitch. Rendering "precisely" at a meaningless number
	// buys nothing and would move existing percussion renders for no reason.
	m := wavref.Measured{F0Hz: 56.52, SemisFromA3: -24}
	if got := renderPitch(m, false); got != -24 {
		t.Errorf("renderPitch = %v, want the stored -24 when f0 is unreliable", got)
	}
}

func TestRenderPitch_FallsBackWhenNoF0(t *testing.T) {
	if got := renderPitch(wavref.Measured{F0Hz: 0, SemisFromA3: -7}, true); got != -7 {
		t.Errorf("renderPitch = %v, want the stored -7 when no f0 was detected", got)
	}
}

// TestRenderPitch_MatchesReferenceFrequency is the property that matters: the
// rendered pitch must reproduce the reference's own frequency.
func TestRenderPitch_MatchesReferenceFrequency(t *testing.T) {
	for _, f0 := range []float64{100.0, 106.49, 90.51, 440.07, 586.78, 829.63} {
		p := renderPitch(wavref.Measured{F0Hz: f0, SemisFromA3: int(math.Round(12 * math.Log2(f0/220)))}, true)
		gotHz := 220 * math.Pow(2, p/12)
		if cents := math.Abs(1200 * math.Log2(gotHz/f0)); cents > 0.01 {
			t.Errorf("f0=%.2f: rendered at %.4f Hz, %.3f cents off", f0, gotHz, cents)
		}
	}
}
