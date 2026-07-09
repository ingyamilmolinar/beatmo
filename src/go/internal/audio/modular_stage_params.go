package audio

// modular_stage_params.go is the BUILD-TAG-NEUTRAL Phase-8A surface that makes
// every migrated recipe's modular pipeline stages USER-CONTROLLABLE. It owns:
//
//   - modularStageParamDefs: the user-facing stage ParamDefs appended to every
//     migrated recipe's schema (osc/env/filter + the post DRIVE/gain knobs +
//     the per-stage enable toggles), REUSED from ModularSynthParamDefs so bounds
//     / labels / groups / enums never drift,
//   - migratedStageDefaults: the off/identity stage defaults a migrated recipe
//     ships with, so the natural sound = pure family voice + POST, byte-identical
//     to before Phase-8A (osc/env/filter/drive OFF, every numeric at identity),
//   - the stage-name → modular-field mapping shared by the native binding
//     (applyUserStageParamsToModular, !test && !js) and the WASM push
//     (applyUserStageParamsToPush), so both render the SAME effective params,
//   - isModularStageParamName, the predicate the oracle sweep + the
//     every-wired-param-mutates effect test use to recognise a stage param.
//
// Why a curated subset of ModularSynthParamDefs (not the whole thing): the
// migrated recipes already carry the FM voice + POST as family/generic knobs.
// The modular FM stage osc (fm_op*_ratio etc.) COLLIDES by name with the FM
// recipes' family knobs. modularStageParamDefs therefore excludes any stage
// def whose name is already present on the recipe, AND drops a stage's enable
// toggle when its numeric knobs were all excluded (no silent no-op toggle).
// The fm_* group is excluded wholesale on the FM recipes (their voice IS FM).
// The modular DRIVE saturator stage (`drive`/`drive_enabled`) is simply NOT in
// modularStageGroups at all — deliberately unexposed in Phase 8A because its
// knob name is the same token as the generic POST `drive` every migrated
// recipe may wire; grit comes from the generic POST drive. (Deferred, not a
// collision-mechanism outcome.)

// modularStageGroup names one collapsible pipeline stage: the modular field
// names it owns (numeric knobs, NOT the toggle) and the toggle that gates it.
// Toggle "" means the group has no enable toggle (e.g. the standalone post gain
// / post_enabled live in their own pseudo-groups).
type modularStageGroup struct {
	numeric []string
	toggle  string
}

// modularStageGroups is the ordered list of user-facing stages appended to a
// migrated recipe, in UI order (osc → fm → env → filter → drive/post). Names
// are looked up in ModularSynthParamDefs so the appended def reuses the exact
// bounds/labels/groups/enums; only the Default is overridden from stageDefaults.
//
// EXCLUDED vs ModularSynthParamDefs: `pitch` (collides with the legacy generic
// post `pitch` every drum recipe already carries; the modular `pitch` stays
// engine-internal for migrated recipes), `noise_seed` / hidden, and the gen_*
// bank (engine plumbing). `post_enabled` is ADDED here (it is a hidden global
// in ModularSynthParamDefs) with Default 1 so the user can force the legacy
// POST stage off.
var modularStageGroups = []modularStageGroup{
	{numeric: []string{"osc_type", "osc_detune", "osc_octave"}, toggle: "osc_enabled"},
	{numeric: []string{
		"fm_algorithm",
		"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
	}, toggle: "fm_enabled"},
	{numeric: []string{"amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve"}, toggle: "env_enabled"},
	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST — the spec-§1 gap
	// closure). Unlike the pre-existing stages these default DISABLED on every
	// recipe (they are new DSP; off = byte-identity). Their numeric defaults are
	// the canonical audible ParamDef defaults, NOT schema identities — see
	// migratedStageDefaults's modulator carve-out. PITCH ENV is context-dependent
	// (needs the OSC stage on with a pitched osc type) exactly like fm_enabled;
	// LFO/BURST are post-mix and shape every voice incl. the family gen-slots. ──
	{numeric: []string{"pitchenv_amt", "pitchenv_decay"}, toggle: "pitchenv_enabled"},
	{numeric: []string{"lfo_rate", "lfo_depth", "lfo_target", "lfo_delay"}, toggle: "lfo_enabled"},
	{numeric: []string{
		"burst_sharp",
		"burst1_off", "burst1_amp", "burst2_off", "burst2_amp",
		"burst3_off", "burst3_amp", "burst4_off", "burst4_amp",
	}, toggle: "burst_enabled"},
	{numeric: []string{"filter_type", "filter_cutoff", "filter_resonance"}, toggle: "filter_enabled"},
	{numeric: []string{"filtenv_amt", "filtenv_decay", "filtenv_attack"}, toggle: "filtenv_enabled"},
	/* Phase-8E/F unison/ensemble + drift, PLUS the Phase-15 humanization knobs
	   (ens_scatter/vib_rate/vib_depth/humanize — same ENSEMBLE stage as the
	   unison stack, Task 8) AND the Phase-16 ens_jitter cycle-roughness knob.
	   No toggle — voices=1 is the identity off-state. */
	{numeric: []string{
		"unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
		"ens_scatter", "ens_vib_rate", "ens_vib_depth", "ens_humanize", "ens_jitter",
	}, toggle: ""},
	// Phase-15 FORMANT vowel-bank stage (Task 8), PLUS the Phase-16 formant_dry
	// dry-blend scaler. formant_enabled is the pill; the numerics are the
	// vowel/voice-type/mix/shape knobs.
	{numeric: []string{
		"formant_vowel", "formant_voice_type", "formant_mix", "formant_shift",
		"formant_breath", "formant_sing", "formant_morph_rate", "formant_morph_to",
		"formant_dry",
	}, toggle: "formant_enabled"},
	// Phase-15 RESONATOR body bank (Task 8). No toggle — body_model==0 ("Off")
	// is the identity off-state, mirroring the unison/ensemble stage.
	{numeric: []string{"body_model", "body_mix", "bow_dynamics"}, toggle: ""},
	// The modular DRIVE stage (`drive` + drive_enabled) is intentionally NOT
	// exposed on migrated recipes: its `drive` knob name is the SAME token the
	// generic post drive uses, and the two are different DSP nodes — surfacing
	// both would create a confusing duplicate-named knob (and break the
	// "drive == generic post drive" invariant the registry discipline tests
	// enforce). Migrated recipes keep grit via their generic `drive` (POST);
	// the extra modular saturator stays engine-internal for Phase 8A.
	//
	// Standalone post knobs: gain (output trim) + post_enabled (force the legacy
	// POST stage off). No enable toggle of their own.
	{numeric: []string{"gain", "post_enabled"}, toggle: ""},
}

