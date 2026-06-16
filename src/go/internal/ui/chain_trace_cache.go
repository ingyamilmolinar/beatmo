package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// chainTraceCacheEnabled gates the Chain-tab waveform-trace cache. It is the
// test seam (mirrors SetRowsLayerWindowingForTest): tests that need the trace
// re-rendered every frame — e.g. a per-frame draw-call count or an animation
// assertion — flip it off so Draw always takes the direct path. Production
// keeps it on so inter-tick frames are a single blit instead of ~5k blits.
var chainTraceCacheEnabled = true

// SetChainTraceCacheForTest toggles the trace cache and returns a restore func.
func SetChainTraceCacheForTest(on bool) func() {
	prev := chainTraceCacheEnabled
	chainTraceCacheEnabled = on
	return func() { chainTraceCacheEnabled = prev }
}

// chainTraceKey fingerprints the layout/display inputs that determine the
// trace pixels. The scope data itself is tracked separately by pinning the
// *scope.State pointer (z.traceState) — see that field's doc for why pointer
// identity (not state.Timestamp) is the correct freshness signal. Sample rate
// and density tokens are session-constant (a density change comes with a
// relayout that changes w/h), so they need not enter the key.
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
}

// chainBlitOp is the reusable blit options for the cached-trace DrawImage so
// the per-frame blit stays allocation-free.
var chainBlitOp ebiten.DrawImageOptions

// drawTracesCached renders the waveform trace area into screen at cr, going
// through an offscreen cache when nothing that affects the trace has changed
// since the last render. drawChainTraces draws entirely rect-relative, so the
// cache is rendered at a (0,0)-origin rect and blitted to cr.Min — the blitted
// pixels are identical to rendering directly at cr.
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
	}

	dimsChanged := z.traceCache == nil ||
		z.traceCache.Bounds().Dx() != w || z.traceCache.Bounds().Dy() != h
	// Rebuild on a new scope state (data refreshed), any layout/display change,
	// or a resize. state == z.traceState is the data-unchanged fast path.
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

// renderTracesDirect computes the effective Y-gain / auto-fit window and draws
// the traces into dst at rect. This is the exact pre-cache body lifted out of
// Draw verbatim, used both on a cache miss (rect at origin) and when the cache
// is disabled (rect == the real content rect). Keeping one renderer guarantees
// the cached and uncached paths are pixel-identical.
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

	// Auto-fit: frame the X window to the signal's active span so the
	// waveform fills the trace. The scope captures ~500ms but a transient is
	// ~2ms; without this the trace is ~98% dead width and the time-axis
	// labels (driven by windowMs) disagree with what's rendered. We sub-slice
	// each tap's samples (alloc-free, into reusable scratch) and derive an
	// effective window so labels match. Manual zoom (z.windowMs) is the
	// ceiling. See [[project_chain_tab_redesign]] / scope.ActiveSpan.
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
