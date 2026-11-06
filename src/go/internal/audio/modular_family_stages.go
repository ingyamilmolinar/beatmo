package audio

import (
	"math"
	"sort"
	"strings"
)

// modular_family_stages.go — THE registry of gen-slot voice families.
//
// A "family" is one C gen_source voice (kick, tom, snare, cymbal, FM …). Before
// this table, teaching the app about a family meant editing about a dozen
// separate places: the ABI, the schema globals, the identity map, the ParamDef
// generator, a per-family stage-meta map, the UI's section router, its
// hidden-param gate, its concept-table classifier, its macro-category claims,
// its focus-renderer map, the plain-English glosses, and two test registries.
// Three of those silently disagreed the last time a family was added.
//
// Everything that is DATA now lives here, and every consumer derives from it.
// Adding a family is a row in modularFamilyStages plus its knob rows in
// modularStageKnobs.

// ModularFamilyStage names one gen-slot voice family.
type ModularFamilyStage struct {
	// Group is the Synth-tab stage group its slot-1 knobs live in, and the
	// value carried on each ParamDef.Group.
	Group string
	// Source is the gen_source value that activates the voice. A recipe whose
	// SEEDED gen1_source equals this is "a <Group> instrument", which is what
	// decides whether the knobs are shown — a config-first drum is identified
	// by its source, never by a family-knob name.
	Source float64
	// Enable is the *_enabled global gating the voice, empty when it has none.
	// It is never shown as a knob: the voice IS the instrument, so the pill
	// would only offer to silence it.
	Enable string
	// Percussive marks the families that get the percussion concept table in
	// the Synth tab rather than the melodic one.
	Percussive bool
}

var modularFamilyStages = []ModularFamilyStage{
	{Group: "bass", Source: 4, Percussive: true},
	{Group: "kick", Source: 5, Enable: "kick_enabled", Percussive: true},
	{Group: "tom", Source: 6, Percussive: true},
	{Group: "snare", Source: 7, Enable: "snare_enabled", Percussive: true},
	{Group: "cymbal", Source: 9, Percussive: true},
	{Group: "fm", Source: 10},
}

// FamilyStageForSource returns the family a gen_source belongs to.
func FamilyStageForSource(source float64) (ModularFamilyStage, bool) {
	for _, s := range modularFamilyStages {
		if s.Source == source {
			return s, true
		}
	}
	return ModularFamilyStage{}, false
}

// FamilyStageGroupOfParam reports the family-stage group a SCHEMA PARAM belongs
// to, matching the gen-bank naming (gen<N>_<field>) against modularStageKnobs.
//
// This is the ONLY way to ask the question: a ParamDef.Group is not a unique
// key, so there is deliberately no IsFamilyStageGroup(group) to reach for
// instead. "fm" names BOTH the modular FM stage (fm_algorithm, fm_op1_ratio, appended to
// every migrated recipe) and the gen-bank FM family (gen1_fm_r1). Testing the
// group alone therefore sweeps the FM stage's own knobs into the voice section
// — which empties the Synth tab's FM stage and makes it disappear. The gen-bank
// name is unambiguous, so it is the key.
func FamilyStageGroupOfParam(name string) (string, bool) {
	field, ok := strings.CutPrefix(name, "gen")
	if !ok {
		return "", false
	}
	i := strings.IndexByte(field, '_')
	if i <= 0 {
		return "", false
	}
	for _, c := range field[:i] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	kn, ok := modularStageKnobs[field[i+1:]]
	if !ok {
		return "", false
	}
	return kn.Group, true
}

// IsFamilyEnableParam reports whether a param is a family's enable gate.
func IsFamilyEnableParam(name string) bool {
	for _, s := range modularFamilyStages {
		if s.Enable != "" && s.Enable == name {
			return true
		}
	}
	return false
}

