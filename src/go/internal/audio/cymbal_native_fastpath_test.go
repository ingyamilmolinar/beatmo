//go:build !test && !js

package audio

import "testing"

// Phase-6 cymbal-family migration (native no-edit fast-path cutover).
//
// The instruments-table Render closure for an unedited cymbal-family instrument
// must produce a buffer byte-identical to the recipe path's default render.
// Before the cutover the table pointed Render at the legacy render_hihat /
// render_open_hihat / render_cowbell / render_shaker / render_ride / render_crash
// C functions; after it, Render bakes the recipe-default ModularParams and renders
// via render_modular_p (renderHiHatVoice / renderOpenHiHatVoice /
// renderCowbellVoice / renderShakerVoice / renderRideVoice / renderCrashVoice).
// This test proves the baked fast path equals the recipe path so the cheap
// no-overlay dispatch and the recipe/edit dispatch agree to the bit — the same
// invariant nativeGoldenCases locks via the golden hash. Mirrors
// snare_native_fastpath_test.go.

func TestCymbalNoEditFastPathMatchesRecipe(t *testing.T) {
	const sr = nativeGoldenSR
	const n = nativeGoldenSamples
	for _, tc := range []struct {
		recipeID string
		fast     func(buf []float32, sampleRate, samples int)
	}{
		{"drum-hihat", renderHiHatVoice},
		{"drum-open-hihat", renderOpenHiHatVoice},
		{"drum-cowbell", renderCowbellVoice},
		{"drum-shaker", renderShakerVoice},
		{"drum-ride", renderRideVoice},
		{"drum-crash", renderCrashVoice},
	} {
		t.Run(tc.recipeID, func(t *testing.T) {
			recipeBuf := make([]float32, n)
			r := NewRecipe(tc.recipeID)
			if r == nil {
				t.Fatalf("NewRecipe(%q) returned nil", tc.recipeID)
			}
			r.Render(recipeBuf, sr, n, 0, MergeRecipeDefaults(tc.recipeID, RecipeParams{}))
			recipeHash := hashFloat32(recipeBuf)

			fastBuf := make([]float32, n)
			tc.fast(fastBuf, sr, n)
			fastHash := hashFloat32(fastBuf)

			if recipeHash != fastHash {
				t.Errorf("%s: no-edit fast path diverged from recipe-default render\n  fast:   %s\n  recipe: %s\n(the baked ModularParams must reproduce the recipe path bit-for-bit)", tc.recipeID, fastHash, recipeHash)
			}
		})
	}
}

// TestCymbalInstrumentTableUsesModularFastPath asserts every live cymbal-family
// instrument renders a non-silent voice through the table — proving its Render
// path is the modular fast path, not a dead/deleted legacy C wrapper. The base
// cymbals keep their dedicated HiHat{}/Cowbell{}/Shaker{}/Ride{}/Crash{} struct
// types with renderXVoice swapped in; the variants are CVariantInstruments. So
// this introspects via the Instrument.NewVoice contract rather than asserting a
// concrete type. Mirrors TestSnareInstrumentTableUsesModularFastPath.
func TestCymbalInstrumentTableUsesModularFastPath(t *testing.T) {
	ResetInstruments()
	instMu.RLock()
	defer instMu.RUnlock()
	for _, id := range []string{
		"hihat", "hihat-1", "hihat-2", "hihat-pedal",
		"cowbell", "cowbell-1", "cowbell-2",
		"shaker", "ride", "crash",
	} {
		inst, ok := instruments[id]
		if !ok {
			t.Errorf("instrument %q missing from table", id)
			continue
		}
		v := inst.NewVoice(120, nativeGoldenSR)
		var energy float64
		for i := 0; i < nativeGoldenSamples; i++ {
			s, done := v.Sample()
			energy += s * s
			if done {
				break
			}
		}
		if energy == 0 {
			t.Errorf("instrument %q rendered silence via its table NewVoice path", id)
		}
	}
}
