package audio

import "math"

// recipeWiredParams is the per-recipe whitelist of generic params that the
// underlying C renderer actually reads. The plan-document table (mirrored
// here verbatim) is verified against drums.c + fmsynth.c by the discipline
// test in synth_recipe_wired_test.go. The set is closed: every recipe in
// builtinRecipeDescriptors must have an entry; missing IDs are caught by
// the discipline test, not silently fall through to the 8-knob generic set.
//
// Why this exists: GenericSynthParamDefs advertises 8 knobs to every recipe,
// but the wired subset is recipe-specific (cowbell ignores drive, hihat
// ignores pitch, every recipe ignores `attack` and `color`). Rendering a
// slider that has no audible effect is the most-cited UX bug on the synth
// tab — see [[project_synth_pipeline_architecture]]. The Synth-tab redesign
// (plan file: hey-please-review-the-vectorized-rocket.md) uses this table
// to render only the wired sliders per recipe.
var recipeWiredParams = map[string][]string{
	// Phase 1 — hand-rolled _p() variants in drums.c (each does its own
	// sp_*(params) reads in the body; the discipline test parses the
	// reader names back from the C source).
	"drum-snare":   {"pitch", "decay", "tone", "drive"},
	"drum-kick":    {"pitch", "decay", "drive", "body"},
	"drum-hihat":   {"decay", "drive", "brightness"},
	"drum-clap":    {"decay", "drive"},
	"drum-tom":     {"pitch", "decay", "drive"},
	"drum-cowbell": {"pitch", "decay"},

	// Phase 2 drums — apply_post_params with a post_config in drums.c.
	"drum-open-hihat":      {"decay", "drive", "brightness"},
	"drum-tom-high":        {"pitch", "decay", "drive"},
	"drum-tom-low":         {"pitch", "decay", "drive"},
	"drum-sub-bass":        {"pitch", "decay", "drive", "body"},
	"drum-snare-rimshot":   {"pitch", "decay", "tone", "drive"},
	"drum-snare-sidestick": {"pitch", "decay", "tone", "drive"},
	"drum-kick-deep":       {"pitch", "decay", "drive", "body"},
	"drum-kick-punchy":     {"pitch", "decay", "drive", "body"},
	"drum-kick-lofi":       {"pitch", "decay", "drive", "body"},
	"drum-kick-tight":      {"pitch", "decay", "drive", "body"},
	"drum-shaker":          {"decay", "drive", "brightness"},
	"drum-ride":            {"decay", "drive", "brightness"},
	"drum-crash":           {"decay", "drive", "brightness"},

	// Phase 2 FM — apply_post_params with a post_config in fmsynth.c.
	"fm-bass":   {"pitch", "decay", "tone", "drive", "body"},
	"fm-bell":   {"pitch", "decay", "drive", "brightness"},
	"fm-lead":   {"pitch", "decay", "tone", "drive", "brightness"},
	"fm-epiano": {"pitch", "decay", "tone", "drive", "brightness"},
	"fm-pluck":  {"pitch", "decay", "tone", "drive"},
}

