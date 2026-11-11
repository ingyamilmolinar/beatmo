package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The synth-tab stage subtitles ("what makes this sound", …) must follow the
// active locale. Under es-419 every stage shows its Spanish gloss.
func TestSynthSectionSubtitleLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[synthSectionID]string{
		synthSectionVoice:    "lo que define este sonido",
		synthSectionPitch:    "qué tan agudo o grave",
		synthSectionEnvelope: "cómo empieza y se apaga",
		synthSectionFilter:   "esculpe el timbre",
		synthSectionTone:     "brillante u opaco",
	}
	for id, want := range cases {
		if got := sectionSubtitle(id); got != want {
			t.Errorf("sectionSubtitle(%v) ES = %q want %q", id, got, want)
		}
	}
}

// English (the default locale) is unchanged — the existing prose still renders.
func TestSynthSectionSubtitleEnglishUnchanged(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	if got := sectionSubtitle(synthSectionVoice); got != "what makes this sound" {
		t.Errorf("sectionSubtitle(VOICE) EN = %q want %q", got, "what makes this sound")
	}
}

// The displayed stage chip labels follow the active locale. Universal audio
// abbreviations (OSC, FM, LFO) intentionally stay identical in Spanish.
func TestSynthSectionLabelLocalizedDisplay(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[synthSectionID]string{
		synthSectionVoice:    "VOZ",
		synthSectionEnvelope: "ENVOLVENTE",
		synthSectionFilter:   "FILTRO",
		synthSectionBurst:    "RÁFAGA",
		synthSectionPost:     "FINAL",
		synthSectionTone:     "TIMBRE",
		synthSectionPitch:    "TONO",
		synthSectionOsc:      "OSC",
		synthSectionFM:       "FM",
		synthSectionLFO:      "LFO",
	}
	for id, want := range cases {
		if got := sectionLabelLocalized(id); got != want {
			t.Errorf("sectionLabelLocalized(%v) ES = %q want %q", id, got, want)
		}
	}
}

// sectionLabel stays the stable canonical English identifier under ANY locale.
// It backs hit-area tags ("synth-chip-VOICE") and the selectSynthSection JS
// lookup, which must not shift when the user changes language.
func TestSynthSectionLabelCanonicalAcrossLocales(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	want := map[synthSectionID]string{
		synthSectionVoice:    "VOICE",
		synthSectionEnvelope: "ENVELOPE",
		synthSectionPost:     "POST",
		synthSectionTone:     "TONE",
	}
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		for id, w := range want {
			if got := sectionLabel(id); got != w {
				t.Errorf("sectionLabel(%v) under %s = %q, want stable %q", id, loc, got, w)
			}
		}
	}
}

// In the default English locale, the localized label is byte-identical to the
// canonical label — proving the i18n EN catalog mirrors the canonical strings,
// so existing English-locale tests and layouts see no behavior change.
func TestSynthSectionLabelLocalizedMatchesCanonicalInEnglish(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	for _, id := range unifiedSynthSectionOrder {
		if got, canon := sectionLabelLocalized(id), sectionLabel(id); got != canon {
			t.Errorf("EN sectionLabelLocalized(%v)=%q != canonical %q", id, got, canon)
		}
	}
}
