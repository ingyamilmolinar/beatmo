//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Generator-waveform knob contract for the native engines. Every
// oscillator-bearing engine exposes a discrete <family>_wave knob
// (0=Sine 1=Saw 2=Square 3=Triangle); the default is the waveform the
// engine has always used, so the byte-for-byte golden contract holds.
//
// Three properties beyond the generic family-knob contract:
//
//  1. EXPLICIT default == golden. The desktop dispatch elides default
//     values to the NaN sentinel, but the browser writes the user's raw
//     overlay — so a user who explicitly selects the default waveform
//     reaches the C generic-waveform path, not the NaN fallback. That
//     path's arithmetic must be identical to the original literal code
//     (osc_wave's Sine/Square cases mirror the original expressions).
//     This sub-test calls the family renderers directly (bypassing
//     elision) to lock it.
//  2. Distinctness: all four waveforms produce pairwise-different output
//     (a selector position that aliases another is a silent no-op).
//  3. Enum shape: the ParamDef renders as a 4-option discrete selector.

// waveCase: one engine's wave-knob wiring for the direct-render sub-test.
type waveCase struct {
	recipe  string
	param   string
	def     float64                                       // engine's native waveform
	render  func(buf []float32, sr, n int, wave float64)  // direct family render with ONLY wave set
}