// recipeExtraParams declares per-recipe ParamDefs that are NOT in the
// shared GenericSynthParamDefs set — knobs that touch the core synthesis
// (oscillator fundamental, FM operator ratios, noise color, etc.) instead
// of the global post-process tail. Phase 3 of the live-instrument
// synthesis remediation plan introduced this slot so recipes can declare
// their own sound-shaping knobs without inflating the generic 6-knob
// surface or polluting unrelated recipes.
//
// Adding an extra requires:
//   - the corresponding sp_*() accessor in src/c/synth_params.h
//   - a synth_params struct field on both C and Go sides
//   - the renderer reading it (otherwise recipe_param_effect_test.go's
//     "every wired param mutates output" assertion fails)
//   - schema entry in src/go/internal/audio/synth_param_schema.go
//   - regenerate the JS ABI via cmd/gen-synth-abi
var recipeExtraParams = map[string][]ParamDef{
	// Kick family (native-deprecation migration): per-variant curated knobs.
	// Defaults MUST equal the per-variant literals in src/c/drums.c
	// (render_kick_internal and the deep/punchy/lofi/tight internals) — the
	// at-default byte-parity contract in native_knob_effect_test.go locks
	// every value. NaN in a spec field = that variant doesn't have the stage
	// (punchy has no noise thud; lofi has no beater click).
	"drum-kick": kickFamilyParamDefs(kickFamilySpec{
		fundamental: 55, h2: 0.40, h3: 0.20, h4: 0.12,
		env0: 5.5, env1: 9.0, peAmt: 0.10, peRate: 30,
		click: 0.35, noise: 0.18,
	}),
	"drum-kick-deep": kickFamilyParamDefs(kickFamilySpec{
		fundamental: 42, h2: 0.15, h3: 0.10, h4: nan,
		env0: 3.5, env1: 7.0, peAmt: 0.15, peRate: 15,
		click: 0.20, noise: 0.10,
	}),
	"drum-kick-punchy": kickFamilyParamDefs(kickFamilySpec{
		fundamental: 62, h2: 0.40, h3: nan, h4: nan,
		env0: 8.0, env1: 14.0, peAmt: 0.25, peRate: 55,
		click: 0.45, noise: nan,
	}),
	"drum-kick-lofi": kickFamilyParamDefs(kickFamilySpec{
		fundamental: 50, h2: 0.40, h3: 0.20, h4: nan,
		env0: 4.5, env1: 7.0, peAmt: 0.08, peRate: 20,
		click: nan, noise: 0.25,
	}),
	"drum-kick-tight": kickFamilyParamDefs(kickFamilySpec{
		fundamental: 58, h2: 0.35, h3: 0.15, h4: nan,
		env0: 7.5, env1: 12.0, peAmt: 0.05, peRate: 65,
		click: 0.40, noise: 0.15,
	}),
	// Bass family.
	"drum-sub-bass": []ParamDef{
		waveParamDef("bass_wave", 0),
		{Name: "fundamental", Label: "Fundamental", Group: "core", Min: 25, Max: 250, Default: 45, Unit: "Hz"},
		{Name: "bass_harmonic", Label: "Overtone", Group: "core", Min: 0, Max: 1, Default: 0.08},
		{Name: "bass_pitch_env", Label: "Pitch Punch", Group: "core", Min: 0, Max: 0.6, Default: 0.15},
		{Name: "bass_attack", Label: "Punch", Group: "core", Min: 0, Max: 1.5, Default: 0.2},
		{Name: "bass_env_rate", Label: "Fade", Group: "core", Min: 0.5, Max: 12, Default: 2.0},
	},

	// Cymbal family: tune multiplier + transient/ring envelope pair +
	// tone/noise mixes (+ noise decay where the engine has a separate
	// noise envelope). NaN = the variant doesn't have that stage.
	"drum-hihat":      cymbalFamilyParamDefs(cymbalFamilySpec{wave: 2, fast: 180, tail: 35, toneMix: 0.85, noiseMix: 0.45, noiseDecay: 100}),
	"drum-open-hihat": cymbalFamilyParamDefs(cymbalFamilySpec{wave: 2, fast: 120, tail: 22, toneMix: 0.7, noiseMix: 0.9, noiseDecay: 18}),
	"drum-ride":       cymbalFamilyParamDefs(cymbalFamilySpec{wave: 0, fast: 40, tail: 8, toneMix: 1.0, noiseMix: 0.3, noiseDecay: 12}),
	"drum-crash":      cymbalFamilyParamDefs(cymbalFamilySpec{wave: 0, fast: 10, tail: 3, toneMix: 1.0, noiseMix: 0.35, noiseDecay: 8}),
	"drum-cowbell":    cymbalFamilyParamDefs(cymbalFamilySpec{wave: 0, fast: 260, tail: 9, toneMix: 1.0, noiseMix: 0.55, noiseDecay: 60}),
	"drum-shaker":     cymbalFamilyParamDefs(cymbalFamilySpec{wave: nan, fast: 200, tail: 25, toneMix: 0.6, noiseMix: 0.5, noiseDecay: nan}),

	// Snare family: semantic union (tone2/noise-tune/decays/mixes/attack);
	// each variant exposes only the stages it synthesizes. NaN = omitted.
	"drum-snare": snareFamilyParamDefs(snareFamilySpec{
		wave:        0,
		fundamental: 200, tone2: 330, tune: 1,
		toneDecay: 28, noiseDecay: 12, tailDecay: 18,
		toneMix: 0.40, noiseMix: 1.1, wireMix: 0.9, attack: 0.3, attackMax: 3,
	}),
	"drum-snare-rimshot": snareFamilyParamDefs(snareFamilySpec{
		wave:        0,
		fundamental: 500, tone2: 1050, tune: 1,
		toneDecay: 40, noiseDecay: 200, tailDecay: nan,
		toneMix: 1.0, noiseMix: 0.7, wireMix: nan, attack: 2.0, attackMax: 6,
	}),
	"drum-snare-sidestick": snareFamilyParamDefs(snareFamilySpec{
		wave:        0,
		fundamental: 500, tone2: 1200, tune: 1,
		toneDecay: 100, noiseDecay: 150, tailDecay: nan,
		toneMix: 0.5, noiseMix: 0.5, wireMix: nan, attack: 1.0, attackMax: 6,
	}),
	"drum-clap": snareFamilyParamDefs(snareFamilySpec{
		wave:        nan,
		fundamental: nan, tone2: nan, tune: 1,
		toneDecay: nan, noiseDecay: 7, tailDecay: 4,
		toneMix: nan, noiseMix: 0.15, wireMix: nan,
		// clap's "attack" is the burst sharpness (exp rate), not a boost.
		attack: 110, attackMin: 40, attackMax: 300,
	}),

	// Tom family: shared structure (pitch-swept fundamental + 1.5x/2.1x
	// overtones + stick transient + room tail), per-variant literals.
	"drum-tom":      tomFamilyParamDefs(tomFamilySpec{fundamental: 150, sweep: 18, ring: 2.8, o1: 0.5, o2: 0.25, stick: 0.35, room: 0.08}),
	"drum-tom-high": tomFamilyParamDefs(tomFamilySpec{fundamental: 170, sweep: 22, ring: 3.5, o1: 0.55, o2: 0.28, stick: 0.38, room: 0.06}),
	"drum-tom-low":  tomFamilyParamDefs(tomFamilySpec{fundamental: 90, sweep: 14, ring: 2.2, o1: 0.45, o2: 0.22, stick: 0.32, room: 0.10}),

	// FM family (native-deprecation migration): the curated knob set per
	// preset. Defaults MUST equal the PRESET_FM_* literals in
	// src/c/fmsynth.c — the at-default byte-parity contract in
	// native_knob_effect_test.go locks every value. Only operators the
	// preset uses are exposed; depth knobs only where the operator has an
	// outgoing mod-matrix edge (carriers without one ignore the slot).
	"fm-bass": fmFamilyParamDefs(fmFamilySpec{
		baseFreq: 55, pitchEnvAmount: 3, pitchEnvDecay: 0.06,
		opRatio: []float64{1, 1}, opDecay: []float64{0.3, 0.15},
		opDepth: map[int]float64{2: 2.5},
	}),
	// fm-bell / fm-epiano ship with NO pitch sweep (amount=0, decay=0); the
	// sweep knob pair is omitted because each knob alone is inaudible (the
	// engine gates on amount != 0 AND decay > 0) — "no silent no-op knobs"
	// outranks completeness. Re-voicing through the modular recipe covers
	// users who want a swept bell.
	"fm-bell": fmFamilyParamDefs(fmFamilySpec{
		baseFreq: 440, noPitchEnv: true,
		opRatio: []float64{1, 3.5}, opDecay: []float64{1.5, 1.2},
		opDepth: map[int]float64{2: 3},
	}),
	"fm-lead": fmFamilyParamDefs(fmFamilySpec{
		baseFreq: 220, pitchEnvAmount: 1.5, pitchEnvDecay: 0.04,
		opRatio: []float64{1, 2, 3}, opDecay: []float64{0.2, 0.12, 0.08},
		opDepth: map[int]float64{2: 3.5, 3: 2},
	}),
	"fm-epiano": fmFamilyParamDefs(fmFamilySpec{
		baseFreq: 220.0, noPitchEnv: true, // A3 = pitch-0 convention (was 261.63/C4 = +3 key bug)
		opRatio: []float64{1, 1, 2}, opDecay: []float64{0.8, 0.2, 0.5},
		opDepth: map[int]float64{2: 2.2},
	}),
	"fm-pluck": fmFamilyParamDefs(fmFamilySpec{
		baseFreq: 196, pitchEnvAmount: 2, pitchEnvDecay: 0.03,
		opRatio: []float64{1, 2}, opDecay: []float64{0.2, 0.04},
		opDepth: map[int]float64{2: 4},
	}),
}