// modularStageKnob is the UI face of ONE gen-slot schema field: which stage
// group owns it, its label, and its enum when it is a selector. A field absent
// from modularStageKnobs stays hidden (ABI completeness only).
type modularStageKnob struct {
	Group string
	Label string
	Enum  []string
	// Macro is the Synth-tab concept card that claims this knob. Every visible
	// knob must be claimed by exactly one, which the UI's macro-discipline test
	// enforces.
	Macro string
}

// modularStageKnobUnits is the display unit of a surfaced knob, for the few
// family knobs whose raw number would otherwise read as a bare float ("3062.7"
// instead of "3.1 kHz"). Keyed like modularStageKnobs.
var modularStageKnobUnits = map[string]string{
	"cym_mode_lo": "Hz", "cym_mode_hi": "Hz", "cym_hf_corner": "Hz",
	"cym_tone_f": "Hz", "cym_body_f": "Hz", "cym_spark_f": "Hz",
	"cym_rattle_decay": "s", "cym_drive_delay": "s", "cym_rattle_hold": "s",
	"kick_hold": "s",
}

// stageKnobRange is a family knob's MUSICAL range (and curve) on the recipes
// that play that family. The ParamDef keeps the engine's wide bounds
// everywhere else; a recipe whose seeded gen1_source is the family gets these
// instead (applyFamilyKnobRanges), so the knob's travel covers useful values.
type stageKnobRange struct {
	Min, Max float64
	Curve    string
}

// modularStageKnobRanges is keyed like modularStageKnobs (prefix-stripped).
var modularStageKnobRanges = map[string]stageKnobRange{
	// BASS (source 4) — ranges of the former drum-sub-bass knobs, extended to
	// the seed values.
	"harm_mix":       {0, 1, ""},
	"pitch_env_amt":  {0, 1, ""},
	"pitch_env_rate": {1, 200, "log"},
	"atk_amt":        {0, 1.5, ""},
	"atk_rate":       {1, 400, "log"},
	"env_fast_rate":  {0.02, 40, "log"}, // holds the Length dial's whole span (one knob, one meaning)
	"sat_k":          {0, 4, ""},
	"out_scale":      {0, 2, ""},
}

// applyFamilyKnobRanges narrows the slot-1 family knobs of a recipe that plays
// that family (seed gen1_source) to their musical range. The seed value is
// always kept inside the range.
func applyFamilyKnobRanges(params []ParamDef, seed RecipeParams) {
	fam, ok := FamilyStageForSource(seed["gen1_source"])
	if !ok {
		return
	}
	for i := range params {
		field, ok := strings.CutPrefix(params[i].Name, "gen1_")
		if !ok {
			continue
		}
		r, ok := modularStageKnobRanges[field]
		if !ok || modularStageKnobs[field].Group != fam.Group {
			continue
		}
		params[i].Min = math.Min(r.Min, params[i].Default)
		params[i].Max = math.Max(r.Max, params[i].Default)
		if r.Curve != "" {
			params[i].Curve = r.Curve
		}
	}
}

// AnyFamilyGroup is the Group of a gen-slot knob every family reads (the
// generator waveform): FamilyStageGroupOfParam reports it, and the Synth tab
// shows such a knob on whichever family is active instead of on exactly one.
const AnyFamilyGroup = "family"

