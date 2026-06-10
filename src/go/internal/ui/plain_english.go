package ui

import "strings"

// PlainEnglish returns a kid-readable, one-line gloss for a UI label that
// otherwise carries an audio-engineering term (Attack, Cutoff, Q, etc.).
// Always-on; callers render the result as a subtitle below the jargon
// label at ~0.7× the body font size, the same way the Synth tab already
// renders section subtitles via sectionSubtitle().
//
// Returns "" when the label has no plain-English mapping — callers treat
// that as "no subtitle, skip the row". Lookup is case-insensitive and
// trims a trailing " (dB)" / " (Hz)" / " %" suffix before matching so
// renderers can pass display labels verbatim without stripping units.
//
// Mappings stay narrow and audio-pedagogy focused. Anything that turns
// into a sentence longer than ~24 characters belongs in the per-tab
// legend chip (see audio_panel_legend.go), not here.
func PlainEnglish(label string) string {
	key := plainEnglishKey(label)
	if v, ok := plainEnglishMap[key]; ok {
		return v
	}
	return ""
}

// plainEnglishKey normalises a label for case- and units-insensitive
// lookup. Strips leading/trailing whitespace, lowercases, and removes a
// trailing " (dB)" / " (Hz)" / " %" suffix.
func plainEnglishKey(label string) string {
	s := strings.TrimSpace(strings.ToLower(label))
	for _, suf := range []string{" (db)", " (hz)", " (khz)", " (s)", " (ms)", " %", " hz", " db", " khz", " ms"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSpace(s[:len(s)-len(suf)])
		}
	}
	return s
}

// plainEnglishMap stores the canonical kid-friendly gloss for each
// supported jargon term. Keep entries short (≤ 24 chars), lowercase the
// keys; the lookup normalises input case + units suffix.
//
// Section-card subtitles already live in sectionSubtitle() (synth_panel
// _zone.go) — those are duplicated here so the same helper can serve
// any callsite that wants to apply the gloss to an individual knob
// label without going through the section-card path.
var plainEnglishMap = map[string]string{
	// Synth pipeline sections (Phase 8B unified Synth tab).
	"voice": "what makes this sound",
	"osc":   "the raw waveform",

	// Pitch
	"pitch":     "how high or low",
	"waveform":  "the raw wave shape",
	"generator": "the raw wave shape",
	"tune":      "how high or low",
	"glide":     "slide between notes",
	"bend":      "pitch wobble",
	"detune":    "slight pitch shift",

	// Envelope
	"envelope": "how it starts and dies",
	"attack":   "how quickly it starts",
	"decay":    "how fast it fades",
	"sustain":  "how loud it holds",
	"release":  "how slowly it ends",

	// Tone / filter
	"tone":      "bright or dull",
	"cutoff":    "bright or dull",
	"filter":    "bright or dull",
	"reso":      "ringing edge",
	"resonance": "ringing edge",
	"body":      "punch vs air",

	// FM family knobs (native-deprecation migration). Op-numbered labels
	// normalise through plainEnglishKey verbatim (no unit suffix), so the
	// keys carry the full lowercase label.
	"base pitch":    "the root note",
	"pitch sweep":   "drop or rise at start",
	"sweep time":    "how fast the sweep",
	"op 1 ratio":    "harmonic of the root",
	"op 2 ratio":    "harmonic of the root",
	"op 3 ratio":    "harmonic of the root",
	"op 4 ratio":    "harmonic of the root",
	"op 1 fm depth": "metallic shimmer",
	"op 2 fm depth": "metallic shimmer",
	"op 3 fm depth": "metallic shimmer",
	"op 4 fm depth": "metallic shimmer",
	"op 1 decay":    "how fast it fades",
	"op 2 decay":    "how fast it fades",
	"op 3 decay":    "how fast it fades",
	"op 4 decay":    "how fast it fades",

	// Kick family knobs (native-deprecation migration).
	"fundamental":  "the root note",
	"sweep speed":  "how fast the sweep",
	"boom decay":   "how fast the boom fades",
	"body decay":   "how fast the body fades",
	"2nd harmonic": "adds body",
	"3rd harmonic": "adds presence",
	"4th harmonic": "adds bite",
	"click":        "beater snap",
	"thud":         "soft air punch",

	// Tom family knobs.
	"ring decay": "how long it rings",
	"overtone 1": "adds warmth",
	"overtone 2": "adds edge",
	"stick":      "stick snap",
	"room":       "room echo feel",

	// Snare family knobs.
	"tone 2":      "second ring note",
	"noise tune":  "noise high or low",
	"tone decay":  "how fast the note fades",
	"noise decay": "how fast the hiss fades",
	"tail decay":  "how long the tail",
	"tone level":  "more note vs noise",
	"noise level": "more noise vs note",
	"wires":       "snare wire buzz",
	"snap":        "sharper hit",

	// Cymbal family knobs.
	"metal tune":   "metal high or low",
	"attack decay": "how fast the hit fades",
	"metal level":  "more metal ring",
	"sizzle level": "more sizzle hiss",
	"sizzle decay": "how fast the sizzle fades",

	// Bass family knobs.
	"pluck":       "soft or hard pluck",
	"pick":        "pick click",
	"fade":        "how fast it fades",
	"overtone":    "adds presence",
	"pitch punch": "punchy start",
	"punch":       "punchy start",

	// Drive / dynamics
	"drive":      "softness vs edge",
	"gain":       "louder or quieter",
	"saturation": "warm edge",
	"sat":        "warm edge",
	"distortion": "rough edge",

	// Spectrum / metering
	"slope":    "tilt for tone balance",
	"slopes":   "tilt for tone balance",
	"pre":      "before EQ",
	"post":     "after EQ",
	"peak":     "loudest moment",
	"rms":      "average loudness",
	"clip":     "too loud — clipping",
	"clips":    "too loud — clipping",
	"headroom": "room before clipping",
	"lufs":     "broadcast loudness",
	"k-20":     "ref scale (-20 = 0)",

	// Mode / chain selectors
	"overlay":   "show A and B together",
	"split":     "show A on top, B on bottom",
	"diff":      "show A minus B",
	"ovr":       "show A and B together",
	"spl":       "show A on top, B on bottom",
	"dif":       "show A minus B",
	"ag":        "auto adjust scale",
	"auto gain": "auto adjust scale",

	// Sends / outs
	"out":    "what reaches the master",
	"delay":  "echo trail",
	"reverb": "room space",
	"send":   "amount to effect",

	// Transport
	"play":  "start the beat",
	"stop":  "stop the beat",
	"pause": "freeze the beat",
}