// waveParamDef is the discrete generator-waveform selector every
// oscillator-bearing engine exposes (the native-deprecation follow-up
// "expose the generator type"). def is the waveform the engine has always
// used — at default the render is byte-identical (NaN-elision keeps the
// original literal path, and osc_wave's Sine/Square cases mirror the
// original expressions for the explicit-default browser path).
func waveParamDef(name string, def float64) ParamDef {
	return ParamDef{
		Name: name, Label: "Generator", Group: "core",
		Min: 0, Max: 3, Default: def,
		Enum: []string{"Sine", "Saw", "Square", "Triangle"},
	}
}

// nan marks a kickFamilySpec stage the variant doesn't synthesize.
var nan = math.NaN()

// kickFamilySpec carries one kick variant's curated-knob defaults (the C
// engine literals). NaN fields are omitted from the ParamDef list.
type kickFamilySpec struct {
	fundamental float64
	h2, h3, h4  float64
	env0, env1  float64
	peAmt       float64
	peRate      float64
	click       float64
	noise       float64
}

// kickFamilyParamDefs expands a spec into the ordered ParamDef list for one
// kick recipe. fundamental keeps its historical name + "core" group (it
// travels in synth_params.base); the family knobs use the kick_ prefix and
// route to PITCH/ENVELOPE/TONE sections via sectionForParam.
func kickFamilyParamDefs(s kickFamilySpec) []ParamDef {
	defs := []ParamDef{
		waveParamDef("kick_wave", 0),
		{Name: "fundamental", Label: "Fundamental", Group: "core", Min: 30, Max: 200, Default: s.fundamental, Unit: "Hz"},
		{Name: "kick_pitch_env_amount", Label: "Pitch Sweep", Group: "core", Min: 0, Max: 0.5, Default: s.peAmt},
		{Name: "kick_pitch_env_rate", Label: "Sweep Speed", Group: "core", Min: 5, Max: 100, Default: s.peRate},
		{Name: "kick_env0_rate", Label: "Boom Decay", Group: "core", Min: 1, Max: 20, Default: s.env0},
		{Name: "kick_env1_rate", Label: "Body Decay", Group: "core", Min: 1, Max: 30, Default: s.env1},
		{Name: "kick_h2_gain", Label: "2nd Harmonic", Group: "core", Min: 0, Max: 1, Default: s.h2},
	}
	add := func(v float64, d ParamDef) {
		if !math.IsNaN(v) {
			d.Default = v
			defs = append(defs, d)
		}
	}
	add(s.h3, ParamDef{Name: "kick_h3_gain", Label: "3rd Harmonic", Group: "core", Min: 0, Max: 1})
	add(s.h4, ParamDef{Name: "kick_h4_gain", Label: "4th Harmonic", Group: "core", Min: 0, Max: 1})
	add(s.click, ParamDef{Name: "kick_click", Label: "Click", Group: "core", Min: 0, Max: 1})
	add(s.noise, ParamDef{Name: "kick_noise", Label: "Thud", Group: "core", Min: 0, Max: 1})
	return defs
}