// modularStageKnobs is keyed by the schema field name WITHOUT its gen<N>_
// prefix. Only slot 1 is surfaced; slots 2..12 stay hidden.
//
// The labels deliberately reuse the wired recipes' existing wording so the
// vocabulary (and its plain-English glosses) is unchanged for the user.
var modularStageKnobs = map[string]modularStageKnob{
	// ── SHARED by every family ──
	// gen_wave is the voice's generator waveform and EVERY family's C voice reads
	// it (bass/kick/tom/snare/cymbal/FM) — the knob the legacy recipes offered as
	// "Generator" (snare_wave, fm_wave, …). It lives in AnyFamilyGroup so it shows
	// on whichever family the recipe plays; the config-first conversion had left
	// it hidden, so a config-first snare or FM bass lost the control.
	"wave": {AnyFamilyGroup, "Generator", []string{"Sine", "Saw", "Square", "Triangle"}, "source"},

	// ── BASS (source 4): the analytic dual-oscillator voice (sub-bass). These
	// are the generic analytic gen-slot fields; only source 4 reads them. ──
	"harm_mix":       {"bass", "Harmonics", nil, "tone"},
	"pitch_env_amt":  {"bass", "Pitch Drop", nil, "pitchdrop"},
	"pitch_env_rate": {"bass", "Drop Speed", nil, "pitchdrop"},
	"atk_amt":        {"bass", "Punch", nil, "punch"},
	"atk_rate":       {"bass", "Punch Speed", nil, "punch"},
	"env_fast_rate":  {"bass", "Body Decay", nil, "length"},
	"sat_k":          {"bass", "Saturation", nil, "grit"},
	"out_scale":      {"bass", "Level", nil, "boom"},

	// ── KICK (source 5) ──
	"kick_variant": {"kick", "Variant", []string{"Base", "Deep", "Punchy", "Lo-Fi", "Tight", "Hybrid", "Modal", "Acoustic", "Physical"}, "source"},
	"kick_h2":      {"kick", "2nd Harmonic", nil, "boom"},
	"kick_h3":      {"kick", "3rd Harmonic", nil, "boom"},
	"kick_h4":      {"kick", "4th Harmonic", nil, "boom"},
	"kick_env0":    {"kick", "Body Decay", nil, "length"},
	"kick_env1":    {"kick", "Harmonic Decay", nil, "length"},
	"kick_pe_amt":  {"kick", "Pitch Drop", nil, "pitchdrop"},
	"kick_pe_rate": {"kick", "Drop Speed", nil, "pitchdrop"},
	"kick_click":   {"kick", "Click", nil, "punch"},
	"kick_noise":   {"kick", "Thud", nil, "boom"}, // "boom", not "punch": matches where the shipped table already put it
	"kick_attack":  {"kick", "Punch", nil, "punch"},
	"kick_fade":    {"kick", "Tail", nil, "length"},
	"kick_sat":     {"kick", "Saturation", nil, "grit"},
	// variant 6 (modal) + variant 7 (acoustic) extras
	"kick_mode_detune": {"kick", "Mode Split", nil, "boom"},
	"kick_mode_gain":   {"kick", "Mode Body", nil, "boom"},
	"kick_mode_decay":  {"kick", "Mode Ring", nil, "length"},
	"kick_reverb":      {"kick", "Reverb", nil, "length"},
	// variant 8 (physical) extras
	"kick_hold": {"kick", "Body Hold", nil, "length"},
	"kick_hiss": {"kick", "Air", nil, "boom"},

	// ── SNARE (source 7) ──
	"snare_variant": {"snare", "Variant", []string{"Snare", "Rimshot", "Sidestick", "Acoustic"}, "source"},
	"snare_tone2":   {"snare", "Tone 2", nil, "boom"},
	"snare_tune":    {"snare", "Noise Tune", nil, "tone"},
	"snare_tone_d":  {"snare", "Body Decay", nil, "length"},
	"snare_noise_d": {"snare", "Noise Decay", nil, "length"},
	"snare_tail_d":  {"snare", "Tail Decay", nil, "length"},
	"snare_tone_m":  {"snare", "Tone Level", nil, "boom"},
	"snare_noise_m": {"snare", "Noise Level", nil, "tone"},
	"snare_wire_m":  {"snare", "Wires", nil, "tone"},
	"snare_attack":  {"snare", "Snap", nil, "punch"},

	// ── TOM (source 6) ──
	"tom_variant": {"tom", "Variant", []string{"Tom", "High Tom", "Low Tom"}, "source"},
	"tom_sweep":   {"tom", "Sweep Speed", nil, "pitchdrop"},
	"tom_ring":    {"tom", "Ring Decay", nil, "length"},
	"tom_o1":      {"tom", "Overtone 1", nil, "boom"},
	"tom_o2":      {"tom", "Overtone 2", nil, "boom"},
	"tom_stick":   {"tom", "Stick", nil, "punch"},
	"tom_room":    {"tom", "Room", nil, "length"},

	// ── CYMBAL (source 9) ──
	"cym_variant":  {"cymbal", "Variant", []string{"Hi-Hat", "Open Hi-Hat", "Cowbell", "Shaker", "Ride", "Crash"}, "source"},
	"cym_tune":     {"cymbal", "Metal Tune", nil, "tone"},
	"cym_env_fast": {"cymbal", "Attack Decay", nil, "punch"},
	"cym_env_tail": {"cymbal", "Ring Decay", nil, "length"},
	"cym_tone_m":   {"cymbal", "Metal Level", nil, "tone"},
	"cym_noise_m":  {"cymbal", "Sizzle Level", nil, "tone"},
	"cym_noise_d":  {"cymbal", "Sizzle Decay", nil, "length"},
	// ── Phase-18 modal plate (always on the hi-hat / ride; on the open hi-hat
	// and crash once Plate Modes >= 1) ──
	"cym_modes":      {"cymbal", "Plate Modes", nil, "source"},
	"cym_mode_lo":    {"cymbal", "Lowest Mode", nil, "boom"},
	"cym_mode_hi":    {"cymbal", "Highest Mode", nil, "tone"},
	"cym_decay_tilt": {"cymbal", "Highs Fade", nil, "length"},
	"cym_hf_corner":  {"cymbal", "Tick Corner", nil, "punch"},
	"cym_beat":       {"cymbal", "Shimmer", nil, "tone"},
	"cym_tone_f":     {"cymbal", "Strike Tone", nil, "tone"},
	"cym_tone_w":     {"cymbal", "Strike Width", nil, "tone"},
	"cym_body_f":     {"cymbal", "Body Tone", nil, "boom"},
	"cym_body_w":     {"cymbal", "Body Width", nil, "boom"},
	"cym_body":       {"cymbal", "Body", nil, "boom"},
	"cym_spark_f":    {"cymbal", "Sparkle Tone", nil, "punch"},
	"cym_spark_w":    {"cymbal", "Sparkle Width", nil, "punch"},
	"cym_spark":      {"cymbal", "Sparkle", nil, "punch"},
	"cym_wash":       {"cymbal", "Wash", nil, "tone"},
	"cym_attack":     {"cymbal", "Softness", nil, "punch"},
	// Hi-hat rattle (read on the hi-hat / open hi-hat variants only).
	"cym_rattle":       {"cymbal", "Rattle", nil, "tone"},
	"cym_rattle_rate":  {"cymbal", "Rattle Speed", nil, "tone"},
	"cym_rattle_decay": {"cymbal", "Rattle Length", nil, "length"},
	"cym_drive_delay":  {"cymbal", "Wire Delay", nil, "punch"},
	"cym_rattle_hold":  {"cymbal", "Buzz Hold", nil, "length"},

	// ── FM (source 10) ──
	"fm_variant": {"fm", "Variant", []string{"Bass", "Bell", "E-Piano", "Lead", "Pluck"}, "source"},
	// "source", not "boom": the FM family is the only non-percussive one, so its
	// knobs land in the MELODIC table, where "boom" aliases to Body — a card
	// gated on the resonator body model, which FM never turns on. Base Pitch is
	// part of the engine's character, so Sound is both its natural home and a
	// card that actually shows.
	"fm_base":     {"fm", "Base Pitch", nil, "source"},
	"fm_pe_amt":   {"fm", "Pitch Sweep", nil, "pitchdrop"},
	"fm_pe_decay": {"fm", "Sweep Time", nil, "pitchdrop"},
	"fm_r1":       {"fm", "Op 1 Ratio", nil, "source"},
	"fm_r2":       {"fm", "Op 2 Ratio", nil, "source"},
	"fm_r3":       {"fm", "Op 3 Ratio", nil, "source"},
	"fm_r4":       {"fm", "Op 4 Ratio", nil, "source"},
	"fm_d1":       {"fm", "Op 1 FM Depth", nil, "source"},
	"fm_d2":       {"fm", "Op 2 FM Depth", nil, "source"},
	"fm_d3":       {"fm", "Op 3 FM Depth", nil, "source"},
	"fm_d4":       {"fm", "Op 4 FM Depth", nil, "source"},
	"fm_dec1":     {"fm", "Op 1 Decay", nil, "length"},
	"fm_dec2":     {"fm", "Op 2 Decay", nil, "length"},
	"fm_dec3":     {"fm", "Op 3 Decay", nil, "length"},
	"fm_dec4":     {"fm", "Op 4 Decay", nil, "length"},
}

