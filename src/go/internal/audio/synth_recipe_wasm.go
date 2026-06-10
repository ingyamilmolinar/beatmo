//go:build js && wasm

package audio

import (
	"encoding/json"
	"syscall/js"
)

// Phase 5: WASM bridge for per-instrument synth-recipe params. Every
// SetInstrumentParam / SetInstrumentParams / ResetInstrumentParams call
// in the audio manager funnels through the platformInstrumentParamsChanged
// hook; in browser builds we forward the new param map to JS so the audio
// pipeline (voice-buffer cache + the per-instrument param dictionary used
// by the next play) stays in sync.
//
// Mirrors insert_effects_wasm.go's pattern: JSON-marshal the payload and
// invoke a global JS function. The JS side (audio.js:updateInstrumentParams)
// decodes + replays the change against the in-browser voice cache.
func init() {
	platformInstrumentParamsChanged = func(id string, params RecipeParams) {
		fn := js.Global().Get("updateInstrumentParams")
		if !fn.Truthy() {
			return
		}
		// Family migration (Phase-2): the browser renders migrated instruments
		// through render_modular_p (paramBlock:'modular'), so the legacy-named
		// overlay (bass_sustain, fundamental, …) must be translated to the
		// binding's fully-resolved modular-named block at this seam. Non-migrated
		// instruments push their params untranslated. This is the ONLY place the
		// rename happens — prefs / export / UI keep legacy names.
		push := params
		if translated, ok := modularPushParamsForInstrument(id, params); ok {
			push = translated
		}
		data, err := json.Marshal(push)
		if err != nil {
			return
		}
		fn.Invoke(id, string(data))
	}

	// Non-destructive sample-edit descriptor push (sample_edit_descriptor.go).
	// Set forwards the edit; Clear forwards the identity edit so the JS side
	// (audio.js:updateSampleEdit) deletes its entry and re-renders un-edited.
	platformSampleEditChanged = func(id string, e SampleEdit) {
		fn := js.Global().Get("updateSampleEdit")
		if !fn.Truthy() {
			return
		}
		data, err := json.Marshal(e)
		if err != nil {
			return
		}
		fn.Invoke(id, string(data))
	}

	// Recipe-binding push (see synth_recipe_binding_hook.go). A rebinding
	// (MigrateGenType at import, or a project whose instrument carries a
	// recipe differing from the static JS table) must repoint the JS render
	// function or the browser keeps playing the old voice. The exemplar id
	// tells audio.js which static RENDER/RENDER_INFO entries to copy.
	platformInstrumentRecipeChanged = func(id, recipeID, exemplar string) {
		fn := js.Global().Get("updateInstrumentRecipe")
		if !fn.Truthy() {
			return
		}
		fn.Invoke(id, recipeID, exemplar)
	}

	// Bootstrap defaults push (see modular_defaults_seed.go). A divergent
	// preset (modular-pad) renders correctly in the browser only if JS knows
	// its default param block before the user edits anything. seedInstrumentDefaults
	// stores it in a defaults map renderToCache falls back to (distinct from
	// the user-overlay map, so Reset stays coherent).
	platformInstrumentDefaultsPush = func(id string, params RecipeParams) {
		fn := js.Global().Get("seedInstrumentDefaults")
		if !fn.Truthy() {
			return
		}
		// Same migration translation as the params-changed hook: a migrated
		// instrument's SHIPPED defaults must reach the browser as the modular-named
		// block so the unedited modular voice renders (the legacy family block is
		// gone). params here is RecipeDefaultParams(recipe), i.e. the full default
		// set — modularPushParamsForInstrument merges + translates.
		push := params
		if translated, ok := modularPushParamsForInstrument(id, params); ok {
			push = translated
		}
		data, err := json.Marshal(push)
		if err != nil {
			return
		}
		fn.Invoke(id, string(data))
	}
}
