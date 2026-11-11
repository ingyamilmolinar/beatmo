package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestButtonKeyResolvesLiveOnLocaleSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	b := NewButtonKey(i18n.KeyTransportPlay, ComponentButtonSecondary, func() {})
	i18n.SetLocale(i18n.LocaleEN)
	if got := b.displayText(); got != "Play" {
		t.Fatalf("EN label=%q want Play", got)
	}
	i18n.SetLocale(i18n.LocaleES)
	if got := b.displayText(); got != "Tocar" {
		t.Fatalf("ES label=%q want Tocar", got)
	}
}

func TestButtonStaticTextUnaffected(t *testing.T) {
	b := NewButton("Static", nil, func() {})
	if got := b.displayText(); got != "Static" {
		t.Fatalf("static displayText=%q want Static", got)
	}
}
