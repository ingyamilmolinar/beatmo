package audio

// Recipe-binding platform hook.
//
// On native, a rebound instrument renders correctly because the dispatch
// (tryRecipeVoice) resolves the recipe through the live binding table at
// trigger time. In the browser, audio.js selects the render function from a
// STATIC instrument-id table (RENDER / RENDER_INFO), so a binding change that
// only lives in the Go manager — a MigrateGenType re-voice at import, or any
// project whose instrument carries a recipe differing from the static table —
// kept rendering the old voice. BindInstrumentToRecipe therefore fires this
// hook; the WASM build (synth_recipe_wasm.go) forwards it to JS
// (audio.js:updateInstrumentRecipe), which repoints RENDER/RENDER_INFO at the
// exemplar's entries and invalidates the render cache.
//
// The exemplar is a builtin instrument id bound to the recipe in
// builtinInstrumentRecipeBindings — the id whose static JS tables already
// carry the recipe's render function + param block. Pushing an exemplar id
// instead of a render-function name keeps audio.js's tables the single source
// of truth (no second recipe→renderer map to drift). Empty when the recipe
// has no builtin exemplar (user Save-As clones): JS then leaves the static
// table untouched.
//
// Default is a no-op (native needs nothing); tests swap it via
// SwapPlatformInstrumentRecipeChangedForTest, mirroring
// platformInstrumentParamsChanged (synth_recipe.go:172).
var platformInstrumentRecipeChanged = func(instrumentID, recipeID, exemplarInstrumentID string) {}

// SwapPlatformInstrumentRecipeChangedForTest replaces the recipe-binding
// platform hook and returns the previous value, mirroring
// SwapPlatformInstrumentParamsChangedForTest.
func SwapPlatformInstrumentRecipeChangedForTest(fn func(instrumentID, recipeID, exemplarInstrumentID string)) func(instrumentID, recipeID, exemplarInstrumentID string) {
	prev := platformInstrumentRecipeChanged
	if fn != nil {
		platformInstrumentRecipeChanged = fn
	}
	return prev
}

// recipeExemplarInstrument returns the lexicographically smallest builtin
// instrument id bound to recipeID (deterministic across map-iteration
// orders), or "" when no builtin instrument ships with that recipe.
func recipeExemplarInstrument(recipeID string) string {
	if recipeID == "" {
		return ""
	}
	best := ""
	for instID, bound := range builtinInstrumentRecipeBindings {
		if bound != recipeID {
			continue
		}
		if best == "" || instID < best {
			best = instID
		}
	}
	return best
}
