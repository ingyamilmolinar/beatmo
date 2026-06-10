package audio

// MigrateGenType converts gen_type-era saved data to the post-deprecation
// model. The "Generator" selector (gen_type) let a bespoke drum/FM recipe
// re-voice through the generic modular pipeline; the native-deprecation
// migration removed it — an instrument's own engine is fully parameterized
// now, and generic re-voicing is the Modular recipe's job.
//
//   - gen_type >= 1 (re-voiced): rebind to synth-modular with
//     osc_type = gen_type-1. Generic knobs that exist on both schemas
//     (pitch, drive, …) carry over untouched; bespoke-only keys are kept
//     too — the modular schema simply ignores them.
//   - gen_type 0 / absent (Native): recipe unchanged; the key is dropped.
//
// Hooked at the project-import seam (the only place re-voicing was
// persisted as structural state) and defensively at userprefs
// recipe-override load. Returns the params map unchanged (same reference)
// when there is nothing to migrate.
func MigrateGenType(recipeID string, params RecipeParams) (string, RecipeParams) {
	gt, ok := params["gen_type"]
	if !ok {
		return recipeID, params
	}
	out := make(RecipeParams, len(params))
	for k, v := range params {
		if k == "gen_type" {
			continue
		}
		out[k] = v
	}
	if gt >= 0.5 {
		// Same mapping the old dispatch used: osc_type = gen_type - 1
		// (1=Sine … 7=Noise Pink → modular osc_type 0..6).
		out["osc_type"] = gt - 1
		return "synth-modular", out
	}
	return recipeID, out
}

// StripGenTypeFromParams removes a stray gen_type key from a knob-override
// map without rebinding (userprefs recipe overrides are knob tweaks, not
// structural state — re-voicing intent lives in the project file and is
// handled by MigrateGenType at import).
func StripGenTypeFromParams(params map[string]float64) map[string]float64 {
	if _, ok := params["gen_type"]; !ok {
		return params
	}
	out := make(map[string]float64, len(params))
	for k, v := range params {
		if k == "gen_type" {
			continue
		}
		out[k] = v
	}
	return out
}
