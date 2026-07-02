package i18n

import "testing"

// synth_knob_test.go pins the Synth-tab KNOB-LABEL and value-ENUM i18n keys
// (the gap that left "Triangle", "Pitch", "Cutoff", … hard-coded English on the
// Synth tab). Keys are listed by their literal wire-format string — not the Key
// consts — so this test compiles and fails cleanly BEFORE the consts/catalog
// entries exist, and independently pins the on-the-wire key names.

// synthKnobLabelKeys are the curated knob-label keys (one per distinct caption
// value; several param ids share a key, e.g. every "Wave" knob).
var synthKnobLabelKeys = []Key{
	"synth.knob.tune", "synth.knob.decay-mult", "synth.knob.tone",
	"synth.knob.drive", "synth.knob.body", "synth.knob.bright", "synth.knob.gain",
	"synth.knob.wave", "synth.knob.detune", "synth.knob.octave",
	"synth.knob.attack", "synth.knob.decay", "synth.knob.sustain",
	"synth.knob.release", "synth.knob.curve",
	"synth.knob.type", "synth.knob.cutoff", "synth.knob.resonance",
	"synth.knob.algorithm", "synth.knob.base-freq",
	"synth.knob.pitch-env", "synth.knob.pitch-env-decay",
	"synth.knob.amount", "synth.knob.rate", "synth.knob.depth",
	"synth.knob.sharpness", "synth.knob.draws", "synth.knob.prelude",
	"synth.knob.seed", "synth.knob.pitch", "synth.knob.click", "synth.knob.noise",
}

// synthKnobNounKeys back the composed FM-operator / burst-hit captions
// ("Op1 Ratio", "Hit 1 Level") — the universal "OpN"/"Hit N" prefix stays
// English; only the noun is keyed.
var synthKnobNounKeys = []Key{
	"synth.noun.ratio", "synth.noun.level", "synth.noun.time", "synth.noun.hit",
}

// synthEnumKeys are the value-enum keys (the knob VALUE that reads "Triangle",
// "Low-pass", "Exponential", …). "FM" and "2-op" are universal and intentionally
// keyless (they pass through unchanged), so they are not listed here.
var synthEnumKeys = []Key{
	"synth.enum.wave-sine", "synth.enum.wave-saw", "synth.enum.wave-square",
	"synth.enum.wave-triangle", "synth.enum.wave-noise-white", "synth.enum.wave-noise-pink",
	"synth.enum.algo-parallel", "synth.enum.algo-3op-chain", "synth.enum.algo-4op-stack",
	"synth.enum.curve-linear", "synth.enum.curve-exponential",
	"synth.enum.filter-lowpass", "synth.enum.filter-highpass", "synth.enum.filter-bandpass",
	"synth.enum.on", "synth.enum.off",
}

func TestSynthKnobKeysPresentBothLocales(t *testing.T) {
	var all []Key
	all = append(all, synthKnobLabelKeys...)
	all = append(all, synthKnobNounKeys...)
	all = append(all, synthEnumKeys...)
	for _, k := range all {
		for _, loc := range []Locale{LocaleEN, LocaleES} {
			if v, ok := catalogFor(loc)[k]; !ok || v == "" {
				t.Errorf("locale %s missing/empty key %q", loc, k)
			}
		}
	}
}

// Every curated knob label and noun has a genuine Spanish form (no universal
// abbreviation survives in this set), so each must differ from its English.
func TestSynthKnobLabelsTranslatedToSpanish(t *testing.T) {
	defer SetLocale(LocaleEN)
	var translatable []Key
	translatable = append(translatable, synthKnobLabelKeys...)
	translatable = append(translatable, synthKnobNounKeys...)
	for _, k := range translatable {
		SetLocale(LocaleEN)
		env := T(k)
		SetLocale(LocaleES)
		esv := T(k)
		if env == "" || esv == "" {
			t.Errorf("key %q empty (en=%q es=%q)", k, env, esv)
			continue
		}
		if env == esv {
			t.Errorf("key %q not translated to Spanish: en==es==%q", k, env)
		}
	}
}

// Every keyed enum value differs in Spanish (universal "FM"/"2-op" are keyless
// and therefore excluded by construction).
func TestSynthEnumsTranslatedToSpanish(t *testing.T) {
	defer SetLocale(LocaleEN)
	for _, k := range synthEnumKeys {
		SetLocale(LocaleEN)
		env := T(k)
		SetLocale(LocaleES)
		esv := T(k)
		if env == "" || esv == "" {
			t.Errorf("enum key %q empty (en=%q es=%q)", k, env, esv)
			continue
		}
		if env == esv {
			t.Errorf("enum key %q not translated to Spanish: en==es==%q", k, env)
		}
	}
}
