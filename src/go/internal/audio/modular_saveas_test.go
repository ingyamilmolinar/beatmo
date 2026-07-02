package audio

import (
	"encoding/json"
	"testing"
)

// A user Save-As of the modular voice must produce a RecipeDoc that round-trips
// through JSON with its enum-bearing ParamDefs and seeded generator choice
// intact — this is the "save your own generator preset" capability.
func TestModularSaveAs_RoundTripsWithEnumAndSeed(t *testing.T) {
	const newID = "user.synth-modular.testpad"
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	seed := map[string]float64{"osc_type": 1, "filter_cutoff": 300, "amp_attack": 0.4}
	doc, err := RegisterUserRecipeFromBase(newID, "My Pad", "synth-modular", seed)
	if err != nil {
		t.Fatalf("RegisterUserRecipeFromBase: %v", err)
	}
	if doc.BaseRecipe != "synth-modular" {
		t.Errorf("BaseRecipe = %q, want synth-modular", doc.BaseRecipe)
	}

	// The generator choice is carried in ParamSeed; the base ParamDefs keep
	// their enum labels. (ToRegistration overlays the seed onto Defaults at
	// registration time — see the registered-recipe assertion below.)
	if doc.ParamSeed["osc_type"] != 1 {
		t.Errorf("ParamSeed osc_type = %v, want 1 (Saw)", doc.ParamSeed["osc_type"])
	}
	var oscDef *ParamDef
	for i := range doc.ParamDefs {
		if doc.ParamDefs[i].Name == "osc_type" {
			oscDef = &doc.ParamDefs[i]
		}
	}
	if oscDef == nil {
		t.Fatalf("saved doc missing osc_type ParamDef")
	}
	if len(oscDef.Enum) != 12 || oscDef.Enum[1] != "Saw" {
		t.Errorf("osc_type enum labels lost in Save-As: %v", oscDef.Enum)
	}

	// The REGISTERED recipe has the seed overlaid onto its osc_type default.
	reg := RecipeRegistrations()[newID]
	if reg == nil {
		t.Fatalf("Save-As recipe %q not registered", newID)
	}
	for _, d := range reg.Params {
		if d.Name == "osc_type" && d.Default != 1 {
			t.Errorf("registered osc_type default = %v, want 1 (seed applied)", d.Default)
		}
	}

	// Full JSON round-trip (the persistence path stores opaque bytes): the
	// enum labels and the seeded generator choice must both survive.
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	var back RecipeDoc
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if back.ParamSeed["osc_type"] != 1 {
		t.Errorf("ParamSeed osc_type lost through JSON: %v", back.ParamSeed["osc_type"])
	}
	if len(back.ParamDefs) != len(doc.ParamDefs) {
		t.Fatalf("round-trip param count %d != %d", len(back.ParamDefs), len(doc.ParamDefs))
	}
	for _, d := range back.ParamDefs {
		if d.Name == "osc_type" && len(d.Enum) != 12 {
			t.Errorf("osc_type enum lost through JSON: %v", d.Enum)
		}
	}

	if r := NewRecipe(newID); r == nil {
		t.Errorf("NewRecipe(%q) = nil after Save-As", newID)
	}
}