func waveCases() []waveCase {
	mk := func(recipe, param string, def float64, render func(buf []float32, sr, n int, wave float64)) waveCase {
		return waveCase{recipe: recipe, param: param, def: def, render: render}
	}
	// Cymbal migrated to the modular engine (Phase-6): the wave knob drives
	// gen1_wave through the native binding (cymbalRecipeToModular, the NaN-elision
	// path). renderCymbalModularWave renders a cymbal-family recipe with an explicit
	// cym_wave override so the explicit-default==golden bit-identity and the
	// all-waveforms-distinct property carry over to the unified engine. At wave==0
	// (or 2 for hats) the override equals the recipe default, so elision restores
	// the C kp_get fallback — byte-identical to the golden (renderHiHatVoice etc.).
	// (Shaker has no tone osc, so it is absent here.)
	renderCymbalModularWave := func(recipeID string) func(buf []float32, sr, n int, wave float64) {
		spec := cymbalVariantSpecs[recipeID]
		wired := cymbalWiredFields(recipeID)
		return func(buf []float32, sr, n int, wave float64) {
			merged := MergeRecipeDefaults(recipeID, RecipeParams{"cym_wave": wave})
			renderModularP(buf, sr, n, cymbalRecipeToModular(recipeID, merged, spec.variant, wired))
		}
	}
	// Bass migrated to the modular engine (Phase-2): the wave knob now drives
	// gen1_wave through the native binding. renderSubBassModularWave renders the
	// sub-bass recipe with an explicit bass_wave override via bassRecipeToModular
	// (the NaN-elision native path), so the explicit-default==golden bit-identity
	// (TestWaveKnob_ExplicitDefaultBitIdentical) and the all-waveforms-distinct
	// property carry over to the unified engine. At wave==0 the override equals
	// the recipe default, so elision restores the kp_get fallback — byte-identical
	// to the golden (renderSubBassVoice).
	renderSubBassModularWave := func(buf []float32, sr, n int, wave float64) {
		merged := MergeRecipeDefaults("drum-sub-bass", RecipeParams{"bass_wave": wave})
		renderModularP(buf, sr, n, bassRecipeToModular("drum-sub-bass", merged))
	}
	// Kick migrated to the modular engine (Phase-3): the wave knob drives
	// gen1_wave through the native binding (kickRecipeToModular, the NaN-elision
	// path). renderKickModularWave renders a kick recipe with an explicit
	// kick_wave override so the explicit-default==golden bit-identity and the
	// all-waveforms-distinct property carry over to the unified engine. At
	// wave==0 the override equals the recipe default, so elision restores the C
	// kp_get fallback — byte-identical to the golden (renderKick*Voice).
	renderKickModularWave := func(recipeID string) func(buf []float32, sr, n int, wave float64) {
		spec := kickVariantSpecs[recipeID]
		wired := kickWiredFields(recipeID)
		return func(buf []float32, sr, n int, wave float64) {
			merged := MergeRecipeDefaults(recipeID, RecipeParams{"kick_wave": wave})
			renderModularP(buf, sr, n, kickRecipeToModular(recipeID, merged, spec.variant, spec.def, wired))
		}
	}
	// Tom migrated to the modular engine (Phase-4): the wave knob drives gen1_wave
	// through the native binding (tomRecipeToModular, the NaN-elision path).
	// renderTomModularWave renders a tom recipe with an explicit tom_wave override
	// so the explicit-default==golden bit-identity and the all-waveforms-distinct
	// property carry over to the unified engine. At wave==0 the override equals the
	// recipe default, so elision restores the C kp_get fallback — byte-identical to
	// the golden (renderTom*Voice).
	renderTomModularWave := func(recipeID string) func(buf []float32, sr, n int, wave float64) {
		spec := tomVariantSpecs[recipeID]
		return func(buf []float32, sr, n int, wave float64) {
			merged := MergeRecipeDefaults(recipeID, RecipeParams{"tom_wave": wave})
			renderModularP(buf, sr, n, tomRecipeToModular(recipeID, merged, spec.variant, spec.def))
		}
	}
	// Snare migrated to the modular engine (Phase-5): the wave knob drives gen1_wave
	// through the native binding (snareRecipeToModular, the NaN-elision path).
	// renderSnareModularWave renders a snare-family recipe with an explicit
	// snare_wave override so the explicit-default==golden bit-identity and the
	// all-waveforms-distinct property carry over to the unified engine. At wave==0
	// the override equals the recipe default, so elision restores the C kp_get
	// fallback — byte-identical to the golden (renderSnare*Voice). (Clap has no
	// tone osc, so it is absent here.)
	renderSnareModularWave := func(recipeID string) func(buf []float32, sr, n int, wave float64) {
		spec := snareVariantSpecs[recipeID]
		wired := snareWiredFields(recipeID)
		return func(buf []float32, sr, n int, wave float64) {
			merged := MergeRecipeDefaults(recipeID, RecipeParams{"snare_wave": wave})
			renderModularP(buf, sr, n, snareRecipeToModular(recipeID, merged, spec.variant, spec.def, wired))
		}
	}
	// renderFMModularWave renders an FM-family recipe with an explicit fm_wave
	// override through the migrated modular binding (source==10 FM voice). At
	// wave==0 the override equals the recipe default, so elision restores the C
	// kp_get fallback — byte-identical to the golden (renderFM*Voice). Mirrors
	// renderSnareModularWave / renderCymbalModularWave (Phase-7 cutover: the
	// render_fm_*_p C path + the FMParams adapter were deleted).
	renderFMModularWave := func(recipeID string) func(buf []float32, sr, n int, wave float64) {
		spec := fmVariantSpecs[recipeID]
		wired := fmWiredFields(recipeID)
		return func(buf []float32, sr, n int, wave float64) {
			merged := MergeRecipeDefaults(recipeID, RecipeParams{"fm_wave": wave})
			renderModularP(buf, sr, n, fmRecipeToModular(recipeID, merged, spec.variant, wired))
		}
	}
	return []waveCase{
		mk("drum-kick", "kick_wave", 0, renderKickModularWave("drum-kick")),
		mk("drum-kick-deep", "kick_wave", 0, renderKickModularWave("drum-kick-deep")),
		mk("drum-kick-punchy", "kick_wave", 0, renderKickModularWave("drum-kick-punchy")),
		mk("drum-kick-lofi", "kick_wave", 0, renderKickModularWave("drum-kick-lofi")),
		mk("drum-kick-tight", "kick_wave", 0, renderKickModularWave("drum-kick-tight")),
		mk("drum-tom", "tom_wave", 0, renderTomModularWave("drum-tom")),
		mk("drum-tom-high", "tom_wave", 0, renderTomModularWave("drum-tom-high")),
		mk("drum-tom-low", "tom_wave", 0, renderTomModularWave("drum-tom-low")),
		mk("drum-snare", "snare_wave", 0, renderSnareModularWave("drum-snare")),
		mk("drum-snare-rimshot", "snare_wave", 0, renderSnareModularWave("drum-snare-rimshot")),
		mk("drum-snare-sidestick", "snare_wave", 0, renderSnareModularWave("drum-snare-sidestick")),
		mk("drum-hihat", "cym_wave", 2, renderCymbalModularWave("drum-hihat")),
		mk("drum-open-hihat", "cym_wave", 2, renderCymbalModularWave("drum-open-hihat")),
		mk("drum-ride", "cym_wave", 0, renderCymbalModularWave("drum-ride")),
		mk("drum-crash", "cym_wave", 0, renderCymbalModularWave("drum-crash")),
		mk("drum-cowbell", "cym_wave", 0, renderCymbalModularWave("drum-cowbell")),
		mk("drum-sub-bass", "bass_wave", 0, renderSubBassModularWave),
		mk("fm-bass", "fm_wave", 0, renderFMModularWave("fm-bass")),
		mk("fm-bell", "fm_wave", 0, renderFMModularWave("fm-bell")),
		mk("fm-lead", "fm_wave", 0, renderFMModularWave("fm-lead")),
		mk("fm-epiano", "fm_wave", 0, renderFMModularWave("fm-epiano")),
		mk("fm-pluck", "fm_wave", 0, renderFMModularWave("fm-pluck")),
	}
}

