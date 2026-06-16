package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestTabLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[PanelTab]string{
		TabWave:     "Onda",
		TabSpectrum: "Espectro",
		TabMeters:   "Niveles",
		TabScope:    "Cadena",
	}
	for tab, want := range cases {
		if got := PanelTabLabel(tab); got != want {
			t.Fatalf("PanelTabLabel(%v) ES = %q want %q", tab, got, want)
		}
	}
	if got := PanelTabLabel(TabEQ); got != "EQ" {
		t.Fatalf("EQ tab should stay EQ, got %q", got)
	}
}
