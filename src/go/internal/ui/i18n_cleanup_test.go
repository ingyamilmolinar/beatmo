package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestCleanupKeysLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[i18n.Key]string{
		i18n.KeyNoNotifications: "Aún no hay notificaciones",
		i18n.KeyNodeLogic:       "Lógica:",
		i18n.KeyNodeGroove:      "Ritmo:",
	}
	for k, want := range cases {
		if got := i18n.T(k); got != want {
			t.Fatalf("%q ES = %q want %q", k, got, want)
		}
	}
}
