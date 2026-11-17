package i18n

import "testing"

// synthStageLabelKeys / synthStageSubKeys are the pipeline stage label and
// kid-friendly subtitle keys added for the synth-tab i18n pass. They are listed
// here by their literal wire-format string (not the Key consts) so this test
// compiles and produces a clean assertion failure BEFORE the consts/catalog
// entries exist — and so the test independently pins the on-the-wire key names.
var synthStageLabelKeys = []Key{
	"synth.stage.voice", "synth.stage.osc", "synth.stage.fm",
	"synth.stage.pitch", "synth.stage.lfo", "synth.stage.burst",
	"synth.stage.envelope", "synth.stage.filter", "synth.stage.post",
	"synth.stage.tone",
}

var synthStageSubKeys = []Key{
	"synth.stage.voice.sub", "synth.stage.osc.sub", "synth.stage.fm.sub",
	"synth.stage.pitch.sub", "synth.stage.lfo.sub", "synth.stage.burst.sub",
	"synth.stage.envelope.sub", "synth.stage.filter.sub", "synth.stage.post.sub",
	"synth.stage.tone.sub",
}

func TestSynthStageKeysPresentBothLocales(t *testing.T) {
	all := append(append([]Key{}, synthStageLabelKeys...), synthStageSubKeys...)
	for _, k := range all {
		for _, loc := range []Locale{LocaleEN, LocaleES} {
			if v, ok := catalogFor(loc)[k]; !ok || v == "" {
				t.Errorf("locale %s missing/empty key %q", loc, k)
			}
		}
	}
}

// Every kid-friendly subtitle is full English prose, so every one must carry a
// distinct Spanish translation — never a verbatim copy of the English.
func TestSynthStageSubtitlesTranslatedToSpanish(t *testing.T) {
	defer SetLocale(LocaleEN)
	for _, k := range synthStageSubKeys {
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

// The translatable stage LABELS must also differ in Spanish. Universal audio
// abbreviations (OSC, FM, LFO) legitimately stay identical and are excluded.
func TestSynthStageLabelsTranslatedToSpanish(t *testing.T) {
	defer SetLocale(LocaleEN)
	translatable := []Key{
		"synth.stage.voice", "synth.stage.pitch", "synth.stage.burst",
		"synth.stage.envelope", "synth.stage.filter", "synth.stage.post",
		"synth.stage.tone",
	}
	for _, k := range translatable {
		SetLocale(LocaleEN)
		env := T(k)
		SetLocale(LocaleES)
		esv := T(k)
		if env == esv {
			t.Errorf("label %q expected Spanish translation, en==es==%q", k, env)
		}
	}
}
