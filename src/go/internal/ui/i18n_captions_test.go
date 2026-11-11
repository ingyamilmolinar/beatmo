package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestCaptionsAndNotificationsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[i18n.Key]string{
		i18n.KeyCapHeadroom:     "MARGEN",
		i18n.KeyCapYourSound:    "Tu sonido",
		i18n.KeyCapSearch:       "Buscar",
		i18n.KeyNotifImported:   "Proyecto JSON importado",
		i18n.KeyNotifInvalidBPM: "BPM inválido",
	}
	for k, want := range cases {
		if got := i18n.T(k); got != want {
			t.Fatalf("%q ES = %q want %q", k, got, want)
		}
	}
}
