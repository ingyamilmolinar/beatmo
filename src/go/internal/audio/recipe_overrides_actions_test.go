package audio

import (
	"encoding/json"
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUseUserRecipeOverridesRoundTrip(t *testing.T) {
	prev := SwapUseUserRecipeOverridesForTest(true)
	t.Cleanup(func() { SwapUseUserRecipeOverridesForTest(prev) })
	if !UseUserRecipeOverrides() {
		t.Errorf("after Swap(true), UseUserRecipeOverrides() = false")
	}
	SetUseUserRecipeOverrides(false)
	if UseUserRecipeOverrides() {
		t.Errorf("after SetUseUserRecipeOverrides(false), UseUserRecipeOverrides() = true")
	}
	SetUseUserRecipeOverrides(true)
	if !UseUserRecipeOverrides() {
		t.Errorf("after SetUseUserRecipeOverrides(true), UseUserRecipeOverrides() = false")
	}
}

func TestApply_DocWithEmptyIDInheritsKey(t *testing.T) {
	const userID = "user.test.empty-id-key"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })
	withGate(t, true)

	doc := RecipeDoc{
		ID:         "",
		BaseRecipe: "drum-snare",
		Origin:     OriginUser,
	}
	raw, _ := json.Marshal(doc)
	src := &stubRecipeSource{recipes: map[string][]byte{userID: raw}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if NewRecipe(userID) == nil {
		t.Errorf("user recipe with empty ID should have inherited map key %q", userID)
	}
}

func TestApply_DocWithEmptyBaseRecipeSkipped(t *testing.T) {
	const userID = "user.test.empty-base"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })
	withGate(t, true)

	doc := RecipeDoc{
		ID:         userID,
		BaseRecipe: "",
		Origin:     OriginUser,
	}
	raw, _ := json.Marshal(doc)
	src := &stubRecipeSource{recipes: map[string][]byte{userID: raw}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if NewRecipe(userID) != nil {
		t.Errorf("user recipe with empty BaseRecipe should not be registered")
	}
}

func TestUpdateRecipeDefaultsAndInvalidate_HappyPath(t *testing.T) {
	const recipeID = "drum-snare"
	originalDefault := -1.0
	for _, p := range RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			originalDefault = p.Default
		}
	}
	if originalDefault < 0 {
		t.Fatalf("precondition: %s has no decay param", recipeID)
	}
	t.Cleanup(func() {
		UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": originalDefault})
	})

	var invalidations atomic.Int64
	prevInvalidator := voiceCacheInvalidate
	voiceCacheInvalidate = func(instID string) { invalidations.Add(1) }
	t.Cleanup(func() { voiceCacheInvalidate = prevInvalidator })

	changed := UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": 1.7})
	if !changed {
		t.Errorf("changed = false; want true")
	}
	if got := RecipeDefaultParams(recipeID)["decay"]; got != 1.7 {
		t.Errorf("decay default = %v; want 1.7", got)
	}
	if invalidations.Load() < 1 {
		t.Errorf("voiceCacheInvalidate fired %d times; want >= 1", invalidations.Load())
	}
}

func TestUpdateRecipeDefaultsAndInvalidate_NoChangeReturnsFalse(t *testing.T) {
	const recipeID = "drum-snare"
	currentDefault := -1.0
	for _, p := range RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			currentDefault = p.Default
		}
	}
	if currentDefault < 0 {
		t.Fatalf("precondition: %s has no decay param", recipeID)
	}
	var invalidations atomic.Int64
	prevInvalidator := voiceCacheInvalidate
	voiceCacheInvalidate = func(instID string) { invalidations.Add(1) }
	t.Cleanup(func() { voiceCacheInvalidate = prevInvalidator })

	changed := UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": currentDefault})
	if changed {
		t.Errorf("changed = true; want false (override equals current default)")
	}
	if invalidations.Load() != 0 {
		t.Errorf("voiceCacheInvalidate fired %d times; want 0", invalidations.Load())
	}
}

