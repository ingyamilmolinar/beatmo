package audio

// family_modular_push.go is the BUILD-TAG-NEUTRAL, FAMILY-AGNOSTIC migration
// registry for the Phase-2 legacy-family→modular rebinds (bass first; the kick
// family and 4 others replicate its shape next). It owns:
//
//   - the migration registry (modularMigrations) keyed by recipe id,
//   - the generic legacy→modular push translation (ModularPushParams), and
//   - the reusable POST-stage / elision / fundamental scaffolding shared by the
//     native binding (//go:build !test && !js) and the WASM push (//go:build js).
//
// Why tag-neutral: the WASM push runs in Go-WASM where neither ModularParams nor
// the native binding exist, yet it must emit the SAME effective param block the
// native binding feeds the C engine. Keeping the scaffolding here (no
// ModularParams dependency) lets both platforms share one source of truth, and
// lets the registry registrations land in a tag-neutral file that BOTH native
// and js builds compile.
//
// The three-way literal agreement this guards: at a param default, the C
// kp_get(field, literal) fallback, the native binding's NaN sentinel (C resolves
// it), and the push's spelled-out literal must all be the SAME double. The push
// side spells the literal out because NaN can't survive JSON / the JS block-fill
// (NaN → JS PARAM_IDENTITY, which for gen-slot fields is 0, NOT the kp_get
// literal). The float32-quantized literals differ from the native exact doubles
// by <1 ULP, within the xplat correlation gate.

// modularFamilyMigration captures everything the generic push needs to translate
// one migrated recipe's MERGED legacy-named params into the fully-resolved
// modular-named block.
type modularFamilyMigration struct {
	// voiceParams emits the gen-slot voice fields, the voice_freq_hz override,
	// and the global stage toggles (osc/env/filter/drive OFF) for this family —
	// every kp_get fallback literal spelled out. The input is the MERGED
	// legacy-named params; the implementation elides defaults internally as
	// needed (mirroring the native binding's elideRecipeDefaults call).
	voiceParams func(merged RecipeParams) RecipeParams
	// post is the shared POST-stage wiring (decay rate + which of the six post
	// sub-effects the legacy family's apply_post_params enabled).
	post familyPostConfig
	// postSkipped mirrors the legacy NULL-base rule: the family's
	// render_<family>_p passed a NULL base (→ apply_post_params no-ops) exactly
	// when the elided generic base was SynthParams-default AND every family field
	// was unset. Returns true → POST stage skipped entirely. Operates on the
	// ELIDED params.
	postSkipped func(elided RecipeParams) bool
}

// modularMigrations is the registry of migrated recipe ids → their family
// migration. Populated by registerModularMigration in each family's tag-neutral
// init (bass_modular_push.go for bass). Tag-neutral so WASM registers them too.
var modularMigrations = map[string]modularFamilyMigration{}

// registerModularMigration records a family migration for recipeID. Called from
// the owning family's tag-neutral init so both native and js builds see it.
func registerModularMigration(recipeID string, m modularFamilyMigration) {
	modularMigrations[recipeID] = m
}

// IsModularMigratedRecipe reports whether a recipe id renders through the
// modular engine via a Phase-2 family migration (so cmd/export_audio + the xplat
// test route it through render_modular_p with the modular block, and the
// platform-push seam translates its legacy-named params to modular names).
func IsModularMigratedRecipe(id string) bool {
	_, ok := modularMigrations[id]
	return ok
}

// ModularPushParams translates a migrated recipe's MERGED legacy-named
// RecipeParams into the FULLY-RESOLVED modular-named block pushed to the
// platform layer. Returns (nil, false) for non-migrated recipe ids.
//
// The output map is keyed by modular schema names and contains: the global stage
// toggles (off), the gen-slot voice fields with their kp_get fallback literals
// spelled out, and the shared POST stage. Only the keys the binding actually
// drives are emitted; the JS block-fill substitutes the modular schema identity
// for every absent key, which matches modularIdentityBase by construction.
func ModularPushParams(recipeID string, merged RecipeParams) (map[string]float64, bool) {
	m, ok := modularMigrations[recipeID]
	if !ok {
		return nil, false
	}
	elided := elideRecipeDefaults(recipeID, merged)
	postSkipped := m.postSkipped(elided)

	out := RecipeParams{}

	// modularIdentityBase parity: the native binding turns the legacy global
	// pipeline stages OFF (osc/env/filter/drive) — the gen-slot voice is the
	// whole sound. The modular SCHEMA identity defaults these toggles to 1 (ON),
	// so the JS block-fill would leave them enabled and stack an unwanted
	// osc/env/filter on top of the gen slot. Spell the OFF state out explicitly
	// so the browser reproduces modularIdentityBase. (fm_enabled stays at
	// identity — unread with osc off; gain/osc/amp/filter numeric fields stay at
	// identity, also unread.)
	out["osc_enabled"] = 0
	out["env_enabled"] = 0
	out["filter_enabled"] = 0
	out["drive_enabled"] = 0

	// Family-specific gen-slot voice + voice_freq_hz, all kp_get literals spelled
	// out.
	for k, v := range m.voiceParams(merged) {
		out[k] = v
	}

	// Phase-8A: overlay the USER's stage params (osc/env/filter/drive/gain +
	// toggles) from the merged set, the SAME write the native binding's
	// applyUserStageParamsToModular performs. At the migrated defaults every
	// stage is OFF / identity, so these overwrite the spelled-out OFF state with
	// the identical value (byte-neutral); a user-toggled stage flows to the
	// browser exactly as it does to C. post_enabled is handled by the POST stage
	// below.
	applyUserStageParamsToPush(out, recipeID, merged)

	applyPushPostStage(out, elided, postSkipped, m.post)
	return map[string]float64(out), true
}