// GenSlotParamsForMacro returns the SLOT-1 param names whose knob is claimed by
// the given Synth-tab macro card, in the registry's declaration order.
//
// The Macro field on each modularStageKnob is what makes a new family's knobs
// reachable from the root of the Synth tab without touching the category
// tables: every visible non-gain param must be claimed by exactly one card
// (TestMacroGroups_EveryInstrumentHasCategories), and listing them by hand is how
// the kick and snare knobs came to be enumerated in two places at once.
//
// The macro names are written in the PERCUSSION table's vocabulary; the melodic
// table names some of the same ideas differently, and the UI aliases them.
func GenSlotParamsForMacro(macro string) []string {
	if macro == "" {
		return nil
	}
	var out []string
	for _, field := range modularStageKnobFields() {
		if modularStageKnobs[field].Macro == macro {
			out = append(out, "gen1_"+field)
		}
	}
	return out
}

// modularStageKnobFields returns the knob fields in a STABLE order (grouped by
// family in registry order, then alphabetically within a family), so the cards
// list their params identically on every run — a map range would reshuffle the
// Synth tab between frames.
func modularStageKnobFields() []string {
	var out []string
	var shared []string
	for field, kn := range modularStageKnobs {
		if kn.Group == AnyFamilyGroup {
			shared = append(shared, field)
		}
	}
	sort.Strings(shared)
	out = append(out, shared...)
	for _, st := range modularFamilyStages {
		var fam []string
		for field, kn := range modularStageKnobs {
			if kn.Group == st.Group {
				fam = append(fam, field)
			}
		}
		sort.Strings(fam)
		out = append(out, fam...)
	}
	return out
}

