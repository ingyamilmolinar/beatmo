//go:build js && !test

package ui

import (
	"syscall/js"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) initJSPlaybackPerf() {
	js.Global().Set("startPlay", jsFn(func(args jsArgs) any {
		js.Global().Get("console").Call("log", "[WASM] startPlay() called")
		g.drum.playPressed = true
		return nil
	}))
	js.Global().Set("stopPlay", jsFn(func(args jsArgs) any {
		g.drum.stopPressed = true
		return nil
	}))

	// The import-cold bench waits on it; the re-import check asserts total==0 (everything cached).
	js.Global().Set("warmupProgress", jsFn(func(args jsArgs) any {
		done, total := 0, 0
		if g.warmup != nil {
			done, total = g.warmup.Progress()
		}
		return map[string]any{"done": done, "total": total, "ready": g.playbackReady()}
	}))

	// __navSegRect(i) – DIAGNOSTIC: bottom-nav segmented-control segment rect.
	js.Global().Set("__navSegRect", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.viewSwitchSegmented == nil || args.Len() == 0 {
			return js.Null()
		}
		r := g.drum.viewSwitchSegmented.SegmentRect(args.Int(0))
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	// __viewMode() – DIAGNOSTIC: current mobile view mode (int).
	js.Global().Set("__viewMode", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(g.drum.currentViewMode))
	}))
	// __samplerHitAreas() – DIAGNOSTIC: sampler-tab hit-area rects+tags.
	js.Global().Set("__samplerHitAreas", jsFn(func(args jsArgs) any {
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
	js.Global().Set("__treeCapturing", jsFn(func(args jsArgs) any {
		o := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.tree == nil {
			return o
		}
		o.Set("capturing", g.drum.tree.Capturing())
		o.Set("tag", g.drum.tree.CapturedTag())
		return o
	}))

	// syncHighlights(): drains highlights when Update is starved, and drives scheduling from the caller's thread
	// so highlights stay current when the sequencer timer is delayed.
	js.Global().Set("syncHighlights", jsFn(func(args jsArgs) any {
		if g.Playing() {
			g.seqScheduleTime()
		}
		g.drainAndDecayHighlights()
		return nil
	}))

	// tickSequencer(): JS timers run outside Go's cooperative scheduler, so a setInterval drive is the only one
	// that fires sub-frame on WASM. No-op when stopped.
	js.Global().Set("tickSequencer", jsFn(func(args jsArgs) any {
		if g.Playing() {
			g.seqScheduleTime()
		}
		return nil
	}))

	// setSimpleDraw(bool) – reduce rendering complexity for perf (web).
	js.Global().Set("setSimpleDraw", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		mode := args.Bool(0)
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
	js.Global().Set("setPerfFastPath", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		g.SetPerfFastPath(args.Bool(0))
		return nil
	}))

	// setAudioLookahead(sec float64) – scheduling lookahead for WebAudio.
	js.Global().Set("setAudioLookahead", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		g.audioLookaheadSec = args.Float(0)
		if g.audioLookaheadSec < 0 {
			g.audioLookaheadSec = 0
		}
		if g.audioLookaheadSec > 0.1 {
			g.audioLookaheadSec = 0.1
		}
		return nil
	}))

	// gridCacheInfo() -> { tileReady: bool, cacheReady: bool, simpleDraw: bool }
	js.Global().Set("gridCacheInfo", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		obj.Set("tileReady", g.gridTile != nil)
		obj.Set("cacheReady", g.gridCache != nil && g.gridCacheW > 0)
		obj.Set("simpleDraw", g.simpleDraw)
		return obj
	}))

	// dumpHeapProbe() -> CSV, one row per second for the last minute; empty unless BEATMO_HEAP_PROBE=1.
	js.Global().Set("dumpHeapProbe", jsFn(func(args jsArgs) any {
		return HeapProbeSnapshot()
	}))

	// dumpLevelsLatch(): the hold fields the Levels renderer reads.
	js.Global().Set("dumpLevelsLatch", jsFn(func(args jsArgs) any {
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

	// forceDraw(): also re-reads the window size so a preceding setViewportSize takes effect at once.
	js.Global().Set("forceDraw", jsFn(func(args jsArgs) any {
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
	js.Global().Set("rowsLayerState", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return js.ValueOf(nil)
		}
		obj := js.Global().Get("Object").New()
		obj.Set("layerOffset", g.drum.rowsLayerOffset)
		obj.Set("layerGen", g.drum.rowsLayerGen)
		return obj
	}))

	// setBPM(n) sets the DrumView BPM directly for perf tests.
	js.Global().Set("setBPM", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		b := args.Int(0)
		if b < 1 {
			b = 1
		}
		g.drum.SetBPM(b)
		return nil
	}))

	// perfStats() -> object with recent perf metrics.
	js.Global().Set("perfStats", jsFn(func(args jsArgs) any {
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
		// On single-threaded WASM every GC pauses the audio-servicing thread: these explain FPS dips.
		obj.Set("gcPauses", int(s.GCPauses))
		obj.Set("gcCycles", int(s.GCCycles))
		obj.Set("gcForced", int(s.GCForced))
		obj.Set("gcPauseP50MS", s.GCPauseP50MS)
		obj.Set("gcPauseP99MS", s.GCPauseP99MS)
		obj.Set("gcPauseMaxMS", s.GCPauseMaxMS)
		obj.Set("gcHeapHighKB", int(s.GCHeapHighKB))
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
		// What Go holds; audio.js reports its side through audioGraphStats().
		obj.Set("mixerChannels", len(audio.InstrumentChannelIDs()))
		obj.Set("registeredInstruments", len(audio.Instruments()))
		obj.Set("liveImages", g.snapshotLiveImages())
		obj.Set("imagesAllocatedTotal", float64(MetricImagesAllocatedTotal()))
		obj.Set("drawCalls", float64(MetricDrawCallsTotal()))
		obj.Set("imagesByTag", imagesByTagSnapshot())
		// Nested so readers of the flat fields see no new keys.
		obj.Set("threeStage", threeStageLatencyJSObject(s.SchedMetrics))
		return obj
	}))
	js.Global().Set("resetPerfStats", jsFn(func(args jsArgs) any {
		g.perf.reset()
		g.gcMetrics.reset()
		return nil
	}))
	// getThreeStageLatency() -> metrics | null: Go's view of all three stages (audio.js sees only Stage C).
	js.Global().Set("getThreeStageLatency", jsFn(func(args jsArgs) any {
		s := g.PerfSnapshot()
		o := threeStageLatencyJSObject(s.SchedMetrics)
		if !o.IsNull() {
			// The lane commits from the sequencer pass itself, so Stage B stays empty.
			o.Set("lane", g.audioLaneOn())
		}
		return o
	}))
	js.Global().Set("resetThreeStageLatency", jsFn(func(args jsArgs) any {
		g.schedMetrics.Reset()
		return nil
	}))

	// forceGameTick(x, y): one Update with the mouse there, for headless Chromium where rAF rarely fires.
	js.Global().Set("forceGameTick", jsFn(func(args jsArgs) any {
		mx, my := 0, 0
		mouseLeft := false
		if args.Len() >= 2 && !args.At(0).IsUndefined() {
			mx = args.Int(0)
			my = args.Int(1)
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

	// forceDrag(x0,y0,x1,y1[,steps]): a REAL left-button drag through Update, since drags do not register under
	// throttled headless rAF.
	js.Global().Set("forceDrag", jsFn(func(args jsArgs) any {
		if args.Len() < 4 {
			return nil
		}
		x0, y0 := args.Int(0), args.Int(1)
		x1, y1 := args.Int(2), args.Int(3)
		steps := 8
		if args.Len() >= 5 && !args.At(4).IsUndefined() {
			steps = args.Int(4)
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
