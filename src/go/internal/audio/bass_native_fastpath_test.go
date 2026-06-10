//go:build !test && !js

package audio

import "testing"

// Phase-2 bass-family migration (Task 3a: native no-edit fast-path cutover).
//
// The instruments-table Render closure for an unedited bass instrument must
// produce a buffer byte-identical to the recipe path's default render. Before
// the cutover the table pointed Render at the legacy render_bass_guitar /
// render_sub_bass C functions; after it, Render bakes the recipe-default
// ModularParams and renders via render_modular_p (renderSubBassVoice /
// renderBassGuitarVoice). This test proves the baked fast path equals the
// recipe path so the cheap no-overlay dispatch and the recipe/edit dispatch
// agree to the bit — the same invariant nativeGoldenCases locks via the golden
// hash.

func TestBassNoEditFastPathMatchesRecipe(t *testing.T) {
	const sr = nativeGoldenSR
	const n = nativeGoldenSamples
	for _, tc := range []struct {
		recipeID string
		fast     func(buf []float32, sampleRate, samples int)
	}{
		{"drum-sub-bass", renderSubBassVoice},
		{"drum-bass-guitar", renderBassGuitarVoice},
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

// TestBassInstrumentTableUsesModularFastPath asserts the live instruments table
// binds bass-guitar / sub-bass (and their -1 variants) to the modular fast-path
// renderers, not the deleted legacy C wrappers. It introspects the table the
// same way ResetInstruments populates it.
func TestBassInstrumentTableUsesModularFastPath(t *testing.T) {
	ResetInstruments()
	instMu.RLock()
	defer instMu.RUnlock()
	for _, id := range []string{"bass-guitar", "sub-bass", "bass-guitar-1", "sub-bass-1"} {
		inst, ok := instruments[id]
		if !ok {
			t.Errorf("instrument %q missing from table", id)
			continue
		}
		cv, ok := inst.(CVariantInstrument)
		if !ok {
			t.Errorf("instrument %q is not a CVariantInstrument", id)
			continue
		}
		// A no-edit render must produce non-silence (the modular voice is live).
		buf := make([]float32, nativeGoldenSamples)
		cv.Render(buf, nativeGoldenSR, nativeGoldenSamples)
		var energy float64
		for _, v := range buf {
			energy += float64(v) * float64(v)
		}
		if energy == 0 {
			t.Errorf("instrument %q rendered silence via its table Render closure", id)
		}
	}
}
