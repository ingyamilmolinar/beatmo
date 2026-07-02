package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// synth_param_display.go — the cosmetic display-name + value-formatter chain
// for the Synth tab (and the Sampler tab's knob captions). Every knob caption
// resolves a human-readable NAME for its param id and a unit-aware VALUE
// string here, so no caption ever leaks a raw snake_case id ("osc_detune") or
// a bare unitless number ("0.40").
//
// This file is purely presentational: it never touches audio rendering, the
// ParamDef ABI, defaults, or the NaN sentinels. It only maps a param id +
// value + unit to display strings. To enumerate the live param universe it
// reads audio.RecipeRegistrations() (read-only) — see
// synth_param_display_test.go, which asserts EVERY reachable ParamDef resolves
// to a non-empty name that is not its raw id.

// synthParamNameOverrides maps known param ids to short, human-readable knob
// captions. The fallback prettifier (prettifySynthParamID) handles everything
// not listed here — chiefly the genN_* unified-generator families — but the
// well-known voice/stage knobs get curated names so the caption reads naturally
// ("Detune", "Op1 Ratio") instead of a prefix-stripped guess.
//
// Disambiguation note: there are two "decay"-flavoured knobs that can share a
// detail pane — the generic post `decay` MULTIPLIER (×) and the envelope
// `amp_decay` (seconds). They get distinct names ("Decay ×" vs "Decay") so the
// pane never shows two captions both reading "Decay".
var synthParamNameOverrides = map[string]string{
	// ---- generic post knobs ----
	"pitch":      "Tune",
	"decay":      "Decay ×", // multiplier — disambiguated from amp_decay (seconds)
	"tone":       "Tone",
	"drive":      "Drive",
	"body":       "Body",
	"brightness": "Bright",
	"gain":       "Gain",

	// ---- OSC stage ----
	"osc_type":   "Wave",
	"osc_detune": "Detune",
	"osc_octave": "Octave",

	// ---- amp envelope (ADSR) ----
	"amp_attack":  "Attack",
	"amp_decay":   "Decay",
	"amp_sustain": "Sustain",
	"amp_release": "Release",
	"amp_curve":   "Curve",

	// ---- filter stage ----
	"filter_type":      "Type",
	"filter_cutoff":    "Cutoff",
	"filter_resonance": "Resonance",

	// ---- FM stage (modular) ----
	"fm_algorithm":        "Algorithm",
	"fm_wave":             "Wave",
	"fm_base_freq":        "Base Freq",
	"fm_op1_ratio":        "Op1 Ratio",
	"fm_op1_level":        "Op1 Level",
	"fm_op1_depth":        "Op1 Depth",
	"fm_op1_decay":        "Op1 Decay",
	"fm_op2_ratio":        "Op2 Ratio",
	"fm_op2_level":        "Op2 Level",
	"fm_op2_depth":        "Op2 Depth",
	"fm_op2_decay":        "Op2 Decay",
	"fm_op3_ratio":        "Op3 Ratio",
	"fm_op3_level":        "Op3 Level",
	"fm_op3_depth":        "Op3 Depth",
	"fm_op3_decay":        "Op3 Decay",
	"fm_op4_ratio":        "Op4 Ratio",
	"fm_op4_level":        "Op4 Level",
	"fm_op4_depth":        "Op4 Depth",
	"fm_pitch_env_amount": "Pitch Env",
	"fm_pitch_env_decay":  "Pitch Env Decay",

	// ---- PITCH-env modulator stage ----
	"pitchenv_amt":   "Amount",
	"pitchenv_decay": "Decay",

	// ---- LFO modulator stage ----
	"lfo_rate":  "Rate",
	"lfo_depth": "Depth",

	// ---- BURST modulator stage ----
	"burst_sharp": "Sharpness",
	"burst1_amp":  "Hit 1 Level",
	"burst1_off":  "Hit 1 Time",
	"burst2_amp":  "Hit 2 Level",
	"burst2_off":  "Hit 2 Time",
	"burst3_amp":  "Hit 3 Level",
	"burst3_off":  "Hit 3 Time",
	"burst4_amp":  "Hit 4 Level",
	"burst4_off":  "Hit 4 Time",

	// ---- noise generator (hidden seed has no UI, but cover the visible ones) ----
	"noise_draws":   "Draws",
	"noise_prelude": "Prelude",
	"noise_seed":    "Seed",

	// ---- voice fundamentals ----
	"fundamental":   "Pitch",
	"voice_freq_hz": "Pitch",

	// ---- a few high-frequency family knobs whose prefix-stripped form is
	// terse or ambiguous; the prettifier covers the rest of each family ----
	"kick_click": "Click",
	"kick_noise": "Noise",
	"kick_wave":  "Wave",
	"snare_wave": "Wave",
	"cym_wave":   "Wave",
	"tom_wave":   "Wave",
	"bass_wave":  "Wave",
}

