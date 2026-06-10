package audio

import "testing"

// The recipe-binding platform hook closes the WASM gap where an imported
// (or migrated) recipe rebinding reached the Go manager but never the JS
// renderer: audio.js selects the render function from a static
// instrument-id table, so a snare rebound to synth-modular kept rendering
// the snare. BindInstrumentToRecipe now fires
// platformInstrumentRecipeChanged with a deterministic exemplar builtin
// instrument id the JS side copies its RENDER/RENDER_INFO entries from.

func TestBindInstrumentToRecipe_FiresPlatformHook(t *testing.T) {
	type call struct{ id, recipe, exemplar string }
	var calls []call
	prev := SwapPlatformInstrumentRecipeChangedForTest(func(id, recipeID, exemplar string) {
		calls = append(calls, call{id, recipeID, exemplar})
	})
	t.Cleanup(func() { SwapPlatformInstrumentRecipeChangedForTest(prev) })

	BindInstrumentToRecipe("snare", "synth-modular")
	t.Cleanup(func() { BindInstrumentToRecipe("snare", "drum-snare") })

	if len(calls) != 1 {
		t.Fatalf("hook fired %d times, want 1 (calls=%v)", len(calls), calls)
	}
	got := calls[0]
	if got.id != "snare" || got.recipe != "synth-modular" {
		t.Errorf("hook payload = %+v, want id=snare recipe=synth-modular", got)
	}
	// synth-modular's exemplar builtin instrument is "modular" — the id whose
	// static JS RENDER entry carries render_modular + the modular paramBlock.
	if got.exemplar != "modular" {
		t.Errorf("exemplar = %q, want %q", got.exemplar, "modular")
	}
}

func TestBindInstrumentToRecipe_ClearFiresHookWithEmptyExemplar(t *testing.T) {
	type call struct{ id, recipe, exemplar string }
	var calls []call
	prev := SwapPlatformInstrumentRecipeChangedForTest(func(id, recipeID, exemplar string) {
		calls = append(calls, call{id, recipeID, exemplar})
	})
	t.Cleanup(func() { SwapPlatformInstrumentRecipeChangedForTest(prev) })

	BindInstrumentToRecipe("snare", "")
	t.Cleanup(func() { BindInstrumentToRecipe("snare", "drum-snare") })

	if len(calls) != 1 {
		t.Fatalf("hook fired %d times, want 1", len(calls))
	}
	if calls[0].recipe != "" || calls[0].exemplar != "" {
		t.Errorf("clear-binding hook payload = %+v, want empty recipe+exemplar", calls[0])
	}
}

func TestRecipeExemplarInstrument_Deterministic(t *testing.T) {
	cases := map[string]string{
		"synth-modular":     "modular",
		"synth-modular-pad": "modular-pad",
		"drum-kick-punchy":  "kick-1",
		// drum-snare binds several variants (snare, snare-1, snare-2,
		// snare-ghost); the exemplar is the lexicographically smallest so the
		// push is deterministic across map-iteration orders.
		"drum-snare": "snare",
		// Unknown recipe (user Save-As clone) has no builtin exemplar.
		"user-clone-xyz": "",
	}
	for recipe, want := range cases {
		if got := recipeExemplarInstrument(recipe); got != want {
			t.Errorf("recipeExemplarInstrument(%q) = %q, want %q", recipe, got, want)
		}
	}
}
