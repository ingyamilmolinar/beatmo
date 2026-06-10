//go:build !test && !js

package audio

import "testing"

// Phase-3 kick-family migration (Task 3a: native no-edit fast-path cutover).
//
// The instruments-table Render closure for an unedited kick instrument must
// produce a buffer byte-identical to the recipe path's default render. Before
// the cutover the table pointed Render at the legacy render_kick /
// render_kick_deep / render_kick_punchy / render_kick_lofi / render_kick_tight C
// functions; after it, Render bakes the recipe-default ModularParams and renders
// via render_modular_p (renderKickVoice / renderKickDeepVoice / …). This test
// proves the baked fast path equals the recipe path so the cheap no-overlay
// dispatch and the recipe/edit dispatch agree to the bit — the same invariant
// nativeGoldenCases locks via the golden hash. Mirrors
// bass_native_fastpath_test.go.

func TestKickNoEditFastPathMatchesRecipe(t *testing.T) {
	const sr = nativeGoldenSR
	const n = nativeGoldenSamples
	for _, tc := range []struct {
		recipeID string
		fast     func(buf []float32, sampleRate, samples int)
	}{
		{"drum-kick", renderKickVoice},
		{"drum-kick-deep", renderKickDeepVoice},
		{"drum-kick-punchy", renderKickPunchyVoice},
		{"drum-kick-lofi", renderKickLofiVoice},
		{"drum-kick-tight", renderKickTightVoice},
	} {
		t.Run(tc.recipeID, func(t *testing.T) {
			// Recipe-path default render (the exact path tryRecipeVoice takes for
			// an unedited instrument).
			recipeBuf := make([]float32, n)
			r := NewRecipe(tc.recipeID)
			if r == nil {
				t.Fatalf("NewRecipe(%q) returned nil", tc.recipeID)
			}
			r.Render(recipeBuf, sr, n, 0, MergeRecipeDefaults(tc.recipeID, RecipeParams{}))
			recipeHash := hashFloat32(recipeBuf)

			// No-edit fast-path render (instruments-table Render closure).
			fastBuf := make([]float32, n)
			tc.fast(fastBuf, sr, n)
			fastHash := hashFloat32(fastBuf)

			if recipeHash != fastHash {
				t.Errorf("%s: no-edit fast path diverged from recipe-default render\n  fast:   %s\n  recipe: %s\n(the baked ModularParams must reproduce the recipe path bit-for-bit)", tc.recipeID, fastHash, recipeHash)
			}
		})
	}
}

// TestKickInstrumentTableUsesModularFastPath asserts every live kick instrument
// renders a non-silent voice through the table — proving its Render path is the
// modular fast path, not a dead/deleted legacy C wrapper. kick / kick-deep keep
// their dedicated Kick{} / KickDeep{} struct types (fixed-second duration model)
// with renderKickVoice / renderKickDeepVoice swapped in; the three variants are
// CVariantInstruments. So this introspects via the Instrument.NewVoice contract
// rather than asserting a concrete type. Mirrors the intent of
// TestBassInstrumentTableUsesModularFastPath (bass was already all CVariant).
func TestKickInstrumentTableUsesModularFastPath(t *testing.T) {
	ResetInstruments()
	instMu.RLock()
	defer instMu.RUnlock()
	for _, id := range []string{"kick", "kick-deep", "kick-1", "kick-2", "kick-tight"} {
		inst, ok := instruments[id]
		if !ok {
			t.Errorf("instrument %q missing from table", id)
			continue
		}
		// A no-edit voice must produce non-silence (the modular voice is live).
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
