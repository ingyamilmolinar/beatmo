package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// waveTraceCacheEnabled gates the Wave-tab waveform cache (test seam, mirrors
// chainTraceCacheEnabled / SetRowsLayerWindowingForTest). Tests that count the
// per-frame trace rects at on-screen coordinates flip it off so Draw renders
// the waveform directly instead of into the (0,0)-origin cache.
var waveTraceCacheEnabled = true

// SetWaveTraceCacheForTest toggles the Wave-tab cache and returns a restore func.
func SetWaveTraceCacheForTest(on bool) func() {
	prev := waveTraceCacheEnabled
	waveTraceCacheEnabled = on
	return func() { waveTraceCacheEnabled = prev }
}

// drawWaveformCached renders the Wave-tab waveform into screen at cr, going
// through an offscreen cache when the analyzer data and layout are unchanged.
//
// The cache key is the (analyzer-state, channel, capture) POINTER identity plus
// the frozen flag and cr size — NOT any timestamp (the WASM analyzer path, like
// the scope path, doesn't stamp one). ScopeState/AnalyzerState hand back a fresh
// *analyzer.State whenever the data refreshes (every ~30 Hz tick on desktop /
// stateCacheTTL on WASM), so a changed pointer is the freshness signal; holding
// the state ref also keeps the &state.Master / state.Detail / state.Capture
// pointers alive and prevents address reuse.
//
// The beat-grid overlay is rendered INTO the cache (preserving its exact
// behind-the-trace z-order) and so refreshes at the 30 Hz data rate — coherent
// with the waveform, which already updates at 30 Hz. The mouse cursor crosshair
// is NOT part of this; the caller draws it live over the returned blit.
func (z *EQPanelZone) drawWaveformCached(screen *ebiten.Image, cr image.Rectangle, state *analyzer.State, ch *analyzer.ChannelMetrics, cap *analyzer.CaptureBuffer, beatGrid []float64) {
	w, h := cr.Dx(), cr.Dy()
	if w < 1 || h < 1 {
		return
	}
	gain, autoOn := z.resolveWaveGain(state, ch, cap)
	tabFrozen := z.tabFrozen(TabWave)
	if !waveTraceCacheEnabled {
		drawAnalyzerWaveform(screen, cr, ch, cap, beatGrid, gain, autoOn, tabFrozen)
		return
	}

	frozen := cap != nil && cap.Frozen
	// Quantize gain to 0.01 so tiny smoothing deltas don't rebuild every frame.
	gq := math.Round(gain*100) / 100
	dimsChanged := z.waveCache == nil ||
		z.waveCache.Bounds().Dx() != w || z.waveCache.Bounds().Dy() != h
	if !z.waveKeyOK || dimsChanged || z.waveKeyW != w || z.waveKeyH != h ||
		z.waveKeyState != state || z.waveKeyCh != ch || z.waveKeyCap != cap ||
		z.waveKeyFrozen != frozen || z.waveKeyTabFrozen != tabFrozen || z.waveKeyGain != gq {
		if dimsChanged {
			releaseImage(z.waveCache)
			z.waveCache = newTrackedImage("waveTrace", w, h)
		} else {
			z.waveCache.Clear()
		}
		drawAnalyzerWaveform(z.waveCache, image.Rect(0, 0, w, h), ch, cap, beatGrid, gain, autoOn, tabFrozen)
		z.waveKeyW, z.waveKeyH = w, h
		z.waveKeyState, z.waveKeyCh, z.waveKeyCap = state, ch, cap
		z.waveKeyFrozen = frozen
		z.waveKeyTabFrozen = tabFrozen
		z.waveKeyGain = gq
		z.waveKeyOK = true
	}

	bumpDrawCall()
	chainBlitOp.GeoM.Reset()
	chainBlitOp.GeoM.Translate(float64(cr.Min.X), float64(cr.Min.Y))
	screen.DrawImage(z.waveCache, &chainBlitOp)
}
