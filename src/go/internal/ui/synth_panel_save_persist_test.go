package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthSave_KeepsLikedParamsForBridge asserts that Save does NOT push an
// empty parameter map to the platform bridge. On WASM the only way the
// WebAudio voice cache learns the active params is via this callback
// (platformInstrumentParamsChanged → updateInstrumentParams); if Save clears
// the per-instrument overlay it pushes "{}", the browser drops the liked tone,
// and the sound reverts. Save must leave the liked params in place.
//
// Pre-fix: SaveActiveRecipe calls ResetInstrumentParams, which pushes an empty
// map — this test fails. Post-fix: no empty push during Save.
func TestSynthSave_KeepsLikedParamsForBridge(t *testing.T) {
	const recipeID = "drum-snare"
	instID := captureBoundInstrument(recipeID)
	if instID == "" {
		t.Fatalf("no shipped instrument bound to %q (fixture broken)", recipeID)
	}

	// Restore the shipped default + clear overlay on cleanup (process-global).
	var origDecay float64
	for _, p := range audio.RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			origDecay = p.Default
		}
	}
	t.Cleanup(func() {
		audio.UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": origDecay})
		audio.ResetInstrumentParams(instID)
	})

	withStubSink(t)

	// The user drags the decay knob (this legitimately pushes {decay:2.0}).
	audio.SetInstrumentParam(instID, "decay", 2.0)

	// Now record only the pushes that happen DURING Save.
	var mu sync.Mutex
	var pushedEmptyDuringSave bool
	prev := audio.SwapPlatformInstrumentParamsChangedForTest(func(id string, p audio.RecipeParams) {
		if id == instID && len(p) == 0 {
			mu.Lock()
			pushedEmptyDuringSave = true
			mu.Unlock()
		}
	})
	t.Cleanup(func() { audio.SwapPlatformInstrumentParamsChangedForTest(prev) })

	dv := setSynthActiveInstrumentForTest(instID)
	if got := dv.SaveActiveRecipe(); got != recipeID {
		t.Fatalf("SaveActiveRecipe = %q, want %q", got, recipeID)
	}

	mu.Lock()
	empty := pushedEmptyDuringSave
	mu.Unlock()
	if empty {
		t.Error("Save pushed an empty param map to the bridge — the browser would drop the liked sound")
	}

	// Belt-and-suspenders: the Save button should un-highlight (not dirty),
	// because the persisted defaults now match the active params.
	if audio.InstrumentParamsDiffer(instID, recipeID) {
		t.Error("instrument still reports dirty after Save (Save button would stay highlighted)")
	}
}

// TestSynthReset_RestoresOriginalAfterSave asserts that Reset returns a recipe
// to its original shipped configuration even after a Save overwrote the
// defaults. Pre-fix, Reset only cleared the per-instrument overlay and left the
// saved defaults in place, so the original tone was unrecoverable.
func TestSynthReset_RestoresOriginalAfterSave(t *testing.T) {
	const recipeID = "drum-snare"
	instID := captureBoundInstrument(recipeID)
	if instID == "" {
		t.Fatalf("no shipped instrument bound to %q (fixture broken)", recipeID)
	}

	// Force a clean baseline and restore it on cleanup (process-global registry).
	audio.ResetRecipeToShipped(recipeID)
	audio.ResetInstrumentParams(instID)
	orig := audio.RecipeDefaultParams(recipeID)
	t.Cleanup(func() {
		audio.ResetRecipeToShipped(recipeID)
		audio.ResetInstrumentParams(instID)
	})

	withStubSink(t)
	dv := setSynthActiveInstrumentForTest(instID)

	// Edit + Save → recipe defaults are overwritten.
	audio.SetInstrumentParam(instID, "decay", 2.0)
	dv.SaveActiveRecipe()
	if audio.RecipeDefaultParams(recipeID)["decay"] == orig["decay"] {
		t.Fatalf("sanity: Save did not change the registered default (decay still %v)", orig["decay"])
	}

	// Reset → must restore the original shipped defaults AND clear the overlay.
	dv.ResetActiveRecipe()

	got := audio.RecipeDefaultParams(recipeID)
	for name, want := range orig {
		if got[name] != want {
			t.Errorf("after Reset, default %q = %v, want original %v", name, got[name], want)
		}
	}
	if overlay := audio.GetInstrumentParams(instID); len(overlay) != 0 {
		t.Errorf("after Reset, per-instrument overlay still has %v, want empty", overlay)
	}
	if audio.RecipeDefaultsCustomized(recipeID) {
		t.Error("after Reset, recipe still reports customized — original not fully restored")
	}
}
