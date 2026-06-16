//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestLegendLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	// EN: legend resolves to the English sentence.
	i18n.SetLocale(i18n.LocaleEN)
	en := i18n.T(audioPanelLegendText[TabWave])
	if !strings.Contains(en, "Wave") {
		t.Fatalf("EN wave legend unexpected: %q", en)
	}
	// ES: resolves to a different (Spanish) string.
	i18n.SetLocale(i18n.LocaleES)
	es := i18n.T(audioPanelLegendText[TabWave])
	if es == en || es == "" {
		t.Fatalf("ES wave legend not localized: %q", es)
	}
}

func TestPlainEnglishLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	en := PlainEnglish("voice")
	i18n.SetLocale(i18n.LocaleES)
	es := PlainEnglish("voice")
	if en == "" || es == "" || en == es {
		t.Fatalf("PlainEnglish(voice) not localized: en=%q es=%q", en, es)
	}
}