// cymbalFamilySpec carries one cymbal-family variant's curated-knob
// defaults (the C engine literals). cym_tune always defaults to 1.0.
type cymbalFamilySpec struct {
	wave       float64 // generator waveform default; NaN = no oscillator (shaker)
	fast       float64
	tail       float64
	toneMix    float64
	noiseMix   float64
	noiseDecay float64 // NaN → variant has no separate noise envelope
}

func cymbalFamilyParamDefs(s cymbalFamilySpec) []ParamDef {
	var defs []ParamDef
	if !math.IsNaN(s.wave) {
		defs = append(defs, waveParamDef("cym_wave", s.wave))
	}
	defs = append(defs, []ParamDef{
		{Name: "cym_tune", Label: "Metal Tune", Group: "core", Min: 0.5, Max: 2, Default: 1},
		{Name: "cym_env_fast", Label: "Attack Decay", Group: "core", Min: 5, Max: 400, Default: s.fast},
		{Name: "cym_env_tail", Label: "Ring Decay", Group: "core", Min: 1, Max: 120, Default: s.tail},
		{Name: "cym_tone_mix", Label: "Metal Level", Group: "core", Min: 0, Max: 1.5, Default: s.toneMix},
		{Name: "cym_noise_mix", Label: "Sizzle Level", Group: "core", Min: 0, Max: 1.5, Default: s.noiseMix},
	}...)
	if !math.IsNaN(s.noiseDecay) {
		defs = append(defs, ParamDef{Name: "cym_noise_decay", Label: "Sizzle Decay", Group: "core", Min: 2, Max: 200, Default: s.noiseDecay})
	}
	return defs
}

