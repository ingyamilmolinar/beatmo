//go:build !test && !js

package audio

import "math"

// family_modular_binding.go holds the FAMILY-GENERIC scaffolding shared by every
// Phase-2 legacy-family→modular rebind (bass, and the 5 families that copy its
// shape next). Each family's binding (e.g. bassRecipeToModular) wires its own
// gen-slot voice + POST config; the reusable identity-seed, elided-param
// readers, and POST-stage application live here so a new family migration is a
// thin wrapper, not a copy.

// modularIdentityBase returns a ModularParams seeded so every GLOBAL pipeline
// stage is a no-op AND the legacy-osc/FM/env/filter/drive stages are OFF — the
// canonical starting point for a family that is "the gen-slot voice is the whole
// sound". It matches the inline seed bassRecipeToModular previously carried:
// schema-identity values for the legacy osc/amp/filter fields (all UNREAD once
// the stages are gated off, but seeded for clarity), and the pipeline toggles
// explicitly cleared. FMEnabled is kept at the engine default — see below.
func modularIdentityBase() ModularParams {
	ident := ModularParamSchemaIdentity()
	mp := ModularParams{}
	// Start from the modular schema identities so every untouched stage is a
	// no-op (osc off, env off, filter off, drive off, gen slots off).
	mp.OscType = ident["osc_type"]
	mp.OscDetune = ident["osc_detune"]
	mp.OscOctave = ident["osc_octave"]
	mp.AmpAttack = ident["amp_attack"]
	mp.AmpDecay = ident["amp_decay"]
	mp.AmpSustain = ident["amp_sustain"]
	mp.AmpRelease = ident["amp_release"]
	mp.AmpCurve = ident["amp_curve"]
	mp.FilterType = ident["filter_type"]
	mp.FilterCutoff = ident["filter_cutoff"]
	mp.FilterResonance = ident["filter_resonance"]
	mp.Gain = ident["gain"]
	mp.NoiseDraws = ident["noise_draws"]

	// All global pipeline stages OFF — a migrated family's source==4 (or other)
	// gen-slot voice is the whole sound.
	mp.OscEnabled = 0
	mp.FMEnabled = ident["fm_enabled"] // unread (osc off); keep the engine default
	mp.EnvEnabled = 0
	mp.FilterEnabled = 0
	mp.DriveEnabled = 0
	return mp
}

// modularStageBase returns modularIdentityBase() with the USER's stage params
// applied on top — the single native injection point for the Phase-8A
// stage-controllability work. Every family binding starts from this instead of
// modularIdentityBase() so a user-toggled stage (osc_enabled=1, filter_cutoff=…)
// flows uniformly to the engine for EVERY migrated family, implemented once.
//
// At the migrated defaults (every stage OFF, every numeric at identity), the
// applied values exactly equal modularIdentityBase()'s hardcoded seed, so the
// default render is byte-identical to the pre-Phase-8A binding. recipeID is only
// used to recover the recipe's existing-name set, so a stage name that collides
// with a family knob (the post `drive`, the FM voice's fm_op*) is NOT mistaken
// for a stage param. post_enabled is owned by applyFamilyPostStage (the
// postSkipped quirk), not applied here.
func modularStageBase(recipeID string, merged RecipeParams) ModularParams {
	mp := modularIdentityBase()
	applyUserStageParamsToModular(&mp, recipeID, merged)
	return mp
}

