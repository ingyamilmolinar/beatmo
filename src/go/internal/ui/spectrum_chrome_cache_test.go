//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/hajimehoshi/ebiten/v2"
)

func setupSpectrumGame(t *testing.T) *Game {
	t.Helper()
	g := driveScene(t, "crop_eq_tab_spectrum")
	snap := snapshotWithSine()
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: channel.Of(r.Instrument), Name: r.Name, Snap: snap})
		}
	}
	testAnalyzerStateOverride = SynthesizeAnalyzerState(channel.Master, "Master", snap, rowSnaps, nil, channel.ID{}, "", audio.SampleRate())
	t.Cleanup(func() { testAnalyzerStateOverride = nil })
	g.drum.eqPanelZone.SetActiveTab(TabSpectrum)
	return g
}

// The dB grid depends on geometry, palette and locale, never audio, so every frame after the first is a hit;
// Spectrum has no data gate and once paid one blit per dash on every frame.
func TestSpectrumChromeCacheCollapsesStaticChrome(t *testing.T) {
	defer SetSpectrumChromeCacheForTest(true)()
	g := setupSpectrumGame(t)
	screen := ebiten.NewImage(1280, 720)

	draw := func() int64 {
		ResetImageMetrics()
		g.Draw(screen)
		return MetricDrawCallsTotal()
	}

	g.Draw(screen) // warm every cache on the tab (chrome + FFT curve sprite)
	cached := draw()

	restore := SetSpectrumChromeCacheForTest(false)
	g.drum.eqPanelZone.spectrumGridSprite.Invalidate()
	g.drum.eqPanelZone.spectrumLabelsSprite.Invalidate()
	g.Draw(screen) // warm the curve sprite again on the uncached path
	uncached := draw()
	restore()

	t.Logf("spectrum blits/frame: cached=%d uncached=%d", cached, uncached)
	if cached >= uncached {
		t.Fatalf("chrome cache saved nothing: cached=%d uncached=%d", cached, uncached)
	}
	// Close to the uncached cost means the chrome repaints per frame again.
	if cached > uncached/3 {
		t.Errorf("chrome cache ineffective: cached=%d is not well under a third of uncached=%d",
			cached, uncached)
	}
}

// Theme and locale change the baked pixels without moving the geometry, so the key must carry both.
func TestSpectrumChromeCacheInvalidatesOnThemeAndLocale(t *testing.T) {
	defer SetSpectrumChromeCacheForTest(true)()

	rect := image.Rect(0, 0, 800, 200)
	base := spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: freqScaleLog,
		theme: ThemeGeneration(), locale: i18n.ActiveLocale()}

	if k := (spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: freqScaleLinear,
		theme: base.theme, locale: base.locale}); k == base {
		t.Error("scale is not part of the chrome key — linear/log share a cache entry")
	}
	if k := (spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: freqScaleLog,
		theme: base.theme + 1, locale: base.locale}); k == base {
		t.Error("theme generation is not part of the chrome key — a theme pick leaves stale chrome")
	}
	other := i18n.LocaleES
	if base.locale == other {
		other = i18n.LocaleEN
	}
	if k := (spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: freqScaleLog,
		theme: base.theme, locale: other}); k == base {
		t.Error("locale is not part of the chrome key — a language switch leaves stale Bass/Mids/Treble labels")
	}
}

// By construction: one renderer, differing only in destination and translation, so the (w, h, colour) multisets
// match; offscreen renders cannot be read back under -tags test.
func TestSpectrumChromeRenderMatchesDirect(t *testing.T) {
	type shape struct {
		w, h int
		c    color.RGBA
		fill bool
	}
	collect := func(r image.Rectangle, detail bool) map[shape]int {
		got := map[shape]int{}
		orig := drawRect
		drawRect = func(d *ebiten.Image, rr image.Rectangle, c color.Color, filled bool) {
			cr, cg, cb, ca := c.RGBA()
			got[shape{rr.Dx(), rr.Dy(),
				color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)}, filled}]++
		}
		defer func() { drawRect = orig }()
		dst := ebiten.NewImage(r.Dx()+r.Min.X+8, r.Dy()+r.Min.Y+8)
		drawSpectrumGridChrome(dst, r, detail, spectrumDBWindow{})
		drawSpectrumLabelChrome(dst, r, freqScaleLog, detail)
		return got
	}

	const w, h = 900, 220
	// The flag is part of the key, so each state has its own entry.
	for _, detail := range []bool{false, true} {
		atOrigin := collect(image.Rect(0, 0, w, h), detail)
		onScreen := collect(image.Rect(37, 121, 37+w, 121+h), detail)

		if len(atOrigin) == 0 {
			t.Fatalf("detail=%v: chrome renderer drew nothing — the check would be vacuous", detail)
		}
		if len(atOrigin) != len(onScreen) {
			t.Fatalf("detail=%v: shape-kind count differs: origin=%d onscreen=%d",
				detail, len(atOrigin), len(onScreen))
		}
		for sh, n := range atOrigin {
			if onScreen[sh] != n {
				t.Errorf("detail=%v shape %+v: origin drew %d, on-screen drew %d",
					detail, sh, n, onScreen[sh])
			}
		}
	}
}