// snareFamilySpec carries one snare-family variant's curated-knob defaults.
// NaN fields are omitted (the variant doesn't synthesize that stage).
type snareFamilySpec struct {
	wave        float64 // generator waveform default; NaN = no oscillator (clap)
	fundamental float64
	tone2       float64
	tune        float64
	toneDecay   float64
	noiseDecay  float64
	tailDecay   float64
	toneMix     float64
	noiseMix    float64
	wireMix     float64
	attack      float64
	attackMin   float64 // 0 → default range [0, attackMax]
	attackMax   float64
}

func snareFamilyParamDefs(s snareFamilySpec) []ParamDef {
	var defs []ParamDef
	add := func(v float64, d ParamDef) {
		if !math.IsNaN(v) {
			d.Default = v
			defs = append(defs, d)
		}
	}
	add(s.wave, waveParamDef("snare_wave", 0))
	add(s.fundamental, ParamDef{Name: "fundamental", Label: "Fundamental", Group: "core", Min: 80, Max: 2000, Unit: "Hz"})
	add(s.tone2, ParamDef{Name: "snare_tone2_freq", Label: "Tone 2", Group: "core", Min: 80, Max: 4000, Unit: "Hz"})
	add(s.tune, ParamDef{Name: "snare_noise_tune", Label: "Noise Tune", Group: "core", Min: 0.5, Max: 2})
	add(s.toneDecay, ParamDef{Name: "snare_tone_decay", Label: "Tone Decay", Group: "core", Min: 5, Max: 300})
	add(s.noiseDecay, ParamDef{Name: "snare_noise_decay", Label: "Noise Decay", Group: "core", Min: 2, Max: 400})
	add(s.tailDecay, ParamDef{Name: "snare_tail_decay", Label: "Tail Decay", Group: "core", Min: 1, Max: 80})
	add(s.toneMix, ParamDef{Name: "snare_tone_mix", Label: "Tone Level", Group: "core", Min: 0, Max: 1.5})
	add(s.noiseMix, ParamDef{Name: "snare_noise_mix", Label: "Noise Level", Group: "core", Min: 0, Max: 1.5})
	add(s.wireMix, ParamDef{Name: "snare_wire_mix", Label: "Wires", Group: "core", Min: 0, Max: 1.5})
	add(s.attack, ParamDef{Name: "snare_attack", Label: "Snap", Group: "core", Min: s.attackMin, Max: s.attackMax})
	return defs
}

