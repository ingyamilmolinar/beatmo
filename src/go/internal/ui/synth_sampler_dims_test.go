//go:build test

package ui

import "testing"

// TestSynthSamplerDimTokens pins the synth/sampler tab layout knobs: the header
// strip height (a desktop/mobile profileOverride), the preview-pane target/min
// widths, and the sampler waveform/body floors — all re-styleable from DESIGN.md.
func TestSynthSamplerDimTokens(t *testing.T) {
	if Profile().SynthHeaderH <= 0 {
		t.Errorf("SynthHeaderH profileOverride unset (got %d)", Profile().SynthHeaderH)
	}
	d := Profile().DensityValues()
	for name, v := range map[string]int{
		"SynthPreviewTargetW": d.SynthPreviewTargetW,
		"SynthPreviewMinW":    d.SynthPreviewMinW,
		"SamplerWaveMinH":     d.SamplerWaveMinH,
		"SamplerMinBodyH":     d.SamplerMinBodyH,
	} {
		if v <= 0 {
			t.Errorf("density token %s unset (got %d)", name, v)
		}
	}
}
