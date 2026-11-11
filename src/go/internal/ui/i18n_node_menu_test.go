package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestNodeMenuLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[i18n.Key]string{
		i18n.KeyNodeTitle:        "Nodo",
		i18n.KeyNodeSecVolume:    "Volumen",
		i18n.KeyNodeSecLogic:     "Lógica",
		i18n.KeyLogicEveryN:      "Disparar cada N",
		i18n.KeyLogicProbability: "Probabilidad",
		i18n.KeyGrooveDelay:      "Retraso",
		i18n.KeyAudMuted:         "Silenciado",
	}
	for k, want := range cases {
		if got := i18n.T(k); got != want {
			t.Fatalf("%q ES = %q want %q", k, got, want)
		}
	}
}