// modularStageParamNames is every name modularStageGroups can emit (numerics +
// toggles), used by isModularStageParamName. Built once at init.
var modularStageParamNames = func() map[string]bool {
	m := map[string]bool{}
	for _, g := range modularStageGroups {
		for _, n := range g.numeric {
			m[n] = true
		}
		if g.toggle != "" {
			m[g.toggle] = true
		}
	}
	return m
}()

// isModularStageParamName reports whether name is one of the Phase-8A stage
// params appended to migrated recipes. The legacy-oracle sweep skips these (the
// legacy renderer had no such capability, so there is no oracle to pin them
// against), and the every-wired-param-mutates effect test skips them too (their
// effect is context-dependent — osc_type does nothing until osc_enabled=1 —
// covered instead by the dedicated context-aware stage_params_test.go).
func isModularStageParamName(name string) bool {
	return modularStageParamNames[name]
}

// isAppendedStageName reports whether name is a Phase-8A stage param that is
// ACTUALLY appended to recipeID's schema — i.e. a stage name whose group did not
// collide with the recipe's family/generic knobs. Used by the bindings/push and
// the discipline tests to tell an appended stage knob from a same-named
// family/generic knob.
func isAppendedStageName(recipeID, name string) bool {
	if !isModularStageParamName(name) {
		return false
	}
	return !recipeExistingStageCollisionSet(recipeID)[name]
}

// IsAppendedStageName is the exported form of isAppendedStageName for the UI:
// reports whether name is a Phase-8B standardized stage param actually appended
// to recipeID's schema (so the Synth tab routes it by stage group + grows the
// stage's enable pill) rather than a same-named family/generic knob.
func IsAppendedStageName(recipeID, name string) bool {
	return isAppendedStageName(recipeID, name)
}

// post_enabled is a hidden global in ModularSynthParamDefs (so it carries no
// UI-facing bounds there). Define its user-facing def once here so
// modularStageParamDefs can emit it with Default 1 (force the legacy POST stage
// off when the user dials it to 0).
var modularPostEnabledDef = ParamDef{
	Name: "post_enabled", Label: "Post", Group: "post", Min: 0, Max: 1, Default: 1,
	Enum: []string{"Off", "On"},
}

// modularStageDefByName returns the user-facing ParamDef for a stage param name,
// reusing the matching ModularSynthParamDefs entry verbatim (bounds / label /
// group / enum). post_enabled is special-cased (hidden in the modular schema).
// Returns ok=false for an unknown name. Built from a cached index so the schema
// is walked once.
func modularStageDefByName(name string) (ParamDef, bool) {
	if name == "post_enabled" {
		return modularPostEnabledDef, true
	}
	d, ok := modularSchemaDefIndex[name]
	return d, ok
}

