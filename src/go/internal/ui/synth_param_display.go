package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Presentation only: a param id + value + unit to display strings, so no caption leaks a snake_case id or a
// bare unitless number. Never touches rendering or the ABI.

// Curated names where the prettifier would guess; "Decay ×" (post multiplier) and "Decay" (envelope seconds)
// differ so one pane never shows two "Decay" captions.
var synthParamNameOverrides = map[string]string{
	// ---- WAV-clone stages (MODAL / STRIKE / NOISE) ----
	"modal_tune": "Tune", "modal_stretch": "Stretch", "modal_decay_x": "Damping", "modal_bright": "Brightness",
	"modal_keep": "Partials", "modal_law_a": "Decay Rate", "modal_law_p": "Decay Tilt", "modal_law_b": "Top Damping",
	"strike_tc": "Contact", "strike_hardness": "Hardness", "strike_click": "Click", "noiseband_level": "Noise Level",

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

	// Family knobs whose prefix-stripped form is terse or ambiguous.
	"kick_click": "Click",
	"kick_noise": "Noise",
	"kick_wave":  "Wave",
	"snare_wave": "Wave",
	"cym_wave":   "Wave",
	"tom_wave":   "Wave",
	"bass_wave":  "Wave",
}

// The override table stays the canonical English (the English catalog mirrors it). FM operators and burst
// hits are composed in composeOpLabel so the "OpN"/"Hit N" prefix stays English.
var synthParamNameKeys = map[string]i18n.Key{
	// ---- generic post knobs ----
	"pitch":      i18n.KeySynthControlTune,
	"decay":      i18n.KeySynthControlDecayMult,
	"tone":       i18n.KeySynthControlTone,
	"drive":      i18n.KeySynthControlDrive,
	"body":       i18n.KeySynthControlBody,
	"brightness": i18n.KeySynthControlBright,
	"gain":       i18n.KeySynthControlGain,

	// ---- FINISH stage (post ops + output limit) ----
	"post_pitch":      i18n.KeySynthControlResample,
	"post_decay":      i18n.KeySynthControlLength,
	"post_decay_rate": i18n.KeySynthControlFadeSpeed,
	"post_body":       i18n.KeySynthControlLowBoost,
	"post_brightness": i18n.KeySynthControlCrisp,
	"post_darken":     i18n.KeySynthControlDarken,
	"post_drive":      i18n.KeySynthControlCrunch,
	"out_clamp":       i18n.KeySynthControlLimit,

	// ---- OSC stage ----
	"osc_type":   i18n.KeySynthControlWave,
	"osc_detune": i18n.KeySynthControlDetune,
	"osc_octave": i18n.KeySynthControlOctave,

	// ---- amp envelope (ADSR) ----
	"amp_attack":  i18n.KeySynthControlAttack,
	"amp_decay":   i18n.KeySynthControlDecay,
	"amp_sustain": i18n.KeySynthControlSustain,
	"amp_release": i18n.KeySynthControlRelease,
	"amp_curve":   i18n.KeySynthControlCurve,

	// ---- filter stage ----
	"filter_type":      i18n.KeySynthControlType,
	"filter_cutoff":    i18n.KeySynthControlCutoff,
	"filter_resonance": i18n.KeySynthControlResonance,

	// ---- FM stage singletons (the OpN families are composed) ----
	"fm_algorithm":        i18n.KeySynthControlAlgorithm,
	"fm_wave":             i18n.KeySynthControlWave,
	"fm_base_freq":        i18n.KeySynthControlBaseFreq,
	"fm_pitch_env_amount": i18n.KeySynthControlPitchEnv,
	"fm_pitch_env_decay":  i18n.KeySynthControlPitchEnvDecay,

	// ---- PITCH-env modulator stage ----
	"pitchenv_amt":   i18n.KeySynthControlAmount,
	"pitchenv_decay": i18n.KeySynthControlDecay,

	// ---- LFO modulator stage ----
	"lfo_rate":  i18n.KeySynthControlRate,
	"lfo_depth": i18n.KeySynthControlDepth,

	// ---- BURST modulator stage (the HitN families are composed) ----
	"burst_sharp": i18n.KeySynthControlSharpness,

	// ---- noise generator ----
	"noise_draws":   i18n.KeySynthControlDraws,
	"noise_prelude": i18n.KeySynthControlPrelude,
	"noise_seed":    i18n.KeySynthControlSeed,

	// ---- voice fundamentals ----
	"fundamental":   i18n.KeySynthControlPitch,
	"voice_freq_hz": i18n.KeySynthControlPitch,

	// ---- drum-family well-known knobs ----
	"kick_click": i18n.KeySynthControlClick,
	"kick_noise": i18n.KeySynthControlNoise,
	"kick_wave":  i18n.KeySynthControlWave,
	"snare_wave": i18n.KeySynthControlWave,
	"cym_wave":   i18n.KeySynthControlWave,
	"tom_wave":   i18n.KeySynthControlWave,
	"bass_wave":  i18n.KeySynthControlWave,
}

