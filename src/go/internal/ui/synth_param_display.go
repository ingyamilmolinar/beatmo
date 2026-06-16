package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

// formatParamValue is the single value formatter shared by every synth-tab knob
// caption AND the Sampler knob captions, so values read coherently across the
// whole panel. Rules by unit:
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
	switch unit {
	case "Hz":
		if math.Abs(value) >= 1000 {
			return fmt.Sprintf("%.1f kHz", value/1000)
		}
		return fmt.Sprintf("%d Hz", int(math.Round(value)))
	case "s":
		return fmt.Sprintf("%.2f s", value)
	case "ms":
		return fmt.Sprintf("%d ms", int(math.Round(value)))
	case "cents", "c":
		return fmt.Sprintf("%+d c", int(math.Round(value)))
	case "st":
		return fmt.Sprintf("%+d st", int(math.Round(value)))
	case "dB":
		return fmt.Sprintf("%+d dB", int(math.Round(value)))
	case "×", "x", "mult":
		return fmt.Sprintf("%.1f×", value)
	case "%":
		return fmt.Sprintf("%d%%", int(math.Round(value)))
	case "":
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.2f %s", value, unit)
	}
}

// formatSynthParamValue formats a ParamDef's current value for a caption,
// special-casing the generic post knobs whose semantic unit isn't carried in
// ParamDef.Unit (they ship as unitless floats but read best as a multiplier or
// percentage). Everything else routes through formatParamValue by Unit.
func formatSynthParamValue(def audio.ParamDef, value float64) string {
	switch def.Name {
	case "decay":
		return formatParamValue(value, "×")
	case "drive", "body", "brightness":
		return formatParamValue(value*100, "%")
	case "pitch":
		// Generic post pitch ships unitless but is a semitone offset.
		return formatParamValue(value, "st")
	}
	return formatParamValue(value, def.Unit)
}
