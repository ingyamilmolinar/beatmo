//go:build test

package audio

import (
	"math"
	"testing"
)

// previewHash is a cheap order-sensitive digest of a PCM buffer for "did it
// change?" comparisons (hashFloat64PCM lives in the !test offline harness).
func previewHash(buf []float64) uint64 {
	var h uint64 = 1469598103934665603
	for _, v := range buf {
		bits := math.Float64bits(math.Round(v*1e6) / 1e6)
		h ^= bits
		h *= 1099511628211
	}
	return h
}

func previewPeak(buf []float64) float64 {
	p := 0.0
	for _, v := range buf {
		if a := math.Abs(v); a > p {
			p = a
		}
	}
	return p
}

// TestRenderInstrumentPreview_RespondsToEveryStageParam is the failing-first
// guard for "the fast-compute preview wave must respond to ANY knob change".
// The preview is an approximate, pure-Go synth (no cgo) so it works identically
// on desktop, wasm, and under test — and reads every major stage param, so
// turning any knob audibly (and visibly) changes the rendered note.
//
// Each case enables its stage (so the param is in the active signal path) then
// changes the param and asserts the rendered PCM hash changes.
func TestRenderInstrumentPreview_RespondsToEveryStageParam(t *testing.T) {
	const inst = "preview-responsiveness-test"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	cases := []struct {
		name   string
		enable string             // *_enabled param to turn the stage on (empty = always on)
		pre    map[string]float64 // prerequisite params set before the baseline render
		param  string
		value  float64
	}{
		{"pitch", "", nil, "fundamental", 180},
		{"osc shape", "", nil, "osc_type", 1},
		{"detune", "", nil, "osc_detune", 40},
		{"octave", "", nil, "osc_octave", 1},
		{"amp decay", "env_enabled", nil, "amp_decay", 1.6},
		{"amp sustain", "env_enabled", nil, "amp_sustain", 0.2},
		{"filter cutoff", "filter_enabled", nil, "filter_cutoff", 600},
		{"filter resonance", "filter_enabled", nil, "filter_resonance", 9},
		{"fm depth", "fm_enabled", nil, "fm_op2_depth", 6},
		// Ratio is only audible with non-zero depth, so seed depth first.
		{"fm ratio", "fm_enabled", map[string]float64{"fm_op2_depth": 4}, "fm_op2_ratio", 3.5},
		{"drive", "drive_enabled", nil, "drive", 0.9},
		{"gain", "", nil, "gain", 0.4},
		{"pitch env", "pitchenv_enabled", nil, "pitchenv_amt", -18},
		{"lfo depth", "lfo_enabled", nil, "lfo_depth", 0.8},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ResetInstrumentParams(inst)
			if c.enable != "" {
				SetInstrumentParam(inst, c.enable, 1)
			}
			for k, v := range c.pre {
				SetInstrumentParam(inst, k, v)
			}
			base := previewHash(RenderInstrumentPreview(inst, 200))
			SetInstrumentParam(inst, c.param, c.value)
			changed := previewHash(RenderInstrumentPreview(inst, 200))
			if base == changed {
				t.Errorf("preview did not respond to %s (%s=%v): identical render", c.name, c.param, c.value)
			}
		})
	}
}

// TestRenderInstrumentPreview_RespondsToFMBaseFreq guards that the fast-compute
// preview honors fm_base_freq (the FM voice's fundamental, exposed as "Base
// Pitch" on fm-bass et al). Changing it must change the rendered carrier period,
// so conceptPitchWave (which routes fm_base_freq) can show it and the "Your
// sound" mirror responds — otherwise the preview is invariant to it.
func TestRenderInstrumentPreview_RespondsToFMBaseFreq(t *testing.T) {
	const inst = "preview-fm-base-freq-test"
	BindInstrumentToRecipe(inst, "fm-bass")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	ResetInstrumentParams(inst)
	SetInstrumentParam(inst, "fm_base_freq", 50)
	low := RenderInstrumentPreview(inst, 24)
	lowHash := previewHash(low)

	SetInstrumentParam(inst, "fm_base_freq", 800)
	high := RenderInstrumentPreview(inst, 24)
	highHash := previewHash(high)

	if len(low) == 0 || len(high) == 0 {
		t.Fatal("empty preview PCM")
	}
	if lowHash == highHash {
		t.Fatalf("preview did not respond to fm_base_freq: identical render at 50 Hz vs 800 Hz")
	}
	// Sanity: the carrier period should genuinely differ, not just a transient.
	// Sum-of-abs-diff must exceed a clear epsilon.
	var diff float64
	for i := 0; i < len(low) && i < len(high); i++ {
		diff += math.Abs(low[i] - high[i])
	}
	if diff < 0.1 {
		t.Fatalf("preview barely responded to fm_base_freq: sum|diff|=%v (expected a clear carrier-period change)", diff)
	}
}

// TestRenderInstrumentPreview_NonTrivial guards that the approximate preview is
// an actual signal (not a zeroed buffer): a default modular note must have an
// audible peak.
func TestRenderInstrumentPreview_NonTrivial(t *testing.T) {
	const inst = "preview-nontrivial-test"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	pcm := RenderInstrumentPreview(inst, 200)
	if len(pcm) == 0 {
		t.Fatal("empty preview PCM")
	}
	if peak := previewPeak(pcm); peak <= 0.01 {
		t.Fatalf("preview is silent (peak=%v) — expected an audible note", peak)
	}
}