// applyUserStageParamsToModular overwrites mp's osc/env/filter/drive/gain stage
// fields from the MERGED params, honoring the recipe's collision set (a stage
// name already owned by a family knob is skipped). Mirrors
// applyUserStageParamsToPush (build-tag-neutral) field-for-field so the native
// render and the browser push agree. post_enabled is excluded (handled by the
// POST-stage wiring).
func applyUserStageParamsToModular(mp *ModularParams, recipeID string, merged RecipeParams) {
	existing := recipeExistingStageCollisionSet(recipeID)
	get := func(name string) (float64, bool) {
		if existing[name] {
			return 0, false
		}
		v, ok := merged[name]
		return v, ok
	}
	if v, ok := get("osc_type"); ok {
		mp.OscType = v
	}
	if v, ok := get("osc_detune"); ok {
		mp.OscDetune = v
	}
	if v, ok := get("osc_octave"); ok {
		mp.OscOctave = v
	}
	if v, ok := get("osc_enabled"); ok {
		mp.OscEnabled = v
	}
	if v, ok := get("fm_algorithm"); ok {
		mp.FMAlgorithm = v
	}
	if v, ok := get("fm_op1_ratio"); ok {
		mp.FMOp1Ratio = v
	}
	if v, ok := get("fm_op2_ratio"); ok {
		mp.FMOp2Ratio = v
	}
	if v, ok := get("fm_op3_ratio"); ok {
		mp.FMOp3Ratio = v
	}
	if v, ok := get("fm_op4_ratio"); ok {
		mp.FMOp4Ratio = v
	}
	if v, ok := get("fm_op1_depth"); ok {
		mp.FMOp1Depth = v
	}
	if v, ok := get("fm_op2_depth"); ok {
		mp.FMOp2Depth = v
	}
	if v, ok := get("fm_op3_depth"); ok {
		mp.FMOp3Depth = v
	}
	if v, ok := get("fm_op4_depth"); ok {
		mp.FMOp4Depth = v
	}
	if v, ok := get("fm_op1_level"); ok {
		mp.FMOp1Level = v
	}
	if v, ok := get("fm_op2_level"); ok {
		mp.FMOp2Level = v
	}
	if v, ok := get("fm_op3_level"); ok {
		mp.FMOp3Level = v
	}
	if v, ok := get("fm_op4_level"); ok {
		mp.FMOp4Level = v
	}
	if v, ok := get("fm_enabled"); ok {
		mp.FMEnabled = v
	}
	if v, ok := get("amp_attack"); ok {
		mp.AmpAttack = v
	}
	if v, ok := get("amp_decay"); ok {
		mp.AmpDecay = v
	}
	if v, ok := get("amp_sustain"); ok {
		mp.AmpSustain = v
	}
	if v, ok := get("amp_release"); ok {
		mp.AmpRelease = v
	}
	if v, ok := get("amp_curve"); ok {
		mp.AmpCurve = v
	}
	if v, ok := get("env_enabled"); ok {
		mp.EnvEnabled = v
	}
	if v, ok := get("filter_type"); ok {
		mp.FilterType = v
	}
	if v, ok := get("filter_cutoff"); ok {
		mp.FilterCutoff = v
	}
	if v, ok := get("filter_resonance"); ok {
		mp.FilterResonance = v
	}
	if v, ok := get("filter_enabled"); ok {
		mp.FilterEnabled = v
	}
	if v, ok := get("drive"); ok {
		mp.Drive = v
	}
	if v, ok := get("drive_enabled"); ok {
		mp.DriveEnabled = v
	}
	if v, ok := get("gain"); ok {
		mp.Gain = v
	}
	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST). Same merged-value
	// flow as the stages above; the toggles default 0 (stage off ⇒ the engine
	// never reads the numerics ⇒ exact bypass at the migrated defaults). ──
	if v, ok := get("pitchenv_enabled"); ok {
		mp.PitchEnvEnabled = v
	}
	if v, ok := get("pitchenv_amt"); ok {
		mp.PitchEnvAmt = v
	}
	if v, ok := get("pitchenv_decay"); ok {
		mp.PitchEnvDecay = v
	}
	if v, ok := get("lfo_enabled"); ok {
		mp.LfoEnabled = v
	}
	if v, ok := get("lfo_rate"); ok {
		mp.LfoRate = v
	}
	if v, ok := get("lfo_depth"); ok {
		mp.LfoDepth = v
	}
	if v, ok := get("burst_enabled"); ok {
		mp.BurstEnabled = v
	}
	if v, ok := get("burst_sharp"); ok {
		mp.BurstSharp = v
	}
	if v, ok := get("burst1_off"); ok {
		mp.Burst1Off = v
	}
	if v, ok := get("burst1_amp"); ok {
		mp.Burst1Amp = v
	}
	if v, ok := get("burst2_off"); ok {
		mp.Burst2Off = v
	}
	if v, ok := get("burst2_amp"); ok {
		mp.Burst2Amp = v
	}
	if v, ok := get("burst3_off"); ok {
		mp.Burst3Off = v
	}
	if v, ok := get("burst3_amp"); ok {
		mp.Burst3Amp = v
	}
	if v, ok := get("burst4_off"); ok {
		mp.Burst4Off = v
	}
	if v, ok := get("burst4_amp"); ok {
		mp.Burst4Amp = v
	}
}

