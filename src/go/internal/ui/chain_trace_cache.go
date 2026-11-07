package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Tests that need a per-frame render turn it off.
var chainTraceCacheEnabled = true

// SetChainTraceCacheForTest toggles the trace cache and returns a restore func.
func SetChainTraceCacheForTest(on bool) func() {
	prev := chainTraceCacheEnabled
	chainTraceCacheEnabled = on
	return func() { chainTraceCacheEnabled = prev }
}

// The scope data is tracked by pointer (traceState); sample rate and density are session-constant.
type chainTraceKey struct {
	w, h     int
	mode     chainDisplayMode
	frozen   bool
	yGain    float64
	autoGain bool
	autoFit  bool
	windowMs float64
	showA    bool
	showB    bool
	// The cache bakes theme colors.
	theme int
}

// Reused so the blit is allocation-free.
var chainBlitOp ebiten.DrawImageOptions

// Rendered at a (0,0)-origin rect and blitted to cr.Min: drawChainTraces is rect-relative, so the
// pixels are identical.
func (z *ChainPanelZone) drawTracesCached(screen *ebiten.Image, cr image.Rectangle, state *scope.State) {
	w, h := cr.Dx(), cr.Dy()
	if w < 1 || h < 1 {
		return
	}
	if !chainTraceCacheEnabled {
		z.renderTracesDirect(screen, cr, state)
		return
	}

	key := chainTraceKey{
		w: w, h: h,
		mode: z.displayMode, frozen: z.frozen,
		yGain: z.yGain, autoGain: z.autoGain, autoFit: z.autoFit,
		windowMs: z.windowMs, showA: z.showTapA, showB: z.showTapB,
		theme: ThemeGeneration(),
	}

	dimsChanged := z.traceCache == nil ||
		z.traceCache.Bounds().Dx() != w || z.traceCache.Bounds().Dy() != h
	// state == traceState is the data-unchanged fast path.
	if !z.traceKeyOK || z.traceKey != key || z.traceState != state || dimsChanged {
		if dimsChanged {
			releaseImage(z.traceCache)
			z.traceCache = newTrackedImage("chainTrace", w, h)
		} else {
			z.traceCache.Clear()
		}
		z.renderTracesDirect(z.traceCache, image.Rect(0, 0, w, h), state)
		z.traceKey = key
		z.traceState = state
		z.traceKeyOK = true
	}

	bumpDrawCall()
	chainBlitOp.GeoM.Reset()
	chainBlitOp.GeoM.Translate(float64(cr.Min.X), float64(cr.Min.Y))
	screen.DrawImage(z.traceCache, &chainBlitOp)
}

// One renderer for both paths, so cached and uncached are pixel-identical.
func (z *ChainPanelZone) renderTracesDirect(dst *ebiten.Image, rect image.Rectangle, state *scope.State) {
	effectiveGain := z.yGain
	if z.autoGain && state != nil {
		peak := chainPeakAmplitude(state)
		if peak > 0.001 {
			effectiveGain = 0.9 / peak
			if effectiveGain < 1.0 {
				effectiveGain = 1.0
			}
			if effectiveGain > 16.0 {
				effectiveGain = 16.0
			}
		}
	}

	// Frame the window to the signal's active span (a transient is ~2 ms of a ~500 ms capture), with the
	// manual windowMs as the ceiling; sub-slicing is alloc-free, and the labels follow the effective window.
	drawState := state
	drawWindowMs := z.windowMs
	if z.autoFit && state != nil {
		if start, end := chainFitSpan(state, z.showTapA, z.showTapB); end > start {
			z.fitState = *state
			z.fitState.TapA.Samples = chainSliceSpan(state.TapA.Samples, start, end)
			z.fitState.TapB.Samples = chainSliceSpan(state.TapB.Samples, start, end)
			drawState = &z.fitState
			if sr := audio.SampleRate(); sr > 0 {
				spanMs := float64(end-start) * 1000.0 / float64(sr)
				if minMs := float64(Profile().DensityValues().ChainAutoFitMinMs); spanMs < minMs {
					spanMs = minMs
				}
				if spanMs > z.windowMs {
					spanMs = z.windowMs // manual zoom ceiling
				}
				drawWindowMs = spanMs
			}
		}
	}
	drawChainTraces(dst, rect, drawState, drawWindowMs, z.displayMode, z.frozen, effectiveGain, z.showTapA, z.showTapB)
}
