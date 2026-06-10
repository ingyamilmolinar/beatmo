package audio

import (
	"errors"
	"math"
	"testing"
)

// Phase 13: plugin substrate end-to-end. Registers a SineProvider at runtime
// and asserts the recipe is reachable via NewRecipe(id), renders audio
// through the standard Render() entry point, and honors per-instrument
// params via MergeRecipeDefaults.
func TestPluginRecipeRoundtrip(t *testing.T) {
	const id = "plugin-sine-roundtrip"
	t.Cleanup(func() { UnregisterRecipeForTest(id) })

	err := RegisterPluginRecipe(PluginRecipeOptions{
		ID:          id,
		DisplayName: "Sine Plugin",
		Category:    "plugin",
		ParamDefs: []ParamDef{
			{Name: "freq", Min: 50, Max: 2000, Default: 220, Unit: "Hz"},
			{Name: "decay", Min: 0.5, Max: 20, Default: 4},
		},
		Provider: SineProvider{BaseFreqHz: 220},
	})
	if err != nil {
		t.Fatalf("RegisterPluginRecipe: %v", err)
	}

	r := NewRecipe(id)
	if r == nil {
		t.Fatalf("NewRecipe(%q) returned nil after registration", id)
	}
	if r.ID() != id {
		t.Errorf("recipe ID = %q, want %q", r.ID(), id)
	}
	if r.DisplayName() != "Sine Plugin" {
		t.Errorf("DisplayName = %q, want %q", r.DisplayName(), "Sine Plugin")
	}
	if r.Category() != "plugin" {
		t.Errorf("Category = %q, want %q", r.Category(), "plugin")
	}
	schema := r.ParamSchema()
	if len(schema) != 2 || schema[0].Name != "freq" {
		t.Errorf("ParamSchema mismatch: got %+v", schema)
	}

	// Render at default params — must produce non-silent audio.
	const sr = 44100
	const samples = sr / 4
	buf := make([]float32, samples)
	r.Render(buf, sr, samples, 0, RecipeDefaultParams(id))
	if !audioHasEnergy(buf) {
		t.Error("default render produced silence")
	}

	// Render at non-default freq — must differ from default render.
	other := make([]float32, samples)
	r.Render(other, sr, samples, 0, RecipeParams{"freq": 880})
	var diffSq float64
	for i := range buf {
		d := float64(buf[i] - other[i])
		diffSq += d * d
	}
	if diffSq < 1e-6 {
		t.Errorf("freq=880 render did not differ from default (diffSq=%g)", diffSq)
	}
}

func TestRegisterPluginRecipe_ValidatesInput(t *testing.T) {
	cases := []struct {
		name string
		opts PluginRecipeOptions
	}{
		{"missing-id", PluginRecipeOptions{Provider: SineProvider{}}},
		{"missing-provider", PluginRecipeOptions{ID: "plugin-x"}},
		{"bad-paramdef", PluginRecipeOptions{
			ID:        "plugin-bad-param",
			Provider:  SineProvider{},
			ParamDefs: []ParamDef{{Name: "x", Min: 1, Max: 0, Default: 0}}, // Min > Max
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RegisterPluginRecipe(c.opts)
			if err == nil {
				t.Fatalf("want error for %s, got nil", c.name)
			}
			if !errors.Is(err, ErrInvalidPluginRecipe) {
				t.Errorf("want ErrInvalidPluginRecipe, got %v", err)
			}
		})
	}
}

// TestPluginRecipe_BindsToInstrumentAndRoutesParams covers the path
// SetInstrumentParam → MergeRecipeDefaults → recipe.Render so a plugin
// registered at runtime sees user-edited params just like a builtin does.
func TestPluginRecipe_BindsToInstrumentAndRoutesParams(t *testing.T) {
	const recipeID = "plugin-binding-test"
	const instID = "plugin-binding-inst"
	t.Cleanup(func() {
		UnregisterRecipeForTest(recipeID)
		ResetInstrumentParams(instID)
		BindInstrumentToRecipe(instID, "")
	})

	err := RegisterPluginRecipe(PluginRecipeOptions{
		ID: recipeID,
		ParamDefs: []ParamDef{
			{Name: "freq", Min: 50, Max: 2000, Default: 220, Unit: "Hz"},
		},
		Provider: SineProvider{BaseFreqHz: 220},
	})
	if err != nil {
		t.Fatalf("RegisterPluginRecipe: %v", err)
	}

	BindInstrumentToRecipe(instID, recipeID)
	SetInstrumentParam(instID, "freq", 660)

	// The merged params flow through the same MergeRecipeDefaults path the
	// builtin recipes use.
	merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(instID))
	if merged["freq"] != 660 {
		t.Errorf("merged freq = %v, want 660", merged["freq"])
	}

	r := NewRecipe(recipeID)
	const sr = 44100
	const samples = sr / 8
	withUser := make([]float32, samples)
	r.Render(withUser, sr, samples, 0, merged)

	withDefault := make([]float32, samples)
	r.Render(withDefault, sr, samples, 0, RecipeDefaultParams(recipeID))

	var diffSq float64
	for i := range withUser {
		d := float64(withUser[i] - withDefault[i])
		diffSq += d * d
	}
	if diffSq < 1e-6 {
		t.Error("user param did not change render output through the plugin path")
	}
}

func audioHasEnergy(buf []float32) bool {
	var sumSq float64
	for _, v := range buf {
		sumSq += float64(v) * float64(v)
	}
	return sumSq > 1e-6 && !math.IsNaN(sumSq) && !math.IsInf(sumSq, 0)
}
