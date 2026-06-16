//go:build test

package audio

import (
	"math"
	"testing"
)

// totalVariation sums |Δ| across the wave — a proxy for "sharpness"/brightness:
// a smooth (dull) wave has low variation, a bright/edgy one has high variation.
func totalVariation(w []float64) float64 {
	s := 0.0
	for i := 1; i < len(w); i++ {
		s += math.Abs(w[i] - w[i-1])
	}
	return s
}

// TestPreviewWave_CleanAndBounded — the wave primitive renders a fixed number of
// clean cycles, soft-clamped into [-1,1] so it can never block out the trace,
// and tall enough to read.
func TestPreviewWave_CleanAndBounded(t *testing.T) {
	const inst = "preview-wave-clean"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	w := RenderInstrumentPreviewWave(inst, nil, 3, 96)
	if len(w) != 3*96 {
		t.Fatalf("want exactly 3 cycles × 96 pts = 288 samples, got %d", len(w))
	}
	for i, v := range w {
		if math.Abs(v) > 1.0001 {
			t.Fatalf("sample %d = %v exceeds [-1,1] — would clip into a solid block", i, v)
		}
	}
	if previewPeak(w) < 0.3 {
		t.Fatalf("wave too flat (peak %v) to be legible", previewPeak(w))
	}
}

// TestPreviewWave_CleanForBespokeDrum — the kick that showed orange/cyan blocks
// must now be a clean bounded wave too (this is the actual regression).
func TestPreviewWave_CleanForBespokeDrum(t *testing.T) {
	const inst = "preview-wave-kick"
	BindInstrumentToRecipe(inst, "drum-kick-punchy")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	w := RenderInstrumentPreviewWave(inst, nil, 3, 96)
	if len(w) == 0 {
		t.Fatal("empty kick wave")
	}
	for i, v := range w {
		if math.Abs(v) > 1.0001 {
			t.Fatalf("kick sample %d = %v exceeds [-1,1] — the block artifact", i, v)
		}
	}
	if previewPeak(w) < 0.3 {
		t.Fatalf("kick wave too flat (peak %v)", previewPeak(w))
	}
}

// TestPreviewWave_DefaultIsSmoothNotBuzzy — a default (sine, filter wide open)
// wave must be SMOOTH, not the Nyquist-buzz square an unstable filter produces.
// A clean 3-cycle sine crosses zero ~6 times; a buzzy ±peak square crosses every
// sample. Total variation of a smooth sine is small; of the buzz, enormous.
func TestPreviewWave_DefaultIsSmoothNotBuzzy(t *testing.T) {
	const inst = "preview-wave-smooth"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	w := RenderInstrumentPreviewWave(inst, nil, 3, 96)
	// A clean 3-cycle sine of amplitude ~0.9 has total variation ≈ 3*4*0.9 ≈ 11.
	// The Nyquist buzz (alternating ±0.9 every sample) has TV ≈ 288*1.8 ≈ 518.
	tv := totalVariation(w)
	if tv > 60 {
		t.Fatalf("default wave total variation %.1f is way too high — it's buzzy, not a clean wave", tv)
	}
	// Count zero crossings: a clean 3-cycle wave has ~6, the buzz has ~280.
	zc := 0
	for i := 1; i < len(w); i++ {
		if (w[i-1] < 0) != (w[i] < 0) {
			zc++
		}
	}
	if zc > 20 {
		t.Fatalf("default wave has %d zero crossings — buzzy, not a clean ~3-cycle wave", zc)
	}
}

// TestPreviewWave_ShapeKnobsChangeWave — each shape knob changes the wave.
func TestPreviewWave_ShapeKnobsChangeWave(t *testing.T) {
	const inst = "preview-wave-shape"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	cases := []struct {
		name, enable, param string
		v                   float64
	}{
		{"osc shape", "", "osc_type", 2},
		{"filter cutoff", "filter_enabled", "filter_cutoff", 150},
		{"drive", "drive_enabled", "drive", 0.95},
		{"fm depth", "fm_enabled", "fm_op2_depth", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ResetInstrumentParams(inst)
			if c.enable != "" {
				SetInstrumentParam(inst, c.enable, 1)
			}
			a := RenderInstrumentPreviewWave(inst, nil, 3, 96)
			SetInstrumentParam(inst, c.param, c.v)
			b := RenderInstrumentPreviewWave(inst, nil, 3, 96)
			if previewHash(a) == previewHash(b) {
				t.Errorf("%s (%s=%v) did not change the rendered wave", c.name, c.param, c.v)
			}
		})
	}
}

