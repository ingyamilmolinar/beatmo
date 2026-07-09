//go:build !test && !js

package audio

import "testing"

// TestModularNoEditFastPathMatchesRecipe generalizes the sax fix
// (TestSaxNoEditFastPathMatchesRecipe) across every SEEDED modular melodic /
// percussion instrument. The instrument-table Render is what
// RenderInstrumentOneShotRaw plays for the as-shipped (no-edit) instrument —
// the Sampler tab's preview/"From Synth" capture source. For each seeded
// instrument it MUST reproduce the recipe-default render (the seed merged over
// the modular identity) bit-for-bit. Before the blanket bake pass these entries
// were the BARE renderModular, so their previews played the unseeded ~220 Hz
// modular voice ("a simple tone"), their goldens pinned that bare render, and
// distinct instruments (e.g. guitar-steel-warm / guitar-nylon-bright) shared a
// checksum. In-game node playback was always fine — melodics take the pitched
// recipe path, which merges the seed unconditionally.
func TestModularNoEditFastPathMatchesRecipe(t *testing.T) {
	Reset()
	ResetInstruments()

	const sr = nativeGoldenSR
	const n = nativeGoldenSamples

	// Every seeded modular instrument baked by the blanket pass (+ sax). The bare
	// "modular" voice is intentionally excluded — it has no seed and correctly
	// renders the modular identity defaults.
	ids := []string{
		"violin", "violin-ensemble", "cello", "cello-warm", "organ-church", "scifi-lead",
		"guitar-nylon", "guitar-nylon-bright", "guitar-steel", "guitar-steel-warm",
		"guitar-electric", "harp", "guitar-electric-neck", "piano-grand", "piano-felt",
		"flute", "flute-breathy", "oboe", "oboe-full", "trumpet", "trumpet-mellow",
		"french-horn", "french-horn-loud", "bass-guitar", "bass-acid", "bass-reese",
		"bass-fm", "bass-808", "conga", "conga-open", "conga-tumba", "organ", "sax",
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			inst, ok := instruments[id].(CVariantInstrument)
			if !ok {
				t.Fatalf("instruments[%q] is not a CVariantInstrument", id)
			}
			recipeID := RecipeForInstrument(id)
			if recipeID == "" {
				t.Fatalf("%s: no recipe binding", id)
			}
			r := NewRecipe(recipeID)
			if r == nil {
				t.Fatalf("NewRecipe(%q) returned nil", recipeID)
			}
			recipeBuf := make([]float32, n)
			r.Render(recipeBuf, sr, n, 0, MergeRecipeDefaults(recipeID, RecipeParams{}))
			recipeHash := hashFloat32(recipeBuf)

			fastBuf := make([]float32, n)
			inst.Render(fastBuf, sr, n)
			fastHash := hashFloat32(fastBuf)

			if recipeHash != fastHash {
				t.Errorf("%s: no-edit fast path diverged from recipe-default render\n  fast:   %s\n  recipe: %s\n(the table Render must bake the seed — bare renderModular plays the unseeded voice in the Sampler preview)", id, fastHash, recipeHash)
			}
		})
	}
}