// synthParamNameKeys maps a param id to the i18n key for its LOCALIZED display
// caption. It is the locale-aware sibling of synthParamNameOverrides: the
// override table remains the canonical-English source of truth (and the
// English catalog mirrors it verbatim, so the English locale is byte-identical
// to the pre-i18n behaviour). Several ids share a key where their caption is the
// same word (every "Wave" knob, both "Pitch" voice knobs). The two regular
// families — FM operators (fm_opN_*) and burst hits (burstN_amp/off) — are NOT
// listed here; they are composed in composeOpLabel so the universal "OpN"/
// "Hit N" prefix stays English while only the noun follows the locale.
var synthParamNameKeys = map[string]i18n.Key{
	// ---- generic post knobs ----
	"pitch":      i18n.KeySynthKnobTune,
	"decay":      i18n.KeySynthKnobDecayMult,
	"tone":       i18n.KeySynthKnobTone,
	"drive":      i18n.KeySynthKnobDrive,
	"body":       i18n.KeySynthKnobBody,
	"brightness": i18n.KeySynthKnobBright,
	"gain":       i18n.KeySynthKnobGain,

	// ---- OSC stage ----
	"osc_type":   i18n.KeySynthKnobWave,
	"osc_detune": i18n.KeySynthKnobDetune,
	"osc_octave": i18n.KeySynthKnobOctave,

	// ---- amp envelope (ADSR) ----
	"amp_attack":  i18n.KeySynthKnobAttack,
	"amp_decay":   i18n.KeySynthKnobDecay,
	"amp_sustain": i18n.KeySynthKnobSustain,
	"amp_release": i18n.KeySynthKnobRelease,
	"amp_curve":   i18n.KeySynthKnobCurve,

	// ---- filter stage ----
	"filter_type":      i18n.KeySynthKnobType,
	"filter_cutoff":    i18n.KeySynthKnobCutoff,
	"filter_resonance": i18n.KeySynthKnobResonance,

	// ---- FM stage singletons (the OpN families are composed) ----
	"fm_algorithm":        i18n.KeySynthKnobAlgorithm,
	"fm_wave":             i18n.KeySynthKnobWave,
	"fm_base_freq":        i18n.KeySynthKnobBaseFreq,
	"fm_pitch_env_amount": i18n.KeySynthKnobPitchEnv,
	"fm_pitch_env_decay":  i18n.KeySynthKnobPitchEnvDecay,

	// ---- PITCH-env modulator stage ----
	"pitchenv_amt":   i18n.KeySynthKnobAmount,
	"pitchenv_decay": i18n.KeySynthKnobDecay,

	// ---- LFO modulator stage ----
	"lfo_rate":  i18n.KeySynthKnobRate,
	"lfo_depth": i18n.KeySynthKnobDepth,

	// ---- BURST modulator stage (the HitN families are composed) ----
	"burst_sharp": i18n.KeySynthKnobSharpness,

	// ---- noise generator ----
	"noise_draws":   i18n.KeySynthKnobDraws,
	"noise_prelude": i18n.KeySynthKnobPrelude,
	"noise_seed":    i18n.KeySynthKnobSeed,

	// ---- voice fundamentals ----
	"fundamental":   i18n.KeySynthKnobPitch,
	"voice_freq_hz": i18n.KeySynthKnobPitch,

	// ---- drum-family well-known knobs ----
	"kick_click": i18n.KeySynthKnobClick,
	"kick_noise": i18n.KeySynthKnobNoise,
	"kick_wave":  i18n.KeySynthKnobWave,
	"snare_wave": i18n.KeySynthKnobWave,
	"cym_wave":   i18n.KeySynthKnobWave,
	"tom_wave":   i18n.KeySynthKnobWave,
	"bass_wave":  i18n.KeySynthKnobWave,
}

