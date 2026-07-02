//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The cents value unit reads "ct" (centésimas) in Spanish, "c" in English.
// Symbol units (st/dB/Hz/%/×/ms/s) are international and stay identical.
func TestCentsUnitLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	if got := formatParamValue(5, "cents"); got != "+5 ct" {
		t.Errorf("formatParamValue(5,cents) ES = %q want %q", got, "+5 ct")
	}
	if got := formatParamValue(3, "st"); got != "+3 st" {
		t.Errorf("st must stay international, ES = %q want %q", got, "+3 st")
	}
	i18n.SetLocale(i18n.LocaleEN)
	if got := formatParamValue(5, "cents"); got != "+5 c" {
		t.Errorf("formatParamValue(5,cents) EN = %q want %q", got, "+5 c")
	}
}

// Sampler length/sample-rate readout: the "of" joiner follows the locale.
func TestSamplerMetaReadoutLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	if got := samplerMetaText(0.50, 1.20, 44.1); got != "0.50s de 1.20s · 44.1 kHz" {
		t.Errorf("samplerMetaText ES = %q", got)
	}
	i18n.SetLocale(i18n.LocaleEN)
	if got := samplerMetaText(0.50, 1.20, 44.1); got != "0.50s of 1.20s · 44.1 kHz" {
		t.Errorf("samplerMetaText EN = %q", got)
	}
}

// Levels long-press tooltips follow the locale (Margen / Recortes / Más fuerte).
func TestLevelsTooltipsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	cases := []struct {
		slot string
		agg  LevelsAggregate
		want string
	}{
		{"headroom", LevelsAggregate{HeadroomDB: 6, HeadroomTxt: "6"}, "Margen: 6 dB"},
		{"headroom", LevelsAggregate{HeadroomDB: 0}, "Margen: 0 dB (saturando)"},
		{"clips", LevelsAggregate{ClipsTxt: "2"}, "Recortes (10s): 2"},
		{"loudest", LevelsAggregate{LoudestName: "Kick"}, "Más fuerte: Kick"},
		{"loudest", LevelsAggregate{LoudestName: ""}, "Más fuerte: —"},
	}
	for _, c := range cases {
		if got := levelsAggregateTooltipText(c.slot, c.agg); got != c.want {
			t.Errorf("tooltip(%s) ES = %q want %q", c.slot, got, c.want)
		}
	}
}

// fitScaleToWidth keeps the base scale when text fits, and shrinks it just
// enough to fit when it doesn't — so longer Spanish strings never overflow.
func TestFitScaleToWidth(t *testing.T) {
	if got := fitScaleToWidth(50, 1.0, 100); got != 1.0 {
		t.Errorf("fits: got %v want 1.0", got)
	}
	got := fitScaleToWidth(200, 1.0, 100)
	if int(float64(200)*got) > 100 {
		t.Errorf("overflow not contained: 200*%v > 100", got)
	}
	if got >= 1.0 {
		t.Errorf("overflowing text should shrink, got %v", got)
	}
}

// Every sampler knob caption renders at FULL font and fits inside its cell —
// the layout widens cells (and wraps to two rows) instead of shrinking text,
// so "Ganancia +0 dB" stays readable and never overlaps a neighbour.
func TestSamplerCaptionsFitFullFontDesktop(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	i18n.SetLocale(i18n.LocaleES)
	g.drum.sampler.captureFromSynth("kick")
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), "kick")
	assertSamplerCaptionsFit(t, g.drum)
}

func TestSamplerCaptionsFitFullFontMobile(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)
	if !g.drum.sampler.hasBuffer() {
		t.Skip("no sampler buffer captured")
	}
	assertSamplerCaptionsFit(t, g.drum)
}

func assertSamplerCaptionsFit(t *testing.T, dv *DrumView) {
	t.Helper()
	s := &dv.sampler
	any := false
	for i := 0; i < samplerKnobCount; i++ {
		if s.knobs[i] == nil || s.knobs[i].Rect().Empty() {
			continue
		}
		any = true
		cap := samplerKnobCaption(i, s)
		cell := s.knobCells[i]
		if cell.Empty() {
			t.Errorf("knob %d has no cell rect", i)
			continue
		}
		if w := TextWidth(cap); w > cell.Dx() {
			t.Errorf("knob %d caption %q width %d > cell width %d (overflows at full font)", i, cap, w, cell.Dx())
		}
	}
	if !any {
		t.Skip("no sampler knobs laid out")
	}
}