// tomFamilySpec carries one tom variant's curated-knob defaults (the C
// engine literals in render_tom / render_tom_high / render_tom_low).
type tomFamilySpec struct {
	fundamental float64
	sweep       float64
	ring        float64
	o1, o2      float64
	stick       float64
	room        float64
}

func tomFamilyParamDefs(s tomFamilySpec) []ParamDef {
	return []ParamDef{
		waveParamDef("tom_wave", 0),
		{Name: "fundamental", Label: "Fundamental", Group: "core", Min: 40, Max: 400, Default: s.fundamental, Unit: "Hz"},
		{Name: "tom_sweep_rate", Label: "Sweep Speed", Group: "core", Min: 4, Max: 80, Default: s.sweep},
		{Name: "tom_ring_rate", Label: "Ring Decay", Group: "core", Min: 0.5, Max: 12, Default: s.ring},
		{Name: "tom_o1_gain", Label: "Overtone 1", Group: "core", Min: 0, Max: 1, Default: s.o1},
		{Name: "tom_o2_gain", Label: "Overtone 2", Group: "core", Min: 0, Max: 1, Default: s.o2},
		{Name: "tom_stick", Label: "Stick", Group: "core", Min: 0, Max: 1, Default: s.stick},
		{Name: "tom_room", Label: "Room", Group: "core", Min: 0, Max: 0.6, Default: s.room},
	}
}

// fmFamilySpec carries one FM preset's curated-knob defaults (the C preset
// literals). opRatio/opDecay are indexed by operator (len == num_ops);
// opDepth is keyed by 1-based operator number, present only for operators
// with an outgoing mod-matrix edge.
type fmFamilySpec struct {
	baseFreq       float64
	pitchEnvAmount float64
	pitchEnvDecay  float64
	noPitchEnv     bool // preset has no sweep → omit the (coupled) knob pair
	opRatio        []float64
	opDecay        []float64
	opDepth        map[int]float64
}

// fmFamilyParamDefs expands a spec into the ordered ParamDef list for one FM
// recipe. Group "fm" routes every knob to the Synth tab's FM section.
func fmFamilyParamDefs(s fmFamilySpec) []ParamDef {
	wave := waveParamDef("fm_wave", 0)
	wave.Group = "fm"
	defs := []ParamDef{
		wave,
		{Name: "fm_base_freq", Label: "Base Pitch", Group: "fm", Min: 20, Max: 2000, Default: s.baseFreq, Unit: "Hz", Curve: "log"},
	}
	if !s.noPitchEnv {
		defs = append(defs,
			ParamDef{Name: "fm_pitch_env_amount", Label: "Pitch Sweep", Group: "fm", Min: -24, Max: 24, Default: s.pitchEnvAmount, Unit: "st"},
			ParamDef{Name: "fm_pitch_env_decay", Label: "Sweep Time", Group: "fm", Min: 0.01, Max: 0.5, Default: s.pitchEnvDecay, Unit: "s"},
		)
	}
	for i, ratio := range s.opRatio {
		n := string(rune('1' + i))
		defs = append(defs, ParamDef{
			Name: "fm_op" + n + "_ratio", Label: "Op " + n + " Ratio", Group: "fm",
			Min: 0.25, Max: 8, Default: ratio,
		})
		if depth, ok := s.opDepth[i+1]; ok {
			defs = append(defs, ParamDef{
				Name: "fm_op" + n + "_depth", Label: "Op " + n + " FM Depth", Group: "fm",
				Min: 0, Max: 8, Default: depth,
			})
		}
		defs = append(defs, ParamDef{
			Name: "fm_op" + n + "_decay", Label: "Op " + n + " Decay", Group: "fm",
			Min: 0.005, Max: 3, Default: s.opDecay[i], Unit: "s",
		})
	}
	return defs
}