// Display only: the canonical strings stay the storage form. Universal members ("FM", "2-op") pass through.
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

func localizeEnumLabel(canonical string) string {
	if k, ok := synthEnumLabelKeys[canonical]; ok {
		return i18n.T(k)
	}
	return canonical
}

// The "OpN"/"Hit N" prefix is universal jargon and stays English; only the noun follows the locale.
func composeOpLabel(name string) (string, bool) {
	if n, noun, ok := splitIndexedFamily(name, "fm_op"); ok {
		var k i18n.Key
		switch noun {
		case "ratio":
			k = i18n.KeySynthNounRatio
		case "level":
			k = i18n.KeySynthNounLevel
		case "depth":
			k = i18n.KeySynthControlDepth
		case "decay":
			k = i18n.KeySynthControlDecay
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

// false for the stage's singletons ("burst_sharp"), whose first post-prefix character is not a digit.
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

var synthFamilyPrefixes = []string{
	"osc_", "amp_", "fm_", "filter_", "lfo_", "pitchenv_", "burst_",
	"noise_", "post_", "kick_", "snare_", "cym_", "tom_", "bass_", "voice_",
}

// Always non-empty and never the raw id.
func synthParamDisplayName(name string) string {
	if name == "" {
		return ""
	}
	// Order: localized labels, composed families, the English overrides, then the prettifier.
	if k, ok := synthParamNameKeys[name]; ok {
		return i18n.T(k)
	}
	// An authored name beats any inference from the id (gen-bank knobs prettify to "Fm Dec2"); below the i18n
	// keys so a translation still wins.
	if lbl, ok := audio.StageKnobLabelForParam(name); ok {
		return lbl
	}
	if lbl, ok := composeOpLabel(name); ok {
		return lbl
	}
	if disp, ok := synthParamNameOverrides[name]; ok {
		return disp
	}
	return prettifySynthParamID(name)
}

// Non-empty, and different from the raw id whenever it has an underscore or a lowercase first letter.
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

// No regexp on a hot-path string op.
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

// The rest is kept as-is so an all-caps acronym survives.
func titleWord(w string) string {
	if w == "" {
		return ""
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

// Enough digits that values one step apart render differently (0.005 → 3); 0 for a non-positive step,
// capped at 6.
func stepDecimals(step float64) int {
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return 0
	}
	// The epsilon keeps an exact decade (0.1) at 1 digit, not 2.
	d := int(math.Ceil(-math.Log10(step) - 1e-9))
	if d < 0 {
		d = 0
	}
	if d > 6 {
		d = 6
	}
	return d
}

// The one formatter for every Synth and Sampler caption, so values read alike across the panel. Hz is
// SI-prefixed above 1000; cents/st/dB signed integers; s two decimals; unknown units two decimals + suffix.
func formatParamValue(value float64, unit string) string {
	return formatParamValueStep(value, math.Inf(1), unit)
}

// Never fewer digits than the unit's natural precision; step is in the RENDERED units, so a caller scaling
// the value (×100, ÷1000) must scale the step too.
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
		// "cents" has a sensible Spanish form ("ct"); the other units are international.
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

// The generic post knobs ship unitless but read as a multiplier or a percentage.
func formatSynthParamValue(def audio.ParamDef, value float64) string {
	return formatSynthParamValueStep(def, value, math.Inf(1))
}

// The special cases scale the step like the value, so the precision tracks what is displayed.
func formatSynthParamValueStep(def audio.ParamDef, value, step float64) string {
	switch def.Name {
	case "decay":
		return formatParamValueStep(value, step, "×")
	case "drive", "body", "brightness":
		return formatParamValueStep(value*100, step*100, "%")
	case "pitch":
		// Generic post pitch ships unitless but is a semitone offset.
		return formatParamValueStep(value, step, "st")
	case "modal_keep":
		// A count of mode rows ("Partials").
		return strconv.Itoa(int(math.Round(value)))
	}
	return formatParamValueStep(value, step, def.Unit)
}