// modularPushParamsForInstrument is the platform-push seam entry point: given an
// instrument id and its LEGACY-named overlay snapshot (as held by the params
// manager), it returns the FULLY-RESOLVED modular-named block to push to the
// browser, or (nil, false) when the instrument is not a migrated family (the
// caller then pushes the untranslated overlay as before).
//
// The overlay is merged over the recipe defaults first, because the binding
// reads effective values (the resolved fundamental, the elide-vs-default
// decision). This is the single seam where legacy param names become modular
// names — prefs / export / UI keep the legacy names everywhere else.
func modularPushParamsForInstrument(instID string, overlay RecipeParams) (RecipeParams, bool) {
	recipeID := RecipeForInstrument(instID)
	if !IsModularMigratedRecipe(recipeID) {
		return nil, false
	}
	merged := MergeRecipeDefaults(recipeID, overlay)
	out, ok := ModularPushParams(recipeID, merged)
	if !ok {
		return nil, false
	}
	return RecipeParams(out), true
}

// familyPostConfig captures the per-family wiring of the shared modular POST
// stage: the decay-rate constant and which of the six post sub-effects the
// legacy family's apply_post_params enabled. Tag-neutral.
type familyPostConfig struct {
	DecayRate  float64
	Pitch      bool
	Decay      bool
	Body       bool
	Brightness bool
	Tone       bool
	Drive      bool
	// PostOrder selects the POST-stage op ORDER (modular_params.post_order):
	// 0 = the shared apply_post_params order (pitch→decay→body→brightness→tone→
	// drive); 1 = the legacy base-kick order (pitch→decay→drive→body). Only the
	// base kick needs 1; every other migrated family's legacy wrapper used
	// apply_post_params, so its order is 0 (the zero value).
	PostOrder float64
}

func boolToParam(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// fundamentalResolve mirrors the deleted drums.c:bass_fund clamp ([25,250]) and
// the "<=0 → default" rule. The legacy path read base.fundamental (0 at the knob
// default), so absent/0 maps to def. Tag-neutral (used by both the native
// binding and the push). Promoted from the bass-specific bassFundResolve so the
// kick family (and the rest) can reuse the same fundamental resolution.
func fundamentalResolve(merged RecipeParams, def float64) float64 {
	f := merged["fundamental"]
	if f <= 0 {
		return def
	}
	if f < 25.0 {
		f = 25.0
	}
	if f > 250.0 {
		f = 250.0
	}
	return f
}

// elidedVal returns the elided value for name if present, else def. Feeds the
// shared POST stage from the elided map, falling back to the generic synth
// identity (0 for pitch/drive/body, 1 for decay) so an untouched knob is a POST
// no-op — exactly like recipeParamsToSynth feeds the legacy base synth_params.
// Tag-neutral (shared by the native binding's applyFamilyPostStage and the WASM
// push's applyPushPostStage).
func elidedVal(elided RecipeParams, name string, def float64) float64 {
	if v, ok := elided[name]; ok {
		return v
	}
	return def
}

// elidedValOr returns the elided value for name if present, else the supplied
// literal (the C kp_get fallback). The push side spells out the literal — unlike
// elidedValOrNaN (native binding), which passes NaN so C recovers the double.
func elidedValOr(elided RecipeParams, name string, literal float64) float64 {
	if v, ok := elided[name]; ok {
		return v
	}
	return literal
}

// applyPushPostStage writes the modular post_* fields into the push map exactly
// as applyFamilyPostStage writes the ModularParams struct (same gates, same
// elided base reads). Kept separate so it stays build-tag-neutral (no
// ModularParams dependency).
func applyPushPostStage(out RecipeParams, elided RecipeParams, postSkipped bool, cfg familyPostConfig) {
	// Phase-8A user override: post_enabled default 1 is elided away (untouched ⇒
	// today's postSkipped logic). A user-set 0 survives elision and FORCES the
	// legacy POST stage off. Default-state behavior is unchanged ⇒ fixtures green.
	if pe, ok := elided["post_enabled"]; ok && pe < 0.5 {
		out["post_enabled"] = 0
		return
	}
	if postSkipped {
		out["post_enabled"] = 0
		return
	}
	out["post_enabled"] = 1
	out["post_decay_rate"] = cfg.DecayRate
	out["post_pitch_on"] = boolToParam(cfg.Pitch)
	out["post_decay_on"] = boolToParam(cfg.Decay)
	out["post_body_on"] = boolToParam(cfg.Body)
	out["post_brightness_on"] = boolToParam(cfg.Brightness)
	out["post_tone_on"] = boolToParam(cfg.Tone)
	out["post_drive_on"] = boolToParam(cfg.Drive)
	out["post_pitch"] = elidedVal(elided, "pitch", 0)
	out["post_decay"] = elidedVal(elided, "decay", 1)
	out["post_drive"] = elidedVal(elided, "drive", 0)
	out["post_body"] = elidedVal(elided, "body", 0)
	out["post_tone"] = elidedVal(elided, "tone", 0)
	out["post_brightness"] = elidedVal(elided, "brightness", 0)
	out["post_order"] = cfg.PostOrder
}
