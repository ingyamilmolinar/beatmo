//go:build js && !test

package ui

import (
	"strconv"
	"syscall/js"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) initJSPlaybackPerf() {
	js.Global().Set("startPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		js.Global().Get("console").Call("log", "[WASM] startPlay() called")
		g.drum.playPressed = true
		return nil
	}))
	js.Global().Set("stopPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.drum.stopPressed = true
		return nil
	}))

	// syncHighlights() – drain pending highlight events and decay animations.
	// Used by tests to process highlight state when Ebiten's Update() is starved.
	// When playing, drives scheduling from the caller's thread so highlights
	// stay current even if the sequencer goroutine's timer is delayed under
	// CPU contention (e.g. parallel test runs).
	js.Global().Set("syncHighlights", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.Playing() {
			g.seqScheduleTime()
		}
		g.drainAndDecayHighlights()
		return nil
	}))

	// setSimpleDraw(bool) – reduce rendering complexity for perf (web).
	js.Global().Set("setSimpleDraw", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		mode := args[0].Bool()
		if mode {
			if g.drum != nil {
				g.simpleDrawSavedFollow = g.drum.FollowPlayback()
				g.simpleDrawSavedFollowValid = true
				g.drum.SetFollow(false)
			}
		} else {
			if g.drum != nil && g.simpleDrawSavedFollowValid {
				g.drum.SetFollow(g.simpleDrawSavedFollow)
			}
			g.simpleDrawSavedFollowValid = false
		}
		g.simpleDraw = mode
		g.simpleDrawAutoDisableFrames = 0
		return nil
	}))

	// setPerfFastPath(bool) – enable lower-overhead Update/refresh for perf harnesses.
	js.Global().Set("setPerfFastPath", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		g.SetPerfFastPath(args[0].Bool())
		return nil
	}))

	// setDrawThrottle(ms int) – minimum interval between Draws (web only).
	js.Global().Set("setDrawThrottle", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		ms := args[0].Int()
		if ms < 0 {
			ms = 0
		}
		g.drawMinInterval = time.Duration(ms) * time.Millisecond
		return nil
	}))

	// setAudioLookahead(sec float64) – scheduling lookahead for WebAudio.
	js.Global().Set("setAudioLookahead", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		g.audioLookaheadSec = args[0].Float()
		if g.audioLookaheadSec < 0 {
			g.audioLookaheadSec = 0
		}
		if g.audioLookaheadSec > 0.1 {
			g.audioLookaheadSec = 0.1
		}
		return nil
	}))

	// gridCacheInfo() -> { tileReady: bool, cacheReady: bool, simpleDraw: bool }
	js.Global().Set("gridCacheInfo", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("tileReady", g.gridTile != nil)
		obj.Set("cacheReady", g.gridCache != nil && g.gridCacheW > 0)
		obj.Set("simpleDraw", g.simpleDraw)
		return obj
	}))

	// forceDraw() – render one frame into an offscreen image to build caches.
	// Also reads current window.innerWidth/innerHeight and calls Layout() so
	// that a preceding setViewportSize (Playwright) is reflected immediately.
	js.Global().Set("forceDraw", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		win := js.Global().Get("window")
		if !win.IsUndefined() {
			iw := win.Get("innerWidth").Int()
			ih := win.Get("innerHeight").Int()
			if iw > 0 && ih > 0 {
				g.Layout(iw, ih)
			}
		}
		w, h := g.winW, g.winH
		if w <= 0 {
			w = 800
		}
		if h <= 0 {
			h = 600
		}
		img := ebiten.NewImage(w, h)
		start := time.Now()
		wasMuted := g.perfDrawMuted
		g.perfDrawMuted = true
		g.lastDrawAt = time.Time{} // bypass draw throttle so drawGridPane runs
		g.Draw(img)
		g.perfDrawMuted = wasMuted
		if !wasMuted && !g.perf.started.IsZero() {
			g.perf.started = g.perf.started.Add(time.Since(start))
		}
		return nil
	}))

	// setRowsLayerStripes(count) – enable stripe-based rows-layer compositing (web-only prototype).
	js.Global().Set("setRowsLayerStripes", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		n := args[0].Int()
		if n < 0 {
			// Auto mode: start at 2 stripes, allow tuner to adjust.
			g.drum.rowsStripingEnabled = true
			g.drum.rowsStripeAuto = true
			g.drum.rowsStripeCount = 0
		} else {
			if n < 2 {
				g.drum.rowsStripingEnabled = false
				g.drum.rowsStripeAuto = false
				g.drum.rowsStripeCount = 0
			} else {
				if n > wasmStripeMaxCount {
					n = wasmStripeMaxCount
				}
				g.drum.rowsStripingEnabled = true
				g.drum.rowsStripeAuto = false
				g.drum.rowsStripeCount = n
			}
		}
		g.drum.rowsStripes = nil
		g.drum.rowsStripeStarts = nil
		g.drum.rowsStripeWidths = nil
		g.drum.rowsStripeGen = 0
		g.drum.rowsStripeScratch = nil
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// rowsLayerState() -> diagnostic info about the drum rows layer caches.
	js.Global().Set("rowsLayerState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(nil)
		}
		obj := js.Global().Get("Object").New()
		obj.Set("layerOffset", g.drum.rowsLayerOffset)
		obj.Set("layerGen", g.drum.rowsLayerGen)
		obj.Set("stripeOffset", g.drum.rowsStripeOffset)
		obj.Set("stripeGen", g.drum.rowsStripeGen)
		obj.Set("striping", g.drum.rowsStripingEnabled && g.drum.rowsStripeCount > 1)
		return obj
	}))

	// setBPM(n) sets the DrumView BPM directly for perf tests.
	js.Global().Set("setBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		b := args[0].Int()
		if b < 1 {
			b = 1
		}
		g.drum.SetBPM(b)
		return nil
	}))

	// commitBPM(n) simulates typing a number and pressing Enter in the BPM box.
	js.Global().Set("commitBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmBox() == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		b := args[0].Int()
		if b < 1 {
			b = 1
		}
		g.drum.bpmBox().focused = true
		g.drum.bpmBox().SetText(strconv.Itoa(b))
		g.drum.SetBPM(b)
		g.drum.bpmBox().focused = false
		return nil
	}))

	// perfStats() -> object with recent perf metrics.
	js.Global().Set("perfStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		s := g.PerfSnapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("frames", int(s.Frames))
		obj.Set("fpsAvg", s.FPSAvg)
		obj.Set("updateAvgMS", s.UpdateAvgMS)
		obj.Set("updateMaxMS", s.UpdateMaxMS)
		obj.Set("drawAvgMS", s.DrawAvgMS)
		obj.Set("drawMaxMS", s.DrawMaxMS)
		obj.Set("audioEnq", int(s.AudioEnq))
		obj.Set("audioDeq", int(s.AudioDeq))
		obj.Set("audioQLatAvg", s.AudioQLatAvg)
		obj.Set("audioQLatMax", s.AudioQLatMax)
		obj.Set("audioCallAvg", s.AudioCallAvg)
		obj.Set("audioCallMax", s.AudioCallMax)
		obj.Set("audioDrops", int(s.AudioDrops))
		obj.Set("heapAllocKB", int(s.HeapAllocKB))
		obj.Set("heapSysKB", int(s.HeapSysKB))
		obj.Set("heapObjects", int(s.HeapObjects))
		obj.Set("goroutines", s.Goroutines)
		// Extra UI metrics for perf analysis (web only usage):
		if g.drum != nil {
			obj.Set("rowsLayerBytes", g.drum.rowsLayerBytes)
			obj.Set("rowsRepaints", g.drum.rowsRepaints)
			obj.Set("rowsStripeEnabled", g.drum.rowsStripingEnabled)
			obj.Set("rowsStripeCount", g.drum.rowsStripeCount)
			obj.Set("rowCacheShift", g.drum.rowCacheShift)
			obj.Set("rowCachePatch", g.drum.rowCachePatch)
			obj.Set("rowCacheFull", g.drum.rowCacheFull)
			obj.Set("gridCachePad", g.gridCachePad)
			obj.Set("edgeCachePad", g.edgeCachePad)
			obj.Set("drawGridMS", g.lastDrawGridMS)
			obj.Set("drawDrumMS", g.lastDrawDrumMS)
			obj.Set("updateMS", g.lastUpdateMS)
			obj.Set("refreshMS", g.lastRefreshMS)
		}
		return obj
	}))
	js.Global().Set("resetPerfStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.perf.reset()
		return nil
	}))

	// forceGameTick(x, y) — Force one Update() tick with mouse at (x, y).
	// Used by the agent test infrastructure to reliably process button clicks
	// in headless Chromium where requestAnimationFrame fires infrequently.
	// Without arguments, runs Update() with mouse unpressed at (0, 0).
	js.Global().Set("forceGameTick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		mx, my := 0, 0
		mouseLeft := false
		if len(args) >= 2 && !args[0].IsUndefined() {
			mx = args[0].Int()
			my = args[1].Int()
			mouseLeft = true
		}
		oldCur := cursorPosition
		oldBtn := isMouseButtonPressed
		cursorPosition = func() (int, int) { return mx, my }
		isMouseButtonPressed = func(b ebiten.MouseButton) bool {
			return mouseLeft && b == ebiten.MouseButtonLeft
		}
		defer func() {
			cursorPosition = oldCur
			isMouseButtonPressed = oldBtn
		}()
		g.Update()
		return nil
	}))
}