func TestRegisterUserRecipeFromBase_HappyPath(t *testing.T) {
	const newID = "user.test.register-from-base.happy"
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	doc, err := RegisterUserRecipeFromBase(newID, "Happy Test", "drum-snare", map[string]float64{
		"decay": 1.5,
	})
	if err != nil {
		t.Fatalf("RegisterUserRecipeFromBase: %v", err)
	}
	if doc.ID != newID {
		t.Errorf("doc.ID = %q want %q", doc.ID, newID)
	}
	if doc.Origin != OriginUser {
		t.Errorf("doc.Origin = %v want OriginUser", doc.Origin)
	}
	if doc.BaseRecipe != "drum-snare" {
		t.Errorf("doc.BaseRecipe = %q want drum-snare", doc.BaseRecipe)
	}
	if len(doc.ParamDefs) != len(RecipeRegistrations()["drum-snare"].Params) {
		t.Errorf("ParamDefs length = %d want %d", len(doc.ParamDefs), len(RecipeRegistrations()["drum-snare"].Params))
	}
	if doc.ParamSeed["decay"] != 1.5 {
		t.Errorf("ParamSeed[decay] = %v want 1.5", doc.ParamSeed["decay"])
	}
	if r := NewRecipe(newID); r == nil {
		t.Errorf("NewRecipe(%q) = nil after registration", newID)
	}
}

func TestRegisterUserRecipeFromBase_UnknownBase(t *testing.T) {
	_, err := RegisterUserRecipeFromBase("user.test.bad-base", "Bad", "does-not-exist", nil)
	if err == nil {
		t.Fatalf("expected error for unknown base recipe")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q does not mention the unknown base id", err.Error())
	}
}

func TestRegisterUserRecipeFromBase_StripsNonFiniteSeed(t *testing.T) {
	const newID = "user.test.strip-nonfinite"
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	doc, err := RegisterUserRecipeFromBase(newID, "Strip", "drum-snare", map[string]float64{
		"decay": 1.5,
		"tone":  math.NaN(),
		"drive": math.Inf(1),
		"pitch": math.Inf(-1),
	})
	if err != nil {
		t.Fatalf("RegisterUserRecipeFromBase: %v", err)
	}
	if _, ok := doc.ParamSeed["decay"]; !ok {
		t.Errorf("ParamSeed lost finite key 'decay'")
	}
	for _, badKey := range []string{"tone", "drive", "pitch"} {
		if _, ok := doc.ParamSeed[badKey]; ok {
			t.Errorf("ParamSeed retained non-finite key %q", badKey)
		}
	}
}

func TestInstrumentParamsDiffer(t *testing.T) {
	const instID = "test.differ.inst"
	const recipeID = "drum-snare"

	cases := []struct {
		name    string
		setup   func()
		instArg string
		recArg  string
		want    bool
	}{
		{
			name:    "empty instID returns false",
			setup:   func() {},
			instArg: "",
			recArg:  recipeID,
			want:    false,
		},
		{
			name:    "empty recipeID returns false",
			setup:   func() {},
			instArg: instID,
			recArg:  "",
			want:    false,
		},
		{
			name:    "no overlay returns false",
			setup:   func() { ResetInstrumentParams(instID) },
			instArg: instID,
			recArg:  recipeID,
			want:    false,
		},
		{
			name: "overlay with key absent from defaults returns true",
			setup: func() {
				ResetInstrumentParams(instID)
				SetInstrumentParam(instID, "decay", 1.5)
			},
			instArg: instID,
			recArg:  recipeID,
			want:    true,
		},
	}
	t.Cleanup(func() { ResetInstrumentParams(instID) })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup()
			got := InstrumentParamsDiffer(c.instArg, c.recArg)
			if got != c.want {
				t.Errorf("InstrumentParamsDiffer(%q, %q) = %v; want %v", c.instArg, c.recArg, got, c.want)
			}
		})
	}
}
