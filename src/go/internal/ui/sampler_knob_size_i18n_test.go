//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// samplerGainDiaDesktop returns the Gain knob's dial diameter on the desktop
// Sampler tab in the given locale.
func samplerGainDiaDesktop(t *testing.T, loc i18n.Locale) int {
	t.Helper()
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	i18n.SetLocale(loc)
	g.drum.sampler.captureFromSynth("kick")
	// A realistic desktop audio-panel content height (the 180px rect other
	// tests use is artificially short — it can't fit a min-diameter dial).
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 260), "kick")
	return g.drum.sampler.knobs[samplerKnobGain].Rect().Dx()
}

// samplerFirstVisibleKnobDiaMobile returns the first VISIBLE knob's dial
// diameter on a real mobile Sampler tab in the given locale. On mobile the knobs
// scroll in a single column, so the last knob (Gain) is usually off-window; the
// first visible knob is what proves the dial renders at a usable, language-
// invariant size.
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

// Knob size must NOT depend on the UI language — a longer Spanish caption may
// not shrink the dial. Desktop: EN and ES must produce the same diameter.
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

// Knobs must stay usable (>= the density's minimum dial) in every language on
// every screen — never the tiny fallback the old caption-driven layout produced.
func TestSamplerKnobUsableSizeAllLanguages(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	min := Profile().DensityValues().SynthKnobMin
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		if d := samplerGainDiaDesktop(t, loc); d < min {
			t.Errorf("desktop %s Gain dial %d < usable min %d", loc, d, min)
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
		min := Profile().DensityValues().SynthKnobMin
		// On mobile the knobs scroll, so only some are laid out — but EVERY
		// VISIBLE dial must be usable (>= min), never the tiny caption-driven
		// fallback the old layout produced. Off-window knobs (empty rect) are
		// reachable by scrolling and intentionally not laid out.
		visible := 0
		for i, k := range g.drum.sampler.knobs {
			if k == nil || k.Rect().Empty() {
				continue
			}
			visible++
			if d := k.Rect().Dx(); d < min {
				t.Errorf("mobile %s knob[%d] dial %d < usable min %d", loc, i, d, min)
			}
		}
		if visible == 0 {
			t.Errorf("mobile %s: no sampler knob visible", loc)
		}
	}
}
