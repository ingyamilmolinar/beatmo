package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Test seam: tests counting trace rects at screen coordinates turn it off.
var waveTraceCacheEnabled = true

// SetWaveTraceCacheForTest toggles the Wave-tab cache and returns a restore func.
func SetWaveTraceCacheForTest(on bool) func() {
	prev := waveTraceCacheEnabled
	waveTraceCacheEnabled = on
	return func() { waveTraceCacheEnabled = prev }
}

// Keyed on pointer identity, not a timestamp (WASM stamps none): a refresh hands back a fresh *State, and
// holding it prevents address reuse. The beat grid is baked in; the cursor is drawn live over the blit.
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
