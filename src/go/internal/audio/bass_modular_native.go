//go:build !test && !js

package audio

// bass_modular_native.go holds the native (CGo) no-edit fast-path renderers for
// the migrated bass family. After the Phase-2 cutover, the instruments-table
// Render closures for bass-guitar / sub-bass no longer call the deleted bespoke
// render_bass_guitar / render_sub_bass C functions; instead they bake the
// recipe-default ModularParams ONCE (via the same bassRecipeToModular /
// bassGuitarRecipeToModular binding the recipe/edit path uses) and render them
// through render_modular_p. This keeps the no-edit dispatch path byte-identical
// to the recipe path for an unedited instrument — the exact renderModularPad
// pattern (modular_c.go).
//
// The baked params are built from the recipe DEFAULTS merged over an empty
// overlay, so they reproduce the exact param block the recipe path renders for
// an instrument the user has never touched. TestBassNoEditFastPathMatchesRecipe
// (bass_native_fastpath_test.go) locks the byte-identity; the existing
// nativeGoldenCases bass entries (re-pointed at these renderers) prove the hash
// is unchanged from the deleted legacy path.

// subBassVoiceParams / bassGuitarVoiceParams are the baked ModularParams for the
// no-edit fast path. Built once at init from the recipe defaults through the
// migration binding, mirroring modularPadVoiceParams.
var subBassVoiceParams = bassRecipeToModular("drum-sub-bass", MergeRecipeDefaults("drum-sub-bass", RecipeParams{}))

var bassGuitarVoiceParams = bassGuitarRecipeToModular("drum-bass-guitar", MergeRecipeDefaults("drum-bass-guitar", RecipeParams{}))

// renderSubBassVoice renders the sub-bass no-edit voice through the modular
// engine with its baked recipe-default params (replaces the deleted
// render_sub_bass legacy fast path).
func renderSubBassVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, subBassVoiceParams)
}

// renderBassGuitarVoice renders the bass-guitar no-edit voice through the
// modular engine with its baked recipe-default params (replaces the deleted
// render_bass_guitar legacy fast path).
func renderBassGuitarVoice(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, bassGuitarVoiceParams)
}
