//go:build js && !test

package ui

import (
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

	// __navSegRect(i) – DIAGNOSTIC: bottom-nav segmented-control segment rect.
	js.Global().Set("__navSegRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.viewSwitchSegmented == nil || len(args) == 0 {
			return js.Null()
		}
		r := g.drum.viewSwitchSegmented.SegmentRect(args[0].Int())
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	// __viewMode() – DIAGNOSTIC: current mobile view mode (int).
	js.Global().Set("__viewMode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(g.drum.currentViewMode))
	}))
	// __samplerHitAreas() – DIAGNOSTIC: sampler-tab hit-area rects+tags.
	js.Global().Set("__samplerHitAreas", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arr := js.Global().Get("Array").New()
		if g.drum == nil {
			return arr
		}
		for _, ha := range g.drum.samplerTabHitAreas() {
			o := js.Global().Get("Object").New()
			o.Set("tag", ha.Tag)
			o.Set("x", ha.Rect.Min.X)
			o.Set("y", ha.Rect.Min.Y)
			o.Set("w", ha.Rect.Dx())
			o.Set("h", ha.Rect.Dy())
			arr.Call("push", o)
		}
		return arr
	}))
	// __treeCapturing() – DIAGNOSTIC: {capturing, tag} of the tree's drag capture.
	js.Global().Set("__treeCapturing", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		o := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.tree == nil {
			return o
		}
		o.Set("capturing", g.drum.tree.Capturing())
		o.Set("tag", g.drum.tree.CapturedTag())
		return o
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

	// tickSequencer() — sub-frame sequencer drive. Called from a JS
	// setInterval at the audio rate so the sequencer fires even when the
	// Go goroutine ticker is starved by long RAF/Update spans. JS timers
	// are queued onto the macro-task queue and run independently of the
	// WASM Go runtime's cooperative scheduler, so this is the only drive
	// that can fire sub-frame in WASM. No-op when not playing.
	js.Global().Set("tickSequencer", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.Playing() {
			g.seqScheduleTime()
		}
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

	// dumpHeapProbe() returns a CSV string of the heap-growth probe ring
	// (one row per ~1-second sample for the past minute). Empty when the
	// probe is disabled (set BEATMO_HEAP_PROBE=1 to enable).
	js.Global().Set("dumpHeapProbe", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return HeapProbeSnapshot()
	}))

	// dumpLevelsLatch() returns the Levels-tab peak/RMS-hold state for
	// debugging Phase 0a of the audio-panel redesign. Stays callable
	// after the redesign lands; helps validate the bar-fills-between-
	// hits invariant by exposing the latch fields the renderer reads.
	js.Global().Set("dumpLevelsLatch", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.eqPanelZone == nil {
			obj.Set("ready", false)
			return obj
		}
		z := g.drum.eqPanelZone
		obj.Set("ready", true)
		obj.Set("peakHoldDB", z.levelsLatch.PeakHoldDB)
		obj.Set("rmsHoldDB", z.levelsLatch.RMSHoldDB)
		obj.Set("peakHoldSeeded", z.levelsLatch.peakHoldSeeded)
		obj.Set("rmsHoldSeeded", z.levelsLatch.rmsHoldSeeded)
		obj.Set("latched", z.levelsLatch.Latched())
		obj.Set("activeTab", int(z.ActiveTab()))
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
		g.Draw(img)
		g.perfDrawMuted = wasMuted
		if !wasMuted && !g.perf.started.IsZero() {
			g.perf.started = g.perf.started.Add(time.Since(start))
		}
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
		obj.Set("recordingActive", s.RecordingActive)
		obj.Set("recordingDrops", int(s.RecordingDrops))
		obj.Set("recordingMasterQDepth", s.RecordingMasterQDepth)
		obj.Set("recordingPoolFree", s.RecordingPoolFree)
		obj.Set("recordingBytesUsed", float64(s.RecordingBytesUsed))
		obj.Set("heapAllocKB", int(s.HeapAllocKB))
		obj.Set("heapSysKB", int(s.HeapSysKB))
		obj.Set("heapObjects", int(s.HeapObjects))
		obj.Set("goroutines", s.Goroutines)
		// Extra UI metrics for perf analysis (web only usage):
		if g.drum != nil {
			obj.Set("rowsLayerBytes", g.drum.rowsLayerBytes)
			obj.Set("rowsRepaints", g.drum.rowsRepaints)
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
		obj.Set("liveImages", g.snapshotLiveImages())
		obj.Set("imagesAllocatedTotal", float64(MetricImagesAllocatedTotal()))
		obj.Set("imagesByTag", imagesByTagSnapshot())
		// Three-stage latency: nested object so callers reading only the
		// existing flat fields don't conflict with the new keys.
		obj.Set("threeStage", threeStageLatencyJSObject(s.SchedMetrics))
		return obj
	}))
	js.Global().Set("resetPerfStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.perf.reset()
		return nil
	}))
	// getThreeStageLatency() — returns Go-side schedule metrics covering
	// all three stages of the real → scheduler → audio pipeline. This is
	// orthogonal to the JS-side audio.js scheduleMetrics (which only sees
	// Stage C). Returns null before any data is observed (count==0).
	js.Global().Set("getThreeStageLatency", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		s := g.PerfSnapshot()
		return threeStageLatencyJSObject(s.SchedMetrics)
	}))
	js.Global().Set("resetThreeStageLatency", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.schedMetrics.Reset()
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

	// forceDrag(x0,y0,x1,y1[,steps]) drives a REAL left-button drag through
	// Update() with overridden input — press at (x0,y0), hold across interpolated
	// points to (x1,y1), then release. Mirrors forceGameTick but for drags, which
	// otherwise don't register in headless (rAF-throttled) because no equivalent
	// forced-tick exists. Used by the agent's drag tool to move EQ band handles,
	// synth knobs, and sliders deterministically.
	js.Global().Set("forceDrag", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			return nil
		}
		x0, y0 := args[0].Int(), args[1].Int()
		x1, y1 := args[2].Int(), args[3].Int()
		steps := 8
		if len(args) >= 5 && !args[4].IsUndefined() {
			steps = args[4].Int()
		}
		if steps < 1 {
			steps = 1
		}
		curX, curY, pressed := x0, y0, true
		oldCur := cursorPosition
		oldBtn := isMouseButtonPressed
		cursorPosition = func() (int, int) { return curX, curY }
		isMouseButtonPressed = func(b ebiten.MouseButton) bool {
			return pressed && b == ebiten.MouseButtonLeft
		}
		defer func() {
			cursorPosition = oldCur
			isMouseButtonPressed = oldBtn
		}()
		// Press at the start point.
		g.Update()
		// Hold the button across interpolated points (the drag motion).
		for i := 1; i <= steps; i++ {
			t := float64(i) / float64(steps)
			curX = x0 + int(float64(x1-x0)*t)
			curY = y0 + int(float64(y1-y0)*t)
			g.Update()
		}
		// Release at the end point.
		curX, curY = x1, y1
		pressed = false
		g.Update()
		return nil
	}))
}
