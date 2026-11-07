// src/go/internal/ui/knob_step_default_test.go
package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The round decade rung nearest span/200: a number a human would count by.
func TestDefaultStepIsRoundDecadeRung(t *testing.T) {
	cases := []struct {
		name string
		def  audio.ParamDef
		want float64
	}{
		{"attack seconds", audio.ParamDef{Name: "attack", Min: 0, Max: 2, Unit: "s"}, 0.01},
		{"cutoff Hz", audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}, 100},
		{"mix unitless", audio.ParamDef{Name: "mix", Min: 0, Max: 1}, 0.01},
		{"gain dB", audio.ParamDef{Name: "gain", Min: -60, Max: 12, Unit: "dB"}, 1},
		{"detune cents", audio.ParamDef{Name: "detune", Min: -100, Max: 100, Unit: "cents"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ladder := stepLadder(tc.def)
			got := ladder[defaultStepIndex(tc.def, ladder)]
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("default step = %v, want %v (ladder %v)", got, tc.want, ladder)
			}
		})
	}
}

// With no decade rung, the nearest rung to span/200, never index 0.
func TestDefaultStepFallsBackWhenLadderHasNoDecade(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 1000}
	ladder := []float64{2, 5, 20, 50} // deliberately decade-free
	idx := defaultStepIndex(def, ladder)
	if idx < 0 || idx >= len(ladder) {
		t.Fatalf("index out of range: %d", idx)
	}
	if got := ladder[idx]; got != 5 { // span/200 == 5
		t.Fatalf("fallback step = %v, want the nearest rung 5", got)
	}
}

// No panic and a valid index for an empty ladder or a zero-width range.
func TestDefaultStepIndexHandlesDegenerateInput(t *testing.T) {
	if got := defaultStepIndex(audio.ParamDef{Min: 1, Max: 1}, []float64{1}); got != 0 {
		t.Fatalf("zero span: got %d, want 0", got)
	}
	if got := defaultStepIndex(audio.ParamDef{Min: 0, Max: 1}, nil); got != 0 {
		t.Fatalf("empty ladder: got %d, want 0", got)
	}
}
