package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestActionButtonLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[i18n.Key]string{
		i18n.KeySave:    "Guardar",
		i18n.KeySaveAs:  "Guardar como",
		i18n.KeyReset:   "Restablecer",
		i18n.KeyPreview: "Escuchar",
		i18n.KeyCancel:  "Cancelar",
	}
	for k, want := range cases {
		if got := i18n.T(k); got != want {
			t.Fatalf("%q ES = %q want %q", k, got, want)
		}
	}
}