// modularSchemaDefIndex indexes ModularSynthParamDefs by name (the canonical
// bounds/labels/groups/enums source). Built once at init.
var modularSchemaDefIndex = func() map[string]ParamDef {
	m := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		m[d.Name] = d
	}
	return m
}()

// StageParamDefByName is the exported form of modularStageDefByName for the UI:
// returns the canonical user-facing ParamDef (bounds / label / group / enum)
// for a standardized stage param name, so the section router can read its Group
// without a registry lookup. ok=false for a non-stage name.
func StageParamDefByName(name string) (ParamDef, bool) {
	return modularStageDefByName(name)
}

// modularStageParamDefs returns the user-facing stage ParamDefs to append to a
// migrated recipe whose existing (pre-stage) param names are `existing`, with
// each stage param's Default overridden from stageDefaults (falling back to the
// modular schema identity when stageDefaults omits the key).
//
// Collision handling (the fm-collision decision): a numeric stage def whose name
// already appears in `existing` is EXCLUDED (the recipe's own knob owns that
// name). When EVERY numeric of a stage is excluded, that stage's enable toggle
// is dropped too — a toggle with no editable knob would be a silent no-op. This
// drops the whole fm_* group on the FM recipes (their voice IS FM) and the DRIVE
// stage everywhere (the generic post `drive` already owns the name).
//
// Order mirrors modularStageGroups (osc → fm → env → filter → drive → post), so
// the UI shows family/voice knobs first (appended earlier in recipeExtraParams)
// then the standardized stages.
func modularStageParamDefs(existing map[string]bool, stageDefaults map[string]float64) []ParamDef {
	var defs []ParamDef
	emit := func(name string) {
		base, ok := modularStageDefByName(name)
		if !ok {
			return
		}
		if v, has := stageDefaults[name]; has {
			base.Default = v
		}
		// Phase-8B: the stage defs ship in their CANONICAL UI group
		// (osc/fm/env/filter/post, carried verbatim from ModularSynthParamDefs /
		// modularPostEnabledDef) so the unified Synth tab renders them into the
		// standardized stage sections + per-stage enable pills. The binding /
		// push / persistence / export key off the param NAME, never the group, so
		// un-hiding is invisible to the audio path — at the migrated defaults the
		// render stays byte-identical (every stage ships OFF/identity). In
		// Phase 8A these shipped Group:hidden; the flip to the canonical group is
		// what makes the standardized pipeline user-visible on every migrated
		// instrument. The oracle/effect skips key off isAppendedStageName (a
		// NAME predicate), not the group, so generation is unchanged.
		// Clamp the override into the def's bounds defensively (validateParamDef
		// rejects an out-of-range default at registration).
		if base.Default < base.Min {
			base.Default = base.Min
		} else if base.Default > base.Max {
			base.Default = base.Max
		}
		defs = append(defs, base)
	}
	for _, g := range modularStageGroups {
		if stageGroupCollides(g, existing) {
			// GROUP-LEVEL exclusion: if ANY numeric in this stage already exists on
			// the recipe, drop the ENTIRE group (numerics + toggle). This is the
			// fm-collision decision — an FM recipe whose family knobs own fm_op1/2
			// must NOT get a partial modular FM stage (fm_op3/4 + a confusing
			// fm_enabled toggle that conflicts with the family FM voice). NOTE: the
			// modular DRIVE stage never appears here — it is not in
			// modularStageGroups at all (deliberately unexposed; see the file
			// header), so no collision logic governs it.
			continue
		}
		for _, n := range g.numeric {
			emit(n)
		}
		if g.toggle != "" {
			emit(g.toggle)
		}
	}
	return defs
}

// stageGroupCollides reports whether any numeric in g already appears on the
// recipe (`existing`). A single collision excludes the whole group — see the
// fm-collision decision in modularStageParamDefs.
func stageGroupCollides(g modularStageGroup, existing map[string]bool) bool {
	for _, n := range g.numeric {
		if existing[n] {
			return true
		}
	}
	return false
}