// TestWaveKnob_ExplicitDefaultBitIdentical locks the browser-equivalent
// path: the family renderer with the wave field EXPLICITLY set to the
// engine's default waveform (no elision) must be byte-identical to the
// unparameterized golden render.
func TestWaveKnob_ExplicitDefaultBitIdentical(t *testing.T) {
	for _, wc := range waveCases() {
		t.Run(wc.recipe, func(t *testing.T) {
			golden := rawGoldenHash(t, wc.recipe)
			buf := make([]float32, nativeGoldenSamples)
			wc.render(buf, nativeGoldenSR, nativeGoldenSamples, wc.def)
			if got := hashFloat32(buf); got != golden {
				t.Errorf("%s: explicit %s=%v diverged from the NaN-fallback golden\n  got:    %s\n  golden: %s\n(osc_wave's default case must mirror the original literal expression)", wc.recipe, wc.param, wc.def, got, golden)
			}
		})
	}
}

// TestWaveKnob_AllWaveformsDistinct proves every selector position is
// audible AND distinct from every other (no aliased positions).
func TestWaveKnob_AllWaveformsDistinct(t *testing.T) {
	for _, wc := range waveCases() {
		t.Run(wc.recipe, func(t *testing.T) {
			hashes := map[string]float64{}
			for w := 0.0; w <= 3; w++ {
				buf := make([]float32, nativeGoldenSamples)
				wc.render(buf, nativeGoldenSR, nativeGoldenSamples, w)
				h := hashFloat32(buf)
				if prev, dup := hashes[h]; dup {
					t.Errorf("%s: waveform %v renders identically to waveform %v (selector position is a silent alias)", wc.recipe, w, prev)
				}
				hashes[h] = w
			}
		})
	}
}

// TestWaveKnob_ParamDefShape locks the discrete-selector contract: every
// *_wave ParamDef is a 4-option enum over [0,3] with an integer default
// matching the engine's native waveform.
func TestWaveKnob_ParamDefShape(t *testing.T) {
	wantEnum := []string{"Sine", "Saw", "Square", "Triangle"}
	for _, wc := range waveCases() {
		t.Run(wc.recipe, func(t *testing.T) {
			var def *ParamDef
			for _, d := range WiredParamsForRecipe(wc.recipe) {
				if d.Name == wc.param {
					dd := d
					def = &dd
					break
				}
			}
			if def == nil {
				t.Fatalf("%s: no %s ParamDef", wc.recipe, wc.param)
			}
			if def.Min != 0 || def.Max != 3 {
				t.Errorf("%s: range [%v,%v], want [0,3]", wc.recipe, def.Min, def.Max)
			}
			if def.Default != wc.def || def.Default != math.Trunc(def.Default) {
				t.Errorf("%s: default %v, want integer %v", wc.recipe, def.Default, wc.def)
			}
			if len(def.Enum) != len(wantEnum) {
				t.Fatalf("%s: enum %v, want %v", wc.recipe, def.Enum, wantEnum)
			}
			for i, lbl := range wantEnum {
				if def.Enum[i] != lbl {
					t.Errorf("%s: enum[%d] = %q, want %q", wc.recipe, i, def.Enum[i], lbl)
				}
			}
		})
	}
}

// TestWaveKnob_NoOscillatorEnginesExcluded documents the curation: engines
// without a core oscillator (clap = filtered noise bursts, shaker = noise
// grains, bass-guitar = Karplus-Strong noise-excited string) must NOT
// declare a wave knob — it would be a silent no-op selector.
func TestWaveKnob_NoOscillatorEnginesExcluded(t *testing.T) {
	for _, recipe := range []string{"drum-clap", "drum-shaker", "drum-bass-guitar"} {
		for _, d := range WiredParamsForRecipe(recipe) {
			switch d.Name {
			case "snare_wave", "cym_wave", "bass_wave":
				t.Errorf("%s declares %q but has no core oscillator (silent no-op selector)", recipe, d.Name)
			}
		}
	}
}