// synthEnumLabelKeys maps a canonical enum string (the audio.ParamDef.Enum
// storage form) to the i18n key for its localized DISPLAY label. The canonical
// strings stay the storage/round-trip form — this is a display-only transform.
// Universal members ("FM", "2-op") are intentionally absent and pass through
// localizeEnumLabel unchanged.
var synthEnumLabelKeys = map[string]i18n.Key{
	"Sine":        i18n.KeySynthEnumSine,
	"Saw":         i18n.KeySynthEnumSaw,
	"Square":      i18n.KeySynthEnumSquare,
	"Triangle":    i18n.KeySynthEnumTriangle,
	"Noise White": i18n.KeySynthEnumNoiseWhite,
	"Noise Pink":  i18n.KeySynthEnumNoisePink,
	"Parallel":    i18n.KeySynthEnumParallel,
	"3-op chain":  i18n.KeySynthEnum3opChain,
	"4-op stack":  i18n.KeySynthEnum4opStack,
	"Linear":      i18n.KeySynthEnumLinear,
	"Exponential": i18n.KeySynthEnumExponential,
	"Low-pass":    i18n.KeySynthEnumLowPass,
	"High-pass":   i18n.KeySynthEnumHighPass,
	"Band-pass":   i18n.KeySynthEnumBandPass,
	"On":          i18n.KeySynthEnumOn,
	"Off":         i18n.KeySynthEnumOff,
}

// localizeEnumLabel returns the localized display label for a canonical enum
// string, passing unmapped / universal strings through unchanged. In the
// English locale the catalog mirrors the canonical strings, so the result is
// byte-identical to the raw enum member.
func localizeEnumLabel(canonical string) string {
	if k, ok := synthEnumLabelKeys[canonical]; ok {
		return i18n.T(k)
	}
	return canonical
}

// composeOpLabel localizes the two regular knob families whose canonical caption
// is "<universal prefix> <noun>": FM operators ("fm_op1_ratio" → "Op1 Ratio")
// and burst hits ("burst1_amp" → "Hit 1 Level"). The "OpN"/"Hit N" prefix is
// universal audio jargon and stays English; only the noun follows the locale.
// Because the English noun keys mirror the canonical words, the English result
// is byte-identical to the old literal-map captions.
func composeOpLabel(name string) (string, bool) {
	if n, noun, ok := splitIndexedFamily(name, "fm_op"); ok {
		var k i18n.Key
		switch noun {
		case "ratio":
			k = i18n.KeySynthNounRatio
		case "level":
			k = i18n.KeySynthNounLevel
		case "depth":
			k = i18n.KeySynthKnobDepth
		case "decay":
			k = i18n.KeySynthKnobDecay
		default:
			return "", false
		}
		return fmt.Sprintf("Op%d %s", n, i18n.T(k)), true
	}
	if n, noun, ok := splitIndexedFamily(name, "burst"); ok {
		var k i18n.Key
		switch noun {
		case "amp":
			k = i18n.KeySynthNounLevel
		case "off":
			k = i18n.KeySynthNounTime
		default:
			return "", false
		}
		return fmt.Sprintf("%s %d %s", i18n.T(i18n.KeySynthNounHit), n, i18n.T(k)), true
	}
	return "", false
}

// splitIndexedFamily parses ids of the shape "<prefix><N>_<noun>" (e.g.
// "fm_op1_ratio", "burst3_off"), returning the integer index and the noun. It
// reports false for non-matching ids — including the singleton members of the
// same stage ("burst_sharp", "fm_base_freq") whose first post-prefix character
// is not a digit.
func splitIndexedFamily(name, prefix string) (int, string, bool) {
	if !strings.HasPrefix(name, prefix) {
		return 0, "", false
	}
	rest := name[len(prefix):] // e.g. "1_ratio"
	us := strings.IndexByte(rest, '_')
	if us <= 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(rest[:us])
	if err != nil {
		return 0, "", false
	}
	return n, rest[us+1:], true
}