// StageKnobLabelForParam returns the human label the registry declares for a
// SCHEMA PARAM (gen<N>_<field>), matching by name the way
// FamilyStageGroupOfParam does.
//
// The labels deliberately reuse the wired recipes' existing wording ("Op 2
// Decay", "Metal Tune", "Sweep Speed"), so a family that moves to a config
// seed keeps the vocabulary its users already know. Without this the Synth tab
// falls back to prettifying the raw id and shows "Fm Dec2" and "Cym Env Fast".
func StageKnobLabelForParam(name string) (string, bool) {
	field, ok := strings.CutPrefix(name, "gen")
	if !ok {
		return "", false
	}
	i := strings.IndexByte(field, '_')
	if i <= 0 {
		return "", false
	}
	for _, c := range field[:i] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	kn, ok := modularStageKnobs[field[i+1:]]
	if !ok || kn.Label == "" {
		return "", false
	}
	return kn.Label, true
}

// StageKnobFor returns the UI face of a gen-slot field (prefix-stripped name),
// for consumers that need the label/group/macro without the ParamDef.
func StageKnobFor(field string) (group, label, macro string, enum []string, ok bool) {
	k, found := modularStageKnobs[field]
	if !found {
		return "", "", "", nil, false
	}
	return k.Group, k.Label, k.Macro, k.Enum, true
}
