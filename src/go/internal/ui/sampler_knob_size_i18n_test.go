//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func samplerGainDiaDesktop(t *testing.T, loc i18n.Locale) int {
	t.Helper()
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	i18n.SetLocale(loc)
	g.drum.sampler.captureFromSynth(roster.Kick)
	// 180 px is too short for a minimum-diameter dial.
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 260), roster.Kick)
	return g.drum.sampler.knobs[samplerKnobGain].Rect().Dx()
}

// Mobile knobs scroll in one column, so Gain is usually off-window; the first visible one shows the size.
func samplerFirstVisibleKnobDiaMobile(t *testing.T, loc i18n.Locale) int {
	t.Helper()
	i18n.SetLocale(loc)
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)
	if !g.drum.sampler.hasBuffer() {
		t.Skip("no sampler buffer captured")
	}
	for _, k := range g.drum.sampler.knobs {
		if k != nil && !k.Rect().Empty() {
			return k.Rect().Dx()
		}
	}
	t.Fatal("no visible sampler knob on mobile")
	return 0
}

// A longer Spanish caption may not shrink the dial.
func TestSamplerKnobSizeLanguageInvariantDesktop(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	en := samplerGainDiaDesktop(t, i18n.LocaleEN)
	es := samplerGainDiaDesktop(t, i18n.LocaleES)
	if en != es {
		t.Errorf("desktop Gain dial diameter EN=%d ES=%d — must be identical regardless of language", en, es)
	}
}

func TestSamplerKnobSizeLanguageInvariantMobile(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	en := samplerFirstVisibleKnobDiaMobile(t, i18n.LocaleEN)
	es := samplerFirstVisibleKnobDiaMobile(t, i18n.LocaleES)
	if en != es {
		t.Errorf("mobile knob dial diameter EN=%d ES=%d — must be identical regardless of language", en, es)
	}
}

// At least half a ParamCardW wide in every language on every screen.
func TestSamplerKnobUsableSizeAllLanguages(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	min := Profile().DensityValues().ParamCardW / 2
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		if d := samplerGainDiaDesktop(t, loc); d < min {
			t.Errorf("desktop %s Gain card width %d < usable min %d (ParamCardW/2)", loc, d, min)
		}
	}
}

func TestSamplerKnobUsableSizeMobileAllLanguages(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		g := newMobileSamplerTabGameForTestSize(t, 390, 844)
		if !g.drum.sampler.hasBuffer() {
			t.Skip("no sampler buffer captured")
		}
		min := Profile().DensityValues().ParamCardW / 2
		wantH := Profile().DensityValues().ParamCardH
		// Every visible card is at least half a ParamCardW wide and exactly ParamCardH tall; off-window knobs are
		// reachable by scrolling.
		visible := 0
		for i, k := range g.drum.sampler.knobs {
			if k == nil || k.Rect().Empty() {
				continue
			}
			visible++
			if d := k.Rect().Dx(); d < min {
				t.Errorf("mobile %s knob[%d] card width %d < usable min %d", loc, i, d, min)
			}
			if h := k.Rect().Dy(); h != wantH {
				t.Errorf("mobile %s knob[%d] card height %d != ParamCardH %d", loc, i, h, wantH)
			}
		}
		if visible == 0 {
			t.Errorf("mobile %s: no sampler knob visible", loc)
		}
	}
}
