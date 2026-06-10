//go:build !test && !js

package audio

import "testing"

// Phase-7 FM-family migration (native no-edit fast-path cutover, the LAST family).
//
// The instruments-table Render closure for an unedited FM-family instrument must
// produce a buffer byte-identical to the recipe path's default render. Before the
// cutover the table pointed Render at the legacy render_fm_bass / render_fm_bell /
// render_fm_lead / render_fm_epiano / render_fm_pluck C functions; after it,
// Render bakes the recipe-default ModularParams and renders via render_modular_p
// (renderFMBassVoice / renderFMBellVoice / renderFMLeadVoice / renderFMEPianoVoice
// / renderFMPluckVoice). This test proves the baked fast path equals the recipe
// path so the cheap no-overlay dispatch and the recipe/edit dispatch agree to the
// bit — the same invariant nativeGoldenCases + TestFMPresetGolden lock via the
// golden hash. Mirrors cymbal_native_fastpath_test.go.

func TestFMNoEditFastPathMatchesRecipe(t *testing.T) {
	const sr = nativeGoldenSR
	const n = nativeGoldenSamples
	for _, tc := range []struct {
		recipeID string
		fast     func(buf []float32, sampleRate, samples int)
	}{
		{"fm-bass", renderFMBassVoice},
		{"fm-bell", renderFMBellVoice},
		{"fm-lead", renderFMLeadVoice},
		{"fm-epiano", renderFMEPianoVoice},
		{"fm-pluck", renderFMPluckVoice},
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

// TestFMInstrumentTableUsesModularFastPath asserts every live FM-family
// instrument renders a non-silent voice through the table — proving its Render
// path is the modular fast path, not a dead/deleted legacy C wrapper. Mirrors
// TestCymbalInstrumentTableUsesModularFastPath.
func TestFMInstrumentTableUsesModularFastPath(t *testing.T) {
	ResetInstruments()
	instMu.RLock()
	defer instMu.RUnlock()
	for _, id := range []string{
		"fm-bass", "fm-bell", "fm-lead", "fm-epiano", "fm-pluck",
		"fm-bass-1", "fm-bell-1", "fm-lead-1", "fm-epiano-1", "fm-pluck-1",
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
