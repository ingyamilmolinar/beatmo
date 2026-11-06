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

// setupSpectrumGame drives the Spectrum tab with a synthetic analyzer state
// through the same override the audio-panel pixel tests use.
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

// TestSpectrumChromeCacheCollapsesStaticChrome pins the reason this cache
// exists. The Spectrum tab's dB grid is STATIC — it depends on the panel
// geometry, the palette and the locale, never on audio — yet it was repainted
// every frame, and its dashed lines cost one blit PER DASH: measured at 832 of
// the tab's 1032 per-frame blits (82%), 4 dashed lines x ~208 dashes.
//
// Unlike the Wave/Chain trace caches, this one is not rate-limited by the
// ~30 Hz analyzer tick: the chrome key does not change at all during playback,
// so every frame after the first is a hit. That matters because Spectrum's
// cost was framerate-INVARIANT — the one data tab that paid full price on
// every frame — which is what starved the sequencer goroutine on the single
// WASM thread and made audio lag under this tab.
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
	// The dashed grid alone was ~832 blits. Anything close to the uncached
	// cost means the chrome is being repainted per frame again.
	if cached > uncached/3 {
		t.Errorf("chrome cache ineffective: cached=%d is not well under a third of uncached=%d",
			cached, uncached)
	}
}

// TestSpectrumChromeCacheInvalidatesOnThemeAndLocale guards the two inputs that
// change the BAKED pixels without changing the geometry. A key missing either
// one leaves stale chrome on screen after a theme pick or a language switch —
// the failure mode the runtime-theme work calls out (a cache that bakes a color
// must be dropped by the theme chokepoint or keyed on the generation).
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

// TestSpectrumChromeRenderMatchesDirect proves the cache is pixel-equivalent BY
// CONSTRUCTION rather than by comparing rasterized output, which Go cannot do
// here (the chain-cache precedent: an offscreen render is not readable back
// under -tags test). The cached and uncached paths call the SAME renderer; the
// only difference is the destination and a translation. So rendering the chrome
// at the origin must issue an identical multiset of (w, h, color) rects to
// rendering it on-screen.
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
	// Both detail states: the flag is part of the sprite key, so each renders
	// into its own cache entry and each must be translation-equivalent.
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
