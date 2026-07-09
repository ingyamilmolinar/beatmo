//go:build !test && !js

package audio

import "testing"

// TestSaxNoEditFastPathMatchesRecipe pins the Sampler-preview bug: the sax's
// instrument-table Render is what RenderInstrumentOneShotRaw plays for the
// as-shipped (no-edit) instrument — the Sampler tab's preview/capture source.
// It must reproduce the recipe path (saxSeed merged over the modular identity)
// bit-for-bit, exactly like the seeded kicks' bakedModularRender contract
// (TestSnareNoEditFastPathMatchesRecipe). Before the fix the entry was the
// BARE renderModular, so the preview played the unseeded ~220 Hz modular
// voice — "a simple tone", not the tuned baritone sax.
func TestSaxNoEditFastPathMatchesRecipe(t *testing.T) {
	Reset()
	ResetInstruments()

	const sr = nativeGoldenSR
	const n = nativeGoldenSamples

	inst, ok := instruments["sax"].(CVariantInstrument)
	if !ok {
		t.Fatalf(`instruments["sax"] is not a CVariantInstrument`)
	}

	recipeBuf := make([]float32, n)
	r := NewRecipe("synth-modular-sax")
	if r == nil {
		t.Fatal(`NewRecipe("synth-modular-sax") returned nil`)
	}
	r.Render(recipeBuf, sr, n, 0, MergeRecipeDefaults("synth-modular-sax", RecipeParams{}))
	recipeHash := hashFloat32(recipeBuf)

	fastBuf := make([]float32, n)
	inst.Render(fastBuf, sr, n)
	fastHash := hashFloat32(fastBuf)

	if recipeHash != fastHash {
		t.Errorf("sax: no-edit fast path diverged from recipe-default render\n  fast:   %s\n  recipe: %s\n(the table Render must bake saxSeed — bare renderModular plays the unseeded voice in the Sampler preview)", fastHash, recipeHash)
	}
}
