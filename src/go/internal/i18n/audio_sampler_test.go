package i18n

import "testing"

// Keys added for the audio-panel + Sampler i18n pass. Listed by literal wire
// string so this test compiles (clean RED) before the Key consts exist and so
// it independently pins the on-the-wire key names.

// Translatable keys: ES MUST differ from EN (real Spanish DAW term).
var audioTranslatableWant = map[Key]struct{ en, es string }{
	// Spectrum band brackets.
	"spectrum.band.bass":   {"Bass", "Graves"},
	"spectrum.band.mids":   {"Mids", "Medios"},
	"spectrum.band.treble": {"Treble", "Agudos"},
	// Levels.
	"levels.peak":  {"PEAK", "PICO"},
	"levels.clips": {"CLIPS", "RECORTES"},
	"levels.clear": {"CLR", "BORR"},
	// Chain display modes (OVR/SPL/AG/FIT translate; DIF is identical both).
	"chain.overlay":   {"OVR", "SUP"},
	"chain.split":     {"SPL", "DIV"},
	"chain.auto_gain": {"AG", "GA"},
	"chain.fit":       {"FIT", "AJU"},
	// Sampler knob group labels.
	"sampler.group.trim":  {"TRIM", "RECORTE"},
	"sampler.group.tune":  {"TUNE", "AFINACIÓN"},
	"sampler.group.level": {"LEVEL", "NIVEL"},
	// Sampler knob caption prefixes.
	"sampler.knob.start": {"Start", "Inicio"},
	"sampler.knob.end":   {"End", "Fin"},
	"sampler.knob.pitch": {"Pitch", "Tono"},
	"sampler.knob.fine":  {"Fine", "Fino"},
	"sampler.knob.gain":  {"Gain", "Ganancia"},
	// Sampler knob glosses.
	"sampler.gloss.start": {"where it begins", "dónde empieza"},
	"sampler.gloss.end":   {"where it ends", "dónde termina"},
	"sampler.gloss.pitch": {"how high or low", "qué tan agudo o grave"},
	"sampler.gloss.fine":  {"tiny pitch nudge", "leve ajuste de tono"},
	"sampler.gloss.gain":  {"louder or quieter", "más fuerte o más suave"},
}

// Routed-but-kept keys: present in both locales, value stays the common DAW
// English term (ES may equal EN).
var audioKeptWant = map[Key]string{
	"chain.diff":          "DIF",
	"chain.stage.synth":   "Synth",
	"chain.stage.antipop": "AntiPop",
	"chain.stage.fx":      "FX",
	"chain.stage.eq":      "EQ",
	"chain.stage.bus":     "Bus",
}

func TestAudioSamplerKeysTranslated(t *testing.T) {
	defer SetLocale(LocaleEN)
	for k, want := range audioTranslatableWant {
		SetLocale(LocaleEN)
		if got := T(k); got != want.en {
			t.Errorf("%q EN = %q want %q", k, got, want.en)
		}
		SetLocale(LocaleES)
		if got := T(k); got != want.es {
			t.Errorf("%q ES = %q want %q", k, got, want.es)
		}
		if want.en == want.es {
			t.Errorf("test invariant: %q EN==ES, should be in audioKeptWant", k)
		}
	}
}

func TestAudioSamplerKeptKeysPresent(t *testing.T) {
	defer SetLocale(LocaleEN)
	for k, want := range audioKeptWant {
		for _, loc := range []Locale{LocaleEN, LocaleES} {
			SetLocale(loc)
			if got := T(k); got != want {
				t.Errorf("%q under %s = %q want %q (common DAW term, kept English)", k, loc, got, want)
			}
		}
	}
}

func TestLevelsReadoutFormatLocalized(t *testing.T) {
	defer SetLocale(LocaleEN)
	k := Key("levels.readout_fmt")
	// 3-%s prefix; the clip COUNT is drawn separately (colored) after it.
	SetLocale(LocaleEN)
	if got, want := T(k), "Pk %s · RMS %s · Hdr %s · Clips "; got != want {
		t.Errorf("EN readout fmt = %q want %q", got, want)
	}
	SetLocale(LocaleES)
	if got, want := T(k), "Pico %s · RMS %s · Margen %s · Recortes "; got != want {
		t.Errorf("ES readout fmt = %q want %q", got, want)
	}
}

// Audit: existing values corrected to common Spanish DAW terms.
func TestExistingTranslationsAuditFixes(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	// "pitch" is agudo/grave, never alto/bajo (which reads as volume).
	if got := T(KeyGlossPitch); got != "qué tan agudo o grave" {
		t.Errorf("KeyGlossPitch ES = %q want %q", got, "qué tan agudo o grave")
	}
	if got := T(KeyGlossTune); got != "qué tan agudo o grave" {
		t.Errorf("KeyGlossTune ES = %q want %q", got, "qué tan agudo o grave")
	}
	// Reverse abbreviates to "Inv" (Invertir); "Rev" misreads as reverb.
	if got := T(KeyReverse); got != "Inv" {
		t.Errorf("KeyReverse ES = %q want %q", got, "Inv")
	}
}
