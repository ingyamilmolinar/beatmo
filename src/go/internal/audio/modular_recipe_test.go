package audio

import (
	"encoding/json"
	"testing"
)

// ParamDef gains an optional Enum field (v2, additive + omitempty) so discrete
// params (oscillator type, filter type, …) carry selector labels while staying
// float-valued. A ParamDef WITHOUT an enum must serialize identically to v1
// (no "enum" key) so existing recipe goldens are unaffected.
func TestParamDefEnumRoundTrip(t *testing.T) {
	withEnum := ParamDef{
		Name: "osc_type", Label: "Oscillator", Min: 0, Max: 4, Default: 0,
		Group: "osc", Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM"},
	}
	raw, err := json.Marshal(withEnum)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ParamDef
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Enum) != 5 || back.Enum[4] != "FM" {
		t.Errorf("enum round-trip lost labels: %v", back.Enum)
	}

	// No enum => key omitted.
	plain := ParamDef{Name: "drive", Min: 0, Max: 1, Default: 0}
	praw, _ := json.Marshal(plain)
	var m map[string]any
	_ = json.Unmarshal(praw, &m)
	if _, ok := m["enum"]; ok {
		t.Errorf("ParamDef without enum must omit the key, got %s", praw)
	}
}

func TestModularSynthParamDefs_ShapeAndDefaults(t *testing.T) {
	defs := ModularSynthParamDefs()
	schema := ModularParamSchema()
	if len(defs) != len(schema) {
		t.Fatalf("ModularSynthParamDefs len = %d, want %d (1:1 with full schema)", len(defs), len(schema))
	}
	ident := ModularParamSchemaIdentity()
	// Phase-8C carve-out: the modulator-stage NUMERICS (pitchenv/lfo/burst
	// knobs) deliberately default to AUDIBLE values (12 st sweep, 8 Hz / 0.5
	// wobble, clap bursts) while their schema identity stays 0. The invariant
	// this test protects — an unedited voice renders identically through the
	// NULL fast path and the recipe path — still holds because each stage's
	// ENABLE defaults 0 (which MUST equal its identity, asserted below) and a
	// disabled stage never reads its numerics (exact bypass; see
	// TestPitchEnvDisabledIsExactBypass et al). The toggles are NOT carved out.
	modulatorNumericCarveOut := map[string]bool{
		"pitchenv_amt": true, "pitchenv_decay": true,
		"lfo_rate": true, "lfo_depth": true,
		"burst_sharp": true,
		"burst1_off":  true, "burst1_amp": true, "burst2_off": true, "burst2_amp": true,
		"burst3_off": true, "burst3_amp": true, "burst4_off": true, "burst4_amp": true,
	}
	seen := map[string]bool{}
	for i, d := range defs {
		if d.Name != schema[i] {
			t.Errorf("def[%d].Name = %q, want %q", i, d.Name, schema[i])
		}
		seen[d.Name] = true
		if d.Default < d.Min || d.Default > d.Max {
			t.Errorf("%s default %v outside [%v,%v]", d.Name, d.Default, d.Min, d.Max)
		}
		if modulatorNumericCarveOut[d.Name] {
			continue
		}
		if d.Default != ident[d.Name] {
			t.Errorf("%s default %v != identity %v (must match C NULL-defaults)", d.Name, d.Default, ident[d.Name])
		}
	}
	for _, n := range modularSchemaWant {
		if !seen[n] {
			t.Errorf("ModularSynthParamDefs missing %q", n)
		}
	}
	// The four discrete params must carry enum labels (osc_type now includes
	// the two noise generators; the five per-stage enable toggles are Off/On).
	enums := map[string]int{
		"osc_type": 7, "fm_algorithm": 4, "amp_curve": 2, "filter_type": 3,
		"osc_enabled": 2, "fm_enabled": 2, "env_enabled": 2, "filter_enabled": 2, "drive_enabled": 2,
		// Phase-8C modulator-stage toggles (Off/On).
		"pitchenv_enabled": 2, "lfo_enabled": 2, "burst_enabled": 2,
	}
	byName := map[string]ParamDef{}
	for _, d := range defs {
		byName[d.Name] = d
	}
	for name, want := range enums {
		if got := len(byName[name].Enum); got != want {
			t.Errorf("%s enum len = %d, want %d", name, got, want)
		}
	}
}

func TestModularRecipeRegistered(t *testing.T) {
	reg, ok := RecipeRegistrations()["synth-modular"]
	if !ok || reg == nil {
		t.Fatalf("synth-modular not registered")
	}
	if reg.Category != "modular" {
		t.Errorf("synth-modular category = %q, want modular", reg.Category)
	}
	if len(reg.Params) != len(ModularParamSchema()) {
		t.Errorf("synth-modular params = %d, want %d", len(reg.Params), len(ModularParamSchema()))
	}
	r := NewRecipe("synth-modular")
	if r == nil {
		t.Fatalf("NewRecipe(synth-modular) = nil")
	}
	if r.ID() != "synth-modular" {
		t.Errorf("recipe ID = %q, want synth-modular", r.ID())
	}
}

func TestModularInstrumentBoundAndListed(t *testing.T) {
	if got := factoryRecipeForInstrument("modular"); got != "synth-modular" {
		t.Errorf("modular instrument bound to %q, want synth-modular", got)
	}
	found := false
	for _, id := range BuiltinInstrumentIDs {
		if id == "modular" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("modular not in BuiltinInstrumentIDs")
	}
}

// TestModularPadPresetRegistered — the second shipped modular preset
// (modular-pad) must be a distinct instrument bound to its own
// synth-modular-pad recipe, whose default params diverge from the base
// synth-modular recipe via the descriptor Seed. Same schema shape (27
// params), different default values — proving preset divergence rides on
// ParamSeed, not a forked schema.
func TestModularPadPresetRegistered(t *testing.T) {
	// Instrument exists and is bound to its own recipe.
	if got := factoryRecipeForInstrument("modular-pad"); got != "synth-modular-pad" {
		t.Errorf("modular-pad instrument bound to %q, want synth-modular-pad", got)
	}
	found := false
	for _, id := range BuiltinInstrumentIDs {
		if id == "modular-pad" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("modular-pad not in BuiltinInstrumentIDs")
	}

	// Recipe registered, same modular category + schema length.
	reg, ok := RecipeRegistrations()["synth-modular-pad"]
	if !ok || reg == nil {
		t.Fatalf("synth-modular-pad not registered")
	}
	if reg.Category != modularRecipeCategory {
		t.Errorf("synth-modular-pad category = %q, want %q", reg.Category, modularRecipeCategory)
	}
	if len(reg.Params) != len(ModularParamSchema()) {
		t.Errorf("synth-modular-pad params = %d, want %d (same schema as base)", len(reg.Params), len(ModularParamSchema()))
	}

	// Defaults must diverge from the base modular recipe.
	base := RecipeDefaultParams("synth-modular")
	pad := RecipeDefaultParams("synth-modular-pad")
	if hashRecipeParams(base) == hashRecipeParams(pad) {
		t.Fatalf("synth-modular-pad defaults are identical to synth-modular; the pad Seed must change the sound")
	}
}