// migratedStageDefaults returns the stage-param defaults a MIGRATED recipe ships
// with: every pipeline stage OFF (osc/env/filter/drive) so the natural sound is
// the pure family gen-slot voice + POST, byte-identical to pre-Phase-8A; every
// numeric at the modular schema identity (no-op); post_enabled ON (the legacy
// POST stage runs per the family's postSkipped logic until the user forces it
// off). Reuses ModularParamSchemaIdentity for the numerics so a future schema
// tweak can't desync the off-state from modularIdentityBase().
func migratedStageDefaults() map[string]float64 {
	ident := ModularParamSchemaIdentity()
	out := map[string]float64{}
	for _, g := range modularStageGroups {
		if modulatorStageToggles[g.toggle] {
			// Phase-8C modulator numerics keep their CANONICAL ParamDef defaults
			// (audible starting points) instead of the schema identities (all 0,
			// which would make the enable pill a silent no-op). Safe because the
			// stage's enable defaults 0 below and a disabled stage never reads
			// its numerics (exact bypass) — byte-identity is the toggle's job.
			continue
		}
		for _, n := range g.numeric {
			if n == "post_enabled" {
				continue
			}
			out[n] = ident[n]
		}
	}
	// All pipeline stages OFF (the gen-slot voice is the whole sound).
	out["osc_enabled"] = 0
	out["fm_enabled"] = ident["fm_enabled"] // unread with osc off; keep engine default
	out["env_enabled"] = 0
	out["filter_enabled"] = 0
	out["drive_enabled"] = 0
	out["post_enabled"] = 1
	// Phase-8C modulator stages OFF (identity 0 — matching the C mp_get
	// fallback; these are NEW stages, so off = byte-identity).
	out["pitchenv_enabled"] = 0
	out["lfo_enabled"] = 0
	out["burst_enabled"] = 0
	return out
}

// modulatorStageToggles names the Phase-8C modulator stages whose NUMERIC
// defaults stay canonical (audible) on migrated recipes — see the carve-out in
// migratedStageDefaults.
var modulatorStageToggles = map[string]bool{
	"pitchenv_enabled": true,
	"lfo_enabled":      true,
	"burst_enabled":    true,
}

// recipeExistingStageCollisionSet returns the set of STAGE param names that are
// NOT appended to recipeID — every stage name in a group that collides with one
// of the recipe's family/generic knobs (GROUP-level exclusion, mirroring
// modularStageParamDefs). The binding/push/tests use it to decide whether a
// stage NAME is a genuine appended stage param on this recipe: a name is appended
// iff it is a stage name AND absent from this set.
//
// Example: fm-bass's family knobs own fm_op1_ratio/fm_op2_ratio, so the whole FM
// stage group is excluded — this set therefore contains fm_op1_ratio AND
// fm_op3_ratio AND fm_enabled (none are appended), even though fm_op3_ratio does
// not individually collide.
func recipeExistingStageCollisionSet(recipeID string) map[string]bool {
	existing := recipePreStageNameSet(recipeID)
	out := map[string]bool{}
	for _, g := range modularStageGroups {
		if !stageGroupCollides(g, existing) {
			continue
		}
		for _, n := range g.numeric {
			out[n] = true
		}
		if g.toggle != "" {
			out[g.toggle] = true
		}
	}
	return out
}

// recipePreStageNameSet returns the recipe's PRE-stage param names (the generic
// wired subset + family extras) restricted to names that are ALSO stage names —
// the collision candidates. Built without re-entering WiredParamsForRecipe
// (which now appends the stage defs themselves).
func recipePreStageNameSet(recipeID string) map[string]bool {
	out := map[string]bool{}
	for _, name := range recipeWiredParams[recipeID] {
		if isModularStageParamName(name) {
			out[name] = true
		}
	}
	for _, d := range recipeExtraParams[recipeID] {
		if isModularStageParamName(d.Name) {
			out[d.Name] = true
		}
	}
	return out
}

// applyUserStageParamsToPush overwrites the push map's stage fields from the
// MERGED params, exactly as applyUserStageParamsToModular overwrites the native
// ModularParams struct. Both consume the same merged stage values, so an
// untouched (default) stage reproduces the off/identity state ModularPushParams
// already spells out, and a user-toggled stage flows identically to the browser.
//
// post_enabled is handled by the POST-stage wiring (applyPushPostStage /
// applyFamilyPostStage), NOT here — those own the postSkipped byte-identity
// quirk. This helper writes only osc/fm/env/filter/drive/gain numerics + toggles.
func applyUserStageParamsToPush(out RecipeParams, recipeID string, merged RecipeParams) {
	existing := recipeExistingStageCollisionSet(recipeID)
	for name := range modularStageParamNames {
		if name == "post_enabled" {
			continue // POST-stage wiring owns this (see applyPushPostStage).
		}
		if existing[name] {
			continue // collision: not a stage param on this recipe.
		}
		if v, ok := merged[name]; ok {
			out[name] = v
		}
	}
}