// synthFamilyPrefixes are the family/stage prefixes the fallback prettifier
// strips before Title-Casing the remainder. The genN_ prefix (gen1_, gen10_…)
// is stripped via a regexp-free numeric check in prettifySynthParamID.
var synthFamilyPrefixes = []string{
	"osc_", "amp_", "fm_", "filter_", "lfo_", "pitchenv_", "burst_",
	"noise_", "post_", "kick_", "snare_", "cym_", "tom_", "bass_", "voice_",
}

// synthParamDisplayName resolves the human caption NAME for a param id. The
// chain is: (1) the curated override table, then (2) the fallback prettifier
// (strip family/genN prefix → underscores to spaces → Title Case). The result
// is always non-empty and never the raw snake_case id (the prettifier collapses
// "_" so even a single-token id like "tone" comes back Title-Cased; the
// override table catches the ones that would otherwise read oddly).
func synthParamDisplayName(name string) string {
	if name == "" {
		return ""
	}
	// (1) curated localized labels, (2) the composed OpN/HitN families,
	// (3) the canonical-English override table (no localized key yet),
	// (4) the fallback prettifier.
	if k, ok := synthParamNameKeys[name]; ok {
		return i18n.T(k)
	}
	if lbl, ok := composeOpLabel(name); ok {
		return lbl
	}
	if disp, ok := synthParamNameOverrides[name]; ok {
		return disp
	}
	return prettifySynthParamID(name)
}

// prettifySynthParamID is the fallback name builder: strip a known family or
// genN_ prefix, replace underscores with spaces, and Title-Case each word.
// Guarantees a non-empty result that differs from the raw id whenever the id
// carries an underscore or a lowercase first letter.
func prettifySynthParamID(name string) string {
	core := name
	// Strip a genN_ prefix (gen1_, gen2_, … gen12_).
	if rest, ok := stripGenPrefix(core); ok {
		core = rest
	} else {
		for _, p := range synthFamilyPrefixes {
			if strings.HasPrefix(core, p) {
				core = core[len(p):]
				break
			}
		}
	}
	if core == "" {
		core = name
	}
	words := strings.Split(core, "_")
	for i, w := range words {
		words[i] = titleWord(w)
	}
	out := strings.Join(words, " ")
	if out == "" {
		return titleWord(name)
	}
	return out
}

// stripGenPrefix removes a leading "genN_" (N decimal digits) and reports
// whether it matched. Avoids a regexp dependency for a hot-path string op.
func stripGenPrefix(name string) (string, bool) {
	if !strings.HasPrefix(name, "gen") {
		return name, false
	}
	i := 3
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 3 || i >= len(name) || name[i] != '_' {
		return name, false
	}
	return name[i+1:], true
}