// TestPreviewWave_BrighterCutoffSharperWave — opening the filter passes more
// harmonics, so a harmonic-rich source gets sharper (higher total variation).
func TestPreviewWave_BrighterCutoffSharperWave(t *testing.T) {
	const inst = "preview-wave-bright"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	SetInstrumentParam(inst, "osc_type", 1) // saw — harmonic rich
	SetInstrumentParam(inst, "filter_enabled", 1)

	SetInstrumentParam(inst, "filter_cutoff", 150)
	dull := totalVariation(RenderInstrumentPreviewWave(inst, nil, 3, 128))
	SetInstrumentParam(inst, "filter_cutoff", 12000)
	bright := totalVariation(RenderInstrumentPreviewWave(inst, nil, 3, 128))
	if bright <= dull {
		t.Errorf("brighter cutoff should sharpen the wave: dull TV=%.3f bright TV=%.3f", dull, bright)
	}
}

// recipeWaveShapeParam returns the param that selects the oscillator/generator
// WAVE SHAPE for a recipe: modular's osc_type, or the bespoke "Generator" enum
// (custom name, group core, Sine/Saw/Square/Triangle), or "" if the recipe has
// no wave-shape selector.
func recipeWaveShapeParam(reg *RecipeRegistration) string {
	if reg == nil {
		return ""
	}
	for _, d := range reg.Params {
		if d.Name == "osc_type" {
			return d.Name
		}
		if d.Label == "Generator" || (len(d.Enum) >= 2 && d.Enum[0] == "Sine" && d.Enum[1] == "Saw") {
			return d.Name
		}
	}
	return ""
}

// TestPreviewWave_RespondsToWaveShapeEveryRecipe is the failing-first guard for
// "changing the wave shape must change the visualization, for EVERY recipe".
// The wave shape is the most important visual; a recipe whose shape selector the
// preview ignores (e.g. bespoke kick_wave / snare_wave vs modular osc_type) is
// the bug. For every recipe that HAS a shape selector, Sine→Saw must change the
// rendered preview wave.
func TestPreviewWave_RespondsToWaveShapeEveryRecipe(t *testing.T) {
	regs := RecipeRegistrations()
	checked := 0
	for id, reg := range regs {
		waveParam := recipeWaveShapeParam(reg)
		if waveParam == "" {
			continue
		}
		checked++
		t.Run(id, func(t *testing.T) {
			inst := "wsx-" + id
			BindInstrumentToRecipe(inst, id)
			t.Cleanup(func() { ResetInstrumentParams(inst) })

			SetInstrumentParam(inst, waveParam, 0) // Sine
			sine := previewHash(RenderInstrumentPreviewWave(inst, nil, 3, 96))
			SetInstrumentParam(inst, waveParam, 1) // Saw
			saw := previewHash(RenderInstrumentPreviewWave(inst, nil, 3, 96))
			if sine == saw {
				t.Errorf("recipe %s: changing wave shape %q Sine→Saw did NOT change the preview wave", id, waveParam)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no recipe with a wave-shape selector was checked — test is vacuous")
	}
}

// TestPreviewWave_OverrideShowsKnobEffect — the per-knob picture renders the
// wave at a contrasting value of one knob (via override) to show its effect.
func TestPreviewWave_OverrideShowsKnobEffect(t *testing.T) {
	const inst = "preview-wave-override"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	SetInstrumentParam(inst, "filter_enabled", 1)
	SetInstrumentParam(inst, "filter_cutoff", 9000)

	cur := RenderInstrumentPreviewWave(inst, nil, 3, 96)
	lo := RenderInstrumentPreviewWave(inst, map[string]float64{"filter_cutoff": 150}, 3, 96)
	if previewHash(cur) == previewHash(lo) {
		t.Fatal("filter_cutoff override did not change the wave — knob effect not demonstrable")
	}
}