// elidedValOrNaN returns the elided value for name if present (i.e. non-default,
// so it was retained by elideRecipeDefaults), else NaN. Passing NaN into the
// modular ABI lets the C analytic voice's kp_get(field, legacy_literal) fall
// back to the EXACT legacy double literal at default, while a non-default value
// arrives float32-quantized — reproducing the legacy NaN-elision byte-for-byte.
func elidedValOrNaN(elided RecipeParams, name string) float64 {
	if v, ok := elided[name]; ok {
		return v
	}
	return math.NaN()
}

// elidedVal, familyPostConfig, boolToParam live in family_modular_push.go
// (build-tag-neutral) so the WASM push seam (//go:build js) shares the exact
// same POST-stage wiring as this native binding.

// applyFamilyPostStage mirrors the legacy POST byte-identity quirk and sets the
// modular post_* fields on mp.
//
// CRITICAL byte-identity quirk: the legacy family renderer passes a NULL base to
// its render_<family>_p (→ apply_post_params no-ops) exactly when the family's
// recipeParamsTo<Family>(elided).toC() == nil, i.e. when the generic base is
// "default" by SynthParams.IsDefault() (which treats Decay==0 as default, NOT
// Decay==1) AND every family field is unset. A decay=min sweep dials decay to 0,
// which trips IsDefault → the legacy POST is SKIPPED ENTIRELY. The caller passes
// that condition in via postSkipped (it's family-struct-specific — the bass
// caller computes it from recipeParamsToBass), and we mirror it here: POST runs
// only when the legacy base would have been non-NULL.
func applyFamilyPostStage(mp *ModularParams, elided RecipeParams, postSkipped bool, cfg familyPostConfig) {
	// Phase-8A user override: post_enabled default 1 is elided away (untouched ⇒
	// today's postSkipped logic). A user-set 0 survives elision and FORCES the
	// legacy POST stage off. Default-state behavior is unchanged ⇒ fixtures green.
	if pe, ok := elided["post_enabled"]; ok && pe < 0.5 {
		mp.PostEnabled = 0
		return
	}
	if postSkipped {
		mp.PostEnabled = 0
		return
	}
	mp.PostEnabled = 1
	mp.PostDecayRate = cfg.DecayRate
	mp.PostPitchOn = boolToParam(cfg.Pitch)
	mp.PostDecayOn = boolToParam(cfg.Decay)
	mp.PostBodyOn = boolToParam(cfg.Body)
	mp.PostBrightnessOn = boolToParam(cfg.Brightness)
	mp.PostToneOn = boolToParam(cfg.Tone)
	mp.PostDriveOn = boolToParam(cfg.Drive)
	mp.PostPitch = elidedVal(elided, "pitch", 0)
	mp.PostDecay = elidedVal(elided, "decay", 1)
	mp.PostDrive = elidedVal(elided, "drive", 0)
	mp.PostBody = elidedVal(elided, "body", 0)
	mp.PostTone = elidedVal(elided, "tone", 0)
	mp.PostBrightness = elidedVal(elided, "brightness", 0)
	mp.PostOrder = cfg.PostOrder
}
