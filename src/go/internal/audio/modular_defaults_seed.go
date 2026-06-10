package audio

// Bootstrap defaults push.
//
// On native, a preset whose defaults diverge from the engine's plain render
// (e.g. modular-pad) renders correctly because its instrument Render closure
// bakes the preset. In the browser, an instrument with no user edits renders
// through render_modular with NO param block, so it would sound like the base
// voice. To fix that without duplicating defaults in JS, Go pushes each seeded
// preset's shipped recipe-default params to the platform layer once at
// bootstrap; the browser stores them in a defaults map that renderToCache
// falls back to when there is no user overlay (so Reset stays coherent — it
// clears the overlay and the defaults map still supplies the preset).

// platformInstrumentDefaultsPush forwards an instrument's shipped recipe-default
// param block to the platform layer. Default is a no-op (native bakes the
// preset into its Render closure); synth_recipe_wasm.go overrides it to invoke
// window.seedInstrumentDefaults. Tests swap it via
// SwapPlatformInstrumentDefaultsPushForTest.
var platformInstrumentDefaultsPush = func(id string, p RecipeParams) {}

// SwapPlatformInstrumentDefaultsPushForTest replaces the defaults-push hook and
// returns the previous value, mirroring SwapPlatformInstrumentParamsChangedForTest.
func SwapPlatformInstrumentDefaultsPushForTest(fn func(id string, p RecipeParams)) func(id string, p RecipeParams) {
	prev := platformInstrumentDefaultsPush
	if fn != nil {
		platformInstrumentDefaultsPush = fn
	}
	return prev
}

// seededRecipeIDs returns the set of builtin recipe ids whose default ParamDefs
// diverge from the schema identity via a descriptor Seed. Their bound
// instruments need a browser bootstrap push of their defaults.
func seededRecipeIDs() map[string]bool {
	out := make(map[string]bool)
	for _, d := range builtinRecipeDescriptors {
		if len(d.Seed) > 0 {
			out[d.ID] = true
		}
	}
	return out
}

// SeedInstrumentDefaultsToPlatform pushes the shipped recipe-default params of
// every builtin instrument bound to a seeded recipe to the platform layer.
// Called once at bootstrap so divergent presets (modular-pad) render their
// intended sound before the user touches a knob. Idempotent. On native the
// hook is a no-op — the instrument's Render closure already carries the preset.
func SeedInstrumentDefaultsToPlatform() {
	seeded := seededRecipeIDs()
	for instID, recipeID := range builtinInstrumentRecipeBindings {
		// Migrated families MUST seed their browser defaults too: their JS
		// RENDER_INFO is now paramBlock:'modular', so an unedited migrated
		// instrument renders through render_modular_p and needs the binding-
		// translated modular default block (otherwise the all-identity modular
		// block silences the voice — no gen slot, osc off). The defaults-push hook
		// translates the legacy-named defaults via modularPushParamsForInstrument
		// at the seam.
		if seeded[recipeID] || IsModularMigratedRecipe(recipeID) {
			platformInstrumentDefaultsPush(instID, RecipeDefaultParams(recipeID))
		}
	}
}
