//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// renderModularSeed renders a seeded voice through the same recipe path the
// modular goldens use: the RecipeParams seed is overlaid on the schema
// identity (recipeParamsToModular fills any absent key from
// ModularParamSchemaIdentity()), then rendered via renderModularP.
func renderModularSeed(t *testing.T, seed RecipeParams, secs float64) []float32 {
	t.Helper()
	const sr = 48000
	n := int(secs * sr)
	buf := make([]float32, n)
	renderModularP(buf, sr, n, recipeParamsToModular(seed))
	return buf
}

// Formant stage off (identity) must be an exact bypass: byte-identical to a
// render with no formant params at all.
func TestFormantStage_OffIsExactBypass(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	withOff := RecipeParams{"osc_type": 1, "osc_enabled": 1, "formant_mix": 0.9} // enabled ABSENT
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, withOff, 0.5)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs with formant disabled: %v vs %v", i, a[i], b[i])
		}
	}
}

// Enabling the stage with mix > 0 must change the output and stay finite.
func TestFormantStage_OnChangesOutputFinite(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	on := RecipeParams{"osc_type": 1, "osc_enabled": 1,
		"formant_enabled": 1, "formant_mix": 0.9, "formant_vowel": 0, "formant_voice_type": 2}
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, on, 0.5)
	diff := false
	for i := range b {
		if math.IsNaN(float64(b[i])) || math.IsInf(float64(b[i]), 0) {
			t.Fatalf("non-finite sample %d", i)
		}
		if a[i] != b[i] {
			diff = true
		}
	}
	if !diff {
		t.Fatal("formant stage enabled but output identical")
	}
}

// Different vowels must produce different spectra (coarse: different bytes).
func TestFormantStage_VowelsDiffer(t *testing.T) {
	mk := func(vowel float64) RecipeParams {
		return RecipeParams{"osc_type": 1, "osc_enabled": 1,
			"formant_enabled": 1, "formant_mix": 0.9, "formant_vowel": vowel, "formant_voice_type": 3}
	}
	a := renderModularSeed(t, mk(0), 0.5)
	u := renderModularSeed(t, mk(4), 0.5)
	same := true
	for i := range a {
		if a[i] != u[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("vowel a and vowel u rendered identically")
	}
}

// Breath: noise-only whisper (osc off, breath 1) must be non-silent and finite.
func TestFormantStage_WhisperNonSilent(t *testing.T) {
	w := RecipeParams{"osc_enabled": 0,
		"formant_enabled": 1, "formant_mix": 1, "formant_breath": 1, "formant_vowel": 2}
	b := renderModularSeed(t, w, 0.5)
	peak := float32(0)
	for _, s := range b {
		if s > peak {
			peak = s
		}
		if math.IsNaN(float64(s)) {
			t.Fatal("NaN in whisper render")
		}
	}
	if peak < 0.01 {
		t.Fatalf("whisper render is silent (peak %v)", peak)
	}
}
