//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestFiltEnv_ZeroAmtIsNeutral asserts that filtenv_enabled with amt=0 produces
// output byte-identical to filtenv_disabled — the AMOUNT knob (not just the
// enable flag) must gate the effect, so a stage that is "on" but at amt=0 is a
// true no-op.
//
// The monotonic-brightening-with-amt coverage and the enabled=0 ≡ baseline
// identity check live in all_instruments_pitch_table_test.go
// (TestFiltEnvMonotonicBrightening). This test pins the orthogonal amt=0 case.
func TestFiltEnv_ZeroAmtIsNeutral(t *testing.T) {
	const sr, n = 48000, 24000

	mkBase := func() ModularParams {
		p := defaultModularParams()
		p.OscType = 1
		p.EnvEnabled = 0
		p.FilterEnabled = 1
		p.FilterType = 0
		p.FilterCutoff = 1500
		p.FiltEnvDecay = 0.2
		return p
	}

	disabled := mkBase()
	disabled.FiltEnvEnabled = 0
	disabled.FiltEnvAmt = 3 // non-zero amt but stage disabled → must be no-op
	bufDisabled := make([]float32, n)
	renderModularP(bufDisabled, sr, n, disabled)

	zeroAmt := mkBase()
	zeroAmt.FiltEnvEnabled = 1
	zeroAmt.FiltEnvAmt = 0 // amt=0 with stage enabled → no cutoff motion
	bufZeroAmt := make([]float32, n)
	renderModularP(bufZeroAmt, sr, n, zeroAmt)

	noFiltEnv := mkBase()
	noFiltEnv.FiltEnvEnabled = 0
	noFiltEnv.FiltEnvAmt = 0
	bufNoFiltEnv := make([]float32, n)
	renderModularP(bufNoFiltEnv, sr, n, noFiltEnv)

	// disabled (with amt=3) must equal not-present.
	var maxDiff float64
	for i := range bufDisabled {
		if d := math.Abs(float64(bufDisabled[i] - bufNoFiltEnv[i])); d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 0 {
		t.Errorf("filtenv disabled with amt=3 differs from no-filtenv (maxDiff=%.6f); "+
			"disabled stage must be an exact bypass regardless of amt", maxDiff)
	}

	// enabled with amt=0 must equal not-present (the amount knob gates the effect).
	maxDiff = 0
	for i := range bufZeroAmt {
		if d := math.Abs(float64(bufZeroAmt[i] - bufNoFiltEnv[i])); d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 1e-6 {
		t.Errorf("filtenv enabled with amt=0 differs from no-filtenv (maxDiff=%.6f); "+
			"zero amt should produce no audible brightening", maxDiff)
	}
}
