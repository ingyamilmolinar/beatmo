package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Spectrum ISO band brackets follow the active locale (Bass/Mids/Treble →
// Graves/Medios/Agudos). Numeric ticks ("31", "1k") stay literal.
func TestSpectrumBandGroupLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	want := []string{"Graves", "Medios", "Agudos"}
	for i, w := range want {
		if got := bandGroupLabelAt(i); got != w {
			t.Errorf("bandGroupLabelAt(%d) ES = %q want %q", i, got, w)
		}
	}
	i18n.SetLocale(i18n.LocaleEN)
	wantEN := []string{"Bass", "Mids", "Treble"}
	for i, w := range wantEN {
		if got := bandGroupLabelAt(i); got != w {
			t.Errorf("bandGroupLabelAt(%d) EN = %q want %q", i, got, w)
		}
	}
}

func TestSamplerGroupLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[int]string{
		samplerKnobStart:     "RECORTE",   // TRIM group
		samplerKnobTranspose: "AFINACIÓN", // TUNE group
		samplerKnobGain:      "NIVEL",     // LEVEL group
	}
	for idx, w := range cases {
		if got := samplerGroupLabel(idx); got != w {
			t.Errorf("samplerGroupLabel(%d) ES = %q want %q", idx, got, w)
		}
	}
}

func TestSamplerKnobCaptionsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	s := &samplerState{}
	cases := map[int]string{
		samplerKnobStart:     "Inicio",
		samplerKnobEnd:       "Fin",
		samplerKnobTranspose: "Tono",
		samplerKnobDetune:    "Fino",
		samplerKnobGain:      "Ganancia",
	}
	for idx, prefix := range cases {
		got := samplerKnobCaption(idx, s)
		if !strings.HasPrefix(got, prefix+" ") {
			t.Errorf("samplerKnobCaption(%d) ES = %q want prefix %q", idx, got, prefix+" ")
		}
	}
}

func TestSamplerKnobGlossesLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := map[int]string{
		samplerKnobStart:     "dónde empieza",
		samplerKnobEnd:       "dónde termina",
		samplerKnobTranspose: "qué tan agudo o grave",
		samplerKnobDetune:    "leve ajuste de tono",
		samplerKnobGain:      "más fuerte o más suave",
	}
	for idx, w := range cases {
		if got := samplerKnobPlainEnglish(idx); got != w {
			t.Errorf("samplerKnobPlainEnglish(%d) ES = %q want %q", idx, got, w)
		}
	}
}

// Chain display-mode pills resolve their labels live through the i18n key.
func TestChainDisplayModeButtonsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	z := NewChainPanelZone(ChainCallbacks{})
	i18n.SetLocale(i18n.LocaleES)
	cases := map[string]*Button{
		"SUP": z.OverlayBtn(),
		"DIV": z.SplitBtn(),
		"DIF": z.DiffBtn(),
		"GA":  z.AGBtn(),
	}
	for want, b := range cases {
		if got := b.displayText(); got != want {
			t.Errorf("chain pill ES displayText = %q want %q", got, want)
		}
	}
	// English restores the original abbreviations.
	i18n.SetLocale(i18n.LocaleEN)
	if got := z.OverlayBtn().displayText(); got != "OVR" {
		t.Errorf("overlay EN displayText = %q want OVR", got)
	}
}

// Chain stage names route through i18n but keep their common DAW English form
// in every locale (per product decision).
func TestChainStageLabelsKeepEnglish(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	want := map[scope.Stage]string{
		scope.StageSynth:    "Synth",
		scope.StageAntiPop:  "AntiPop",
		scope.StageInsertFX: "FX",
		scope.StageEQ:       "EQ",
		scope.StageSends:    "Bus",
		scope.StageMaster:   "Master",
	}
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		for st, w := range want {
			if got := chainStageLabelLocalized(st); got != w {
				t.Errorf("chainStageLabelLocalized(%v) under %s = %q want %q", st, loc, got, w)
			}
		}
	}
}