// elideRecipeDefaults returns a copy of p with every key whose value exactly
// equals the recipe's declared ParamDef Default removed. The drum-family C
// engines do their arithmetic in double; a knob value that round-trips
// through the float32 ABI would perturb the constant by ~1e-8 even when the
// user never touched it (MergeRecipeDefaults fills every default in). Eliding
// exact-default values makes them arrive as the NaN sentinel instead, so the
// C engine keeps its exact double literal — "knob at default == factory
// constant", bit-for-bit. Values from MergeRecipeDefaults are copied from
// these same Defaults, so the equality is exact for unedited knobs.
func elideRecipeDefaults(recipeID string, p RecipeParams) RecipeParams {
	defs := WiredParamsForRecipe(recipeID)
	if len(defs) == 0 || len(p) == 0 {
		return p
	}
	defaults := make(map[string]float64, len(defs))
	for _, d := range defs {
		defaults[d.Name] = d.Default
	}
	out := make(RecipeParams, len(p))
	for k, v := range p {
		if dv, ok := defaults[k]; ok && v == dv {
			continue
		}
		out[k] = v
	}
	return out
}

// WiredParamsForRecipe returns the generic ParamDef list filtered down to the
// params the recipe's C renderer actually reads, appended with any per-recipe
// extras declared in recipeExtraParams. The returned slice preserves the
// order of GenericSynthParamDefs (pitch, decay, tone, attack, drive, body,
// color, brightness) so UI rendering order is stable across recipes; the
// section layout in synth_panel_sections.go regroups them by sonic role.
// Extras are appended at the end so existing UI layouts stay stable.
//
// Unknown IDs return nil so the caller falls back to GenericSynthParamDefs
// (preserves the historical behaviour for ad-hoc test recipes registered
// outside builtinRecipeDescriptors).
func WiredParamsForRecipe(id string) []ParamDef {
	wired, ok := recipeWiredParams[id]
	if !ok {
		return nil
	}
	keep := make(map[string]bool, len(wired))
	for _, name := range wired {
		keep[name] = true
	}
	all := GenericSynthParamDefs()
	out := make([]ParamDef, 0, len(all)+len(recipeExtraParams[id]))
	for _, d := range all {
		if keep[d.Name] {
			out = append(out, d)
		}
	}
	if extras, hasExtras := recipeExtraParams[id]; hasExtras {
		out = append(out, extras...)
	}
	// Phase-8A: every modular-migrated recipe additionally exposes the unified
	// pipeline stage params (osc/env/filter/drive + per-stage enable toggles)
	// so users can toggle/edit any stage on any instrument. Appended AFTER the
	// family/voice knobs (UI order: voice first, then stages). The collision set
	// is everything already on the recipe — modularStageParamDefs excludes any
	// stage name already taken (the fm-* group on FM recipes; the modular DRIVE
	// stage is not offered at all — see modular_stage_params.go header) so
	// there are no duplicate ParamDefs. Defaults are the migrated
	// off/identity state, so the default render is byte-identical to before.
	if IsModularMigratedRecipe(id) {
		existing := make(map[string]bool, len(out))
		for _, d := range out {
			existing[d.Name] = true
		}
		out = append(out, modularStageParamDefs(existing, migratedStageDefaults())...)
	}
	return out
}