// titleWord upper-cases the first letter of a single token, leaving the rest
// as-is so already-cased abbreviations (e.g. an all-caps acronym) survive.
func titleWord(w string) string {
	if w == "" {
		return ""
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

// stepDecimals returns how many fractional digits are needed so that two values
// one `step` apart render as distinct strings: 0.005 → 3, 0.05 → 2, 0.5 → 1,
// 1 → 0, 5 → 0. A non-positive / non-finite step yields 0 (caller falls back to
// the unit's natural precision). Capped at 6 so a pathologically tiny step can't
// blow up the format width.
func stepDecimals(step float64) int {
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return 0
	}
	// ceil(-log10(step)) with a tiny epsilon to absorb float error on exact
	// decade boundaries (e.g. step 0.1 → exactly 1, not 2).
	d := int(math.Ceil(-math.Log10(step) - 1e-9))
	if d < 0 {
		d = 0
	}
	if d > 6 {
		d = 6
	}
	return d
}

// formatParamValue is the single value formatter shared by every synth-tab knob
// caption AND the Sampler knob captions, so values read coherently across the
// whole panel. It renders at each unit's NATURAL precision; it is exactly
// formatParamValueStep with an infinite (i.e. coarsest) step. Rules by unit:
//
//	"Hz"            → SI-prefixed: "8.0 kHz" above 1000 Hz (one decimal, kHz),
//	                  "62 Hz" below (integer, no trailing ".00").
//	"s"            → seconds with two decimals: "0.30 s".
//	"cents"        → signed integer cents: "+12 c" / "0 c".
//	"st"           → signed integer semitones: "+3 st".
//	"dB"           → signed integer dB: "+0 dB".
//	"×" / "x"      → multiplier with one decimal: "1.0×".
//	"%"            → integer percent: "40%".
//	""  (unitless) → one decimal: "0.4".
//	anything else  → two decimals + the raw unit suffix (forward-compatible).
func formatParamValue(value float64, unit string) string {
	return formatParamValueStep(value, math.Inf(1), unit)
}

// formatParamValueStep is the resolution-aware sibling of formatParamValue: it
// renders `value` with at least as many fractional digits as the active `step`
// resolution requires (but never fewer than the unit's natural precision), so a
// fine step (e.g. x0.005) stays visible in the rendered number instead of being
// rounded away to the natural 1–2 decimals. Passing a coarse/infinite step
// reproduces formatParamValue byte-for-byte. `step` is in the SAME units as the
// rendered number, so callers that scale the value (percent ×100, kHz ÷1000)
// must scale the step the same way.
func formatParamValueStep(value, step float64, unit string) string {
	switch unit {
	case "Hz":
		if math.Abs(value) >= 1000 {
			d := max(1, stepDecimals(step/1000)) // step is in Hz; the readout is kHz
			return fmt.Sprintf("%.*f kHz", d, value/1000)
		}
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%.*f Hz", d, value)
		}
		return fmt.Sprintf("%d Hz", int(math.Round(value)))
	case "s":
		return fmt.Sprintf("%.*f s", max(2, stepDecimals(step)), value)
	case "ms":
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%.*f ms", d, value)
		}
		return fmt.Sprintf("%d ms", int(math.Round(value)))
	case "cents", "c":
		// "cents" is the one value unit with a sensible Spanish form ("ct" =
		// centésimas); the others (st/dB/Hz/ms/s/×/%) are international.
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%+.*f %s", d, value, i18n.T(i18n.KeyUnitCents))
		}
		return fmt.Sprintf("%+d %s", int(math.Round(value)), i18n.T(i18n.KeyUnitCents))
	case "st":
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%+.*f st", d, value)
		}
		return fmt.Sprintf("%+d st", int(math.Round(value)))
	case "dB":
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%+.*f dB", d, value)
		}
		return fmt.Sprintf("%+d dB", int(math.Round(value)))
	case "×", "x", "mult":
		return fmt.Sprintf("%.*f×", max(1, stepDecimals(step)), value)
	case "%":
		if d := stepDecimals(step); d > 0 {
			return fmt.Sprintf("%.*f%%", d, value)
		}
		return fmt.Sprintf("%d%%", int(math.Round(value)))
	case "":
		return fmt.Sprintf("%.*f", max(1, stepDecimals(step)), value)
	default:
		return fmt.Sprintf("%.*f %s", max(2, stepDecimals(step)), value, unit)
	}
}

// formatSynthParamValue formats a ParamDef's current value for a caption,
// special-casing the generic post knobs whose semantic unit isn't carried in
// ParamDef.Unit (they ship as unitless floats but read best as a multiplier or
// percentage). Everything else routes through formatParamValue by Unit.
func formatSynthParamValue(def audio.ParamDef, value float64) string {
	return formatSynthParamValueStep(def, value, math.Inf(1))
}

// formatSynthParamValueStep is the resolution-aware sibling of
// formatSynthParamValue: it threads the active `step` (real param units) through
// to formatParamValueStep so the rendered number gains the decimals the current
// resolution needs. The generic-post special cases scale the step exactly as
// they scale the value (×100 for the percent knobs) so the precision tracks the
// displayed magnitude. An infinite step reproduces formatSynthParamValue.
func formatSynthParamValueStep(def audio.ParamDef, value, step float64) string {
	switch def.Name {
	case "decay":
		return formatParamValueStep(value, step, "×")
	case "drive", "body", "brightness":
		return formatParamValueStep(value*100, step*100, "%")
	case "pitch":
		// Generic post pitch ships unitless but is a semitone offset.
		return formatParamValueStep(value, step, "st")
	}
	return formatParamValueStep(value, step, def.Unit)
}
