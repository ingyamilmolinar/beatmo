package i18n

import "testing"

// Round-2 fixes: Fade wording, translatable units, sampler meta readout,
// levels long-press tooltips. Keys listed by literal string for clean RED.
var round2TranslatableWant = map[Key]struct{ en, es string }{
	"unit.cents":               {"c", "ct"},
	"sampler.meta_fmt":         {"%.2fs of %.2fs · %.1f kHz", "%.2fs de %.2fs · %.1f kHz"},
	"levels.tip.headroom":      {"Headroom: %s dB", "Margen: %s dB"},
	"levels.tip.headroom_clip": {"Headroom: 0 dB (clipping)", "Margen: 0 dB (saturando)"},
	"levels.tip.clips":         {"Clips (10s): %s", "Recortes (10s): %s"},
	"levels.tip.loudest":       {"Loudest: %s", "Más fuerte: %s"},
	"levels.tip.loudest_none":  {"Loudest: —", "Más fuerte: —"},
}

func TestRound2KeysTranslated(t *testing.T) {
	defer SetLocale(LocaleEN)
	for k, want := range round2TranslatableWant {
		SetLocale(LocaleEN)
		if got := T(k); got != want.en {
			t.Errorf("%q EN = %q want %q", k, got, want.en)
		}
		SetLocale(LocaleES)
		if got := T(k); got != want.es {
			t.Errorf("%q ES = %q want %q", k, got, want.es)
		}
	}
}

// Fade: the user rejected "Fundido"; the chosen term is "Atenuar".
func TestFadeIsAtenuar(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	if got := T(KeyFade); got != "Atenuar" {
		t.Errorf("KeyFade ES = %q want %q", got, "Atenuar")
	}
	SetLocale(LocaleEN)
	if got := T(KeyFade); got != "Fade" {
		t.Errorf("KeyFade EN = %q want %q", got, "Fade")
	}
}
