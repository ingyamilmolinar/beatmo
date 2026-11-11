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

	mkBase := func() modularBlock {
		p := defaultModularParams()
		p.set("osc_type", 1)
		p.set("env_enabled", 0)
		p.set("filter_enabled", 1)
		p.set("filter_type", 0)
		p.set("filter_cutoff", 1500)
		p.set("filtenv_decay", 0.2)
		return p
	}

	disabled := mkBase()
	disabled.set("filtenv_enabled", 0)
	disabled.set("filtenv_amt", 3) // non-zero amt but stage disabled → must be no-op
	bufDisabled := make([]float32, n)
	renderModularBlock(bufDisabled, sr, n, disabled)

	zeroAmt := mkBase()
	zeroAmt.set("filtenv_enabled", 1)
	zeroAmt.set("filtenv_amt", 0) // amt=0 with stage enabled → no cutoff motion
	bufZeroAmt := make([]float32, n)
	renderModularBlock(bufZeroAmt, sr, n, zeroAmt)

	noFiltEnv := mkBase()
	noFiltEnv.set("filtenv_enabled", 0)
	noFiltEnv.set("filtenv_amt", 0)
	bufNoFiltEnv := make([]float32, n)
	renderModularBlock(bufNoFiltEnv, sr, n, noFiltEnv)

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
