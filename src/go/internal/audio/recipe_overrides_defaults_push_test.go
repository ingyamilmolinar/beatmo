package audio

import (
	"math"
	"testing"
)

// A Save (UpdateRecipeDefaultsAndInvalidate) or a startup override reload
// (ApplyUserRecipeOverrides) mutates the recipe's registered defaults — which
// the desktop dispatch renders directly. The browser only sees what is pushed
// over the bridge, so both sites must forward the new defaults to
// platformInstrumentDefaultsPush for every bound instrument; otherwise a
// cleared overlay reverts the WASM render to shipped while desktop keeps the
// saved tone (the post-restart / post-Save divergence).

func TestUpdateRecipeDefaults_PushesDefaultsToPlatform(t *testing.T) {
	pushes := map[string]RecipeParams{}
	prev := SwapPlatformInstrumentDefaultsPushForTest(func(id string, p RecipeParams) {
		pushes[id] = p
	})
	t.Cleanup(func() { SwapPlatformInstrumentDefaultsPushForTest(prev) })

	shipped := RecipeShippedDefaults("drum-kick-punchy")
	want := shipped["decay"] + 1.5
	if !UpdateRecipeDefaultsAndInvalidate("drum-kick-punchy", map[string]float64{"decay": want}) {
		t.Fatal("precondition: UpdateRecipeDefaultsAndInvalidate changed nothing")
	}
	t.Cleanup(func() { ResetRecipeToShipped("drum-kick-punchy") })

	// kick-1 is the builtin instrument bound to drum-kick-punchy.
	got, ok := pushes["kick-1"]
	if !ok {
		t.Fatalf("Save did not push defaults for bound instrument kick-1 (pushes=%v)", pushes)
	}
	if math.Abs(got["decay"]-want) > 1e-9 {
		t.Errorf("pushed decay = %v, want %v (the saved default)", got["decay"], want)
	}
}

func TestApplyUserRecipeOverrides_PushesDefaultsToPlatform(t *testing.T) {
	restoreGate := SwapUseUserRecipeOverridesForTest(true)
	t.Cleanup(func() { SwapUseUserRecipeOverridesForTest(restoreGate) })

	pushes := map[string]RecipeParams{}
	prev := SwapPlatformInstrumentDefaultsPushForTest(func(id string, p RecipeParams) {
		pushes[id] = p
	})
	t.Cleanup(func() { SwapPlatformInstrumentDefaultsPushForTest(prev) })

	shipped := RecipeShippedDefaults("drum-kick-punchy")
	want := shipped["decay"] + 1.25
	src := stubUserRecipeSource{overrides: map[string]map[string]float64{
		"drum-kick-punchy": {"decay": want},
	}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("ApplyUserRecipeOverrides: %v", err)
	}
	t.Cleanup(func() { ResetRecipeToShipped("drum-kick-punchy") })

	got, ok := pushes["kick-1"]
	if !ok {
		t.Fatalf("override reload did not push defaults for bound instrument kick-1 (pushes=%v)", pushes)
	}
	if math.Abs(got["decay"]-want) > 1e-9 {
		t.Errorf("pushed decay = %v, want %v (the reloaded default)", got["decay"], want)
	}
}

type stubUserRecipeSource struct {
	overrides map[string]map[string]float64
}

func (s stubUserRecipeSource) LoadUserRecipes() (map[string][]byte, error) { return nil, nil }
func (s stubUserRecipeSource) LoadRecipeOverrides() (map[string]map[string]float64, error) {
	return s.overrides, nil
}
