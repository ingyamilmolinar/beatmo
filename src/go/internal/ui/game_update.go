package ui

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// bench-desktop.sh greps stdout for "[bench_json] "; a constant so a test can pin the key set.
const benchJSONFormat = `[bench_json] {"bpm":%d,"frames":%d,"fps":%.2f,"updateAvgMS":%.3f,"updateMaxMS":%.3f,"drawAvgMS":%.3f,"drawMaxMS":%.3f,"audioEnq":%d,"audioDeq":%d,"audioQLatAvgMS":%.3f,"audioQLatMaxMS":%.3f,"audioCallAvgMS":%.3f,"audioCallMaxMS":%.3f,"schedCount":%d,"schedOverdue":%d,"schedMinLeadMS":%s,"schedMaxLeadMS":%s,"schedAvgLeadMS":%s,"schedAvgLagMS":%s,"schedMaxLagMS":%s,"schedLagP90MS":%s,"schedLagP99MS":%s,"schedSmallLeadCount":%d,"heapAllocKB":%d,"heapSysKB":%d,"heapObjects":%d,"goroutines":%d,"gcCycles":%d,"gcForced":%d,"gcPauses":%d,"gcPauseP50MS":%.3f,"gcPauseP99MS":%.3f,"gcPauseMaxMS":%.3f,"gcHeapHighKB":%d,"parityScans":%d,"parityAvgMS":%.3f,"parityMaxMS":%.3f,"liveImages":%d,"imagesAllocatedTotal":%d,"imagesByTag":"%s","renderCacheHits":%d,"renderCacheMisses":%d,"renderDispatchRenders":%d,"renderPrewarmRenders":%d,"renderAvgMS":%.3f,"renderMaxMS":%.3f` +
	`,"renderStalePlays":%d,"warmSetSize":%d,"warmSkippedCached":%d,"warmMS":%.3f` +
	`,"stormScenario":%q,"stormPushes":%d}` + "\n"

func benchFmtMS(sec float64) string {
	if math.IsNaN(sec) || math.IsInf(sec, 0) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", sec*1000)
}

// "null" for NaN/Inf keeps the JSON valid.
func benchJsonMS(sec float64) string {
	if math.IsNaN(sec) || math.IsInf(sec, 0) {
		return "null"
	}
	return fmt.Sprintf("%.3f", sec*1000)
}

/* ─────────────── Update & tick ────────────────────────────────────────── */

func (g *Game) Update() error {
	t0 := time.Now()
	defer func() {
		dur := time.Since(t0)
		g.lastUpdateMS = float64(dur) / 1e6
		g.perf.onUpdate(dur)
		g.heapProbeTick()
		// Periodic perf log (opt-in: PERF_LOG=1)
		if perfLogEnabled {
			now := time.Now()
			if g.perf.nextLog.IsZero() {
				g.perf.nextLog = now.Add(2 * time.Second)
			}
			if now.After(g.perf.nextLog) {
				s := g.perf.snapshot()
				parAvg, parMax, parCount := g.parityScanStats()
				rowCacheFull := 0
				rowCacheShift := 0
				rowCachePatch := 0
				rowsRepaints := 0
				if g.drum != nil {
					rowCacheFull = g.drum.rowCacheFull
					rowCacheShift = g.drum.rowCacheShift
					rowCachePatch = g.drum.rowCachePatch
					rowsRepaints = g.drum.rowsRepaints
				}
				gc := g.gcMetrics.snapshot()
				g.logger.Debugf("[perf] ui: fps=%.1f upd_avg=%.3fms upd_max=%.3fms refresh=%.3fms draw_grid=%.3fms draw_drum=%.3fms rows_repaint=%d row_cache(full=%d shift=%d patch=%d) a_enq=%d a_deq=%d qlat_avg=%.3fms qlat_max=%.3fms acall_avg=%.3fms acall_max=%.3fms par_avg=%.3fms par_max=%.3fms par_n=%d gc(cyc=%d forced=%d pauseMax=%.3fms heapHigh=%dKB) live_images=%d images_alloc=%d images_by_tag=[%s]",
					s.FPSAvg, s.UpdateAvgMS, s.UpdateMaxMS, g.lastRefreshMS, g.lastDrawGridMS, g.lastDrawDrumMS, rowsRepaints, rowCacheFull, rowCacheShift, rowCachePatch, s.AudioEnq, s.AudioDeq, s.AudioQLatAvg, s.AudioQLatMax, s.AudioCallAvg, s.AudioCallMax, parAvg, parMax, parCount, gc.Cycles, gc.Forced, gc.PauseMaxMS, gc.HeapHighKB, g.snapshotLiveImages(), MetricImagesAllocatedTotal(), imagesByTagSnapshot())
				g.perf.reset()
				g.resetParityPerf()
				g.perf.nextLog = now.Add(2 * time.Second)
			}
		}
		// Timing-gate metrics window — always on, independent of PERF_LOG.
		g.maybeResetSchedMetricsWindow(time.Now())
	}()
	// Tick the transient error toast (grid-pane alert) once per frame.
	if g.drum != nil {
		g.drum.logsToast.tick()
	}
	g.syncHeldInstruments()
	g.checkWarmupSampleRate() // trip the gate if the observed sample rate rotated
	g.warmup.Tick(0)          // nil-safe; enforces the readiness deadline
	// Every mutation from this frame's one gesture collapses into ONE undo step; a no-op on idle
	// frames. The safety net under the per-operation groups.
	if g.undoManager != nil {
		g.undoManager.beginGroup("")
		defer g.undoManager.endGroup()
	}
	// ── Screenshot mode ──
	if g.screenshotReady() {
		return ebiten.Termination
	}
	// ── Showcase tour: feeds this frame's scripted input before anything reads it ──
	if g.tour != nil && g.tour.tick(g) {
		return ebiten.Termination
	}
	// ── Benchmark mode lifecycle ──
	if g.benchBPM > 0 {
		if !g.benchStarted && g.demoBuilt {
			// Demo ready → override BPM, start playback, begin timer
			g.drum.SetBPM(g.benchBPM)
			g.bpm = g.benchBPM
			g.SetPlaying(true)
			g.engine.Start()
			g.benchStarted = true
			g.benchStart = time.Now()
			g.perf.reset()
			g.gcMetrics.reset()
			g.schedMetrics.Reset()
			ResetImageMetrics()
			audio.ResetRenderLatencyMetrics() // exclude startup renders from the window
			g.logger.Infof("[bench] Started: bpm=%d duration=%s", g.benchBPM, g.benchDuration)
			if g.benchRecord {
				g.startBenchRecording()
			}
		} else if g.benchStarted && time.Since(g.benchStart) < g.benchDuration {
			// Opt-in; off leaves bench output unchanged.
			if g.benchStorm != benchStormOff && g.benchStormInst != "" {
				for _, m := range benchStormMutationsForTick(g.benchStorm, g.benchStormTick) {
					audio.SetInstrumentParam(g.benchStormInst, m.Param, m.Value)
					g.benchStormPushes++
				}
				g.benchStormTick++
			}
		} else if g.benchStarted && time.Since(g.benchStart) >= g.benchDuration {
			// Duration elapsed → log stats, stop, exit cleanly
			s := g.PerfSnapshot()
			sm := s.SchedMetrics
			g.logger.Infof("[bench] Complete: frames=%d fps=%.1f upd=%.2f/%.2fms draw=%.2f/%.2fms audio_enq=%d audio_deq=%d qlat=%.2f/%.2fms sched(count=%d overdue=%d minLead=%sms lagP90=%sms lagP99=%sms) heap=%dKB goroutines=%d",
				s.Frames, s.FPSAvg,
				s.UpdateAvgMS, s.UpdateMaxMS,
				s.DrawAvgMS, s.DrawMaxMS,
				s.AudioEnq, s.AudioDeq,
				s.AudioQLatAvg, s.AudioQLatMax,
				sm.Count, sm.Overdue,
				benchFmtMS(sm.MinLead), benchFmtMS(sm.LagP90), benchFmtMS(sm.LagP99),
				s.HeapAllocKB, s.Goroutines)
			g.logger.Infof("[bench] GC: cycles=%d forced=%d pauses=%d pauseP50=%.3fms pauseP99=%.3fms pauseMax=%.3fms heapHigh=%dKB",
				s.GCCycles, s.GCForced, s.GCPauses,
				s.GCPauseP50MS, s.GCPauseP99MS, s.GCPauseMaxMS, s.GCHeapHighKB)
			rl := audio.GetRenderLatencyMetrics()
			g.logger.Infof("[bench] RenderLatency: cacheHits=%d cacheMisses=%d dispatchRenders=%d prewarmRenders=%d stalePlays=%d renderAvg=%.2fms renderMax=%.2fms",
				rl.CacheHits, rl.CacheMisses, rl.DispatchRenders, rl.PrewarmRenders, rl.StalePlays,
				float64(rl.RenderAvgNS)/1e6, float64(rl.RenderMaxNS)/1e6)
			pAvg, pMax, pCount := g.parityScanStats()
			g.logger.Infof("[bench] Parity: scans=%d avg=%.3fms max=%.3fms", pCount, pAvg, pMax)
			jm := benchJsonMS
			// To stdout, not the logger: benches run at -log INFO and this tag is DEBUG by contract, so a
			// logger call would be suppressed.
			warmSize, warmSkipped, warmMS := 0, 0, 0.0
			if g.warmup != nil {
				warmSize, warmSkipped, warmMS = g.warmup.BenchStats()
			}
			fmt.Printf(benchJSONFormat,
				g.benchBPM, s.Frames, s.FPSAvg,
				s.UpdateAvgMS, s.UpdateMaxMS,
				s.DrawAvgMS, s.DrawMaxMS,
				s.AudioEnq, s.AudioDeq,
				s.AudioQLatAvg, s.AudioQLatMax,
				s.AudioCallAvg, s.AudioCallMax,
				sm.Count, sm.Overdue,
				jm(sm.MinLead), jm(sm.MaxLead),
				jm(sm.AvgLead), jm(sm.AvgLag),
				jm(sm.MaxLag), jm(sm.LagP90), jm(sm.LagP99),
				sm.SmallLeadCount,
				s.HeapAllocKB, s.HeapSysKB, s.HeapObjects, s.Goroutines,
				s.GCCycles, s.GCForced, s.GCPauses,
				s.GCPauseP50MS, s.GCPauseP99MS, s.GCPauseMaxMS, s.GCHeapHighKB,
				pCount, pAvg, pMax,
				g.snapshotLiveImages(), MetricImagesAllocatedTotal(), imagesByTagSnapshot(),
				rl.CacheHits, rl.CacheMisses, rl.DispatchRenders, rl.PrewarmRenders,
				float64(rl.RenderAvgNS)/1e6, float64(rl.RenderMaxNS)/1e6,
				rl.StalePlays, warmSize, warmSkipped, warmMS,
				benchStormName(g.benchStorm), g.benchStormPushes)
			g.SetPlaying(false)
			g.engine.Stop()
			if g.benchRecord {
				g.finishBenchRecording(s)
			}
			return ebiten.Termination
		}
	}

	// Snapshot state at frame start to support precise pause without visual drift.
	if g.simpleDrawAutoDisableFrames > 0 {
		g.simpleDrawAutoDisableFrames--
		if g.simpleDrawAutoDisableFrames == 0 && g.simpleDraw {
			g.logger.Debugf("[game] auto-disabling simple draw for interactive session")
			g.simpleDraw = false
		}
	}
	beatsAtFrameStart := g.elapsedBeats
	if cap(g.startNextBuf) < len(g.nextBeatIdxs) {
		g.startNextBuf = make([]int, len(g.nextBeatIdxs))
	} else {
		g.startNextBuf = g.startNextBuf[:len(g.nextBeatIdxs)]
	}
	copy(g.startNextBuf, g.nextBeatIdxs)
	startNext := g.startNextBuf
	fastPath := g.perfMode.FastPathEnabled()

	// Rebase at the current position after a node-param edit to avoid phase drift.
	if g.engine != nil && g.engine.Predictor != nil && g.paramsDirty {
		base := g.elapsedBeats
		if len(g.nextBeatIdxs) > 0 {
			min := g.nextBeatIdxs[0]
			for i := 1; i < len(g.nextBeatIdxs); i++ {
				if g.nextBeatIdxs[i] < min {
					min = g.nextBeatIdxs[i]
				}
			}
			if min > 0 {
				base = min
			}
		}
		if base < 0 {
			base = 0
		}
		g.engine.Predictor.RebaseAt(base)
		g.pathsDirty = true
		g.paramsDirty = false
	}
	// Stopped: drain ticks without advancing, so counters freeze the moment the user hits Stop.
	for {
		select {
		case evt := <-g.engine.Events:
			if g.Playing() {
				g.onTick(evt.Step)
			}
		default:
			goto eventsDone
		}
	}
eventsDone:
	// Build demo on first Update after scheduling to avoid blocking ctor/layout.
	if g.demoScheduled && !g.demoBuilt && !runningUnderGoTest() {
		g.buildDemo()
		g.demoScheduled = false
	}
	// Apply scope visibility once the DrumView is ready.
	if g.scopeOpen && g.drum != nil && !g.scopeApplied {
		g.drum.SetChainVisible(true)
		g.scopeApplied = true
	}
	// Before the dispatcher, so in-drag clamping sees current sizes; the drum bounds follow AFTER it
	// moves the divider.
	g.split.UpdateResize(g.winH, g.winW)

	// Frozen while a finger is down (ActiveTouchCount is last frame's here), so the guard cannot expire
	// before the GestureTap fires on the lift frame.
	if g.sidebar.ClosedGuard() > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.DecrementClosedGuard()
	}
	// Every frame, so momentum decays after the touch ends.
	if g.sidebar.IsOpen() {
		g.sidebar.UpdateScroll()
	}
	// Whatever removed the node (undo, delete, import).
	g.pruneDanglingNodeRefs()

	// FIRST, before any cursor read, so the touch-to-mouse override holds for the whole frame.
	gesture := globalTouchState.Update()
	touchHandled := false

	// Inject only when the real touch-down was NOT consumed: the injection replays a tap the dead zone
	// blocked, and replaying a consumed one double-processes it (a popup opens then closes, or a tap
	// outside one opens what was beneath). A dead-zone tap always injects.
	if gesture != nil && gesture.Kind == GestureTap && !g.split.InGridPane(gesture.X, gesture.Y) && g.drum != nil && !globalTouchState.RecentMultiTouch() &&
		(g.drum.tapInScrollDeadZone(gesture.X, gesture.Y) || !suppressClicksUntilRelease) && g.drum.tapGate.accept(gesture.X, gesture.Y) {
		injectTouchTap(gesture.X, gesture.Y)
		// Reset wasPressed, which the blocked hold left set, so the tree sees the injection as a press.
		if g.drum.tree != nil {
			g.drum.tree.wasPressed = false
		}
		if g.drum.audioTree != nil {
			g.drum.audioTree.wasPressed = false
		}
		if g.drum.rootTree != nil {
			g.drum.rootTree.ResetPressEdge()
		}
	}

	updateTouchOverride()

	// Clear the override so every handler sees a clean released state (a valid transition, never a
	// skip); rotating back simply stops blocking.
	blockLandscapeInput := g.landscapeUnsupported()
	if blockLandscapeInput {
		touchOverrideActive = false
	}

	// ALWAYS poll: input transitions cannot be skipped without corrupting state.
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// Unconditional and before all gating, so Ctrl/Cmd+Z works over popups, during drags and anywhere.
	g.handleUndoRedoKeys()
	g.handleGlobalShortcuts()

	// Before panOK below.
	g.updateGuideAim(mx, my, left)

	// Single-finger drag goes through the mouse override instead.
	if gesture != nil && !blockLandscapeInput {
		switch gesture.Kind {
		case GestureTap:
			if gesture.Y >= gridTopOffset() && g.split.InGridPane(gesture.X, gesture.Y) && !globalTouchState.RecentMultiTouch() {
				// Only when the canvas owns the gesture: a control that took the press owns it all, and the tap
				// arrives at the lift point, which a drifting finger can carry off the control.
				if !g.inputDispatcher.GestureOwnedByHandler() {
					g.handleTapInGrid(gesture.X, gesture.Y)
				}
				touchHandled = true
			}
			// Drum-area taps are injected above and handled via dispatcher.
		case GestureLongPress:
			// A long-press that began on an overlay control must not delete the node under it.
			if !g.inputDispatcher.GestureOwnedByHandler() {
				g.handleTouchLongPress(gesture.X, gesture.Y)
			}
			if g.drum != nil {
				g.drum.CancelAllDeferredTaps()
			}
			g.pendingClick = false
			touchHandled = true
		case GesturePinch:
			g.handleTouchPinch(gesture.CenterX, gesture.CenterY, gesture.Scale)
			touchHandled = true
		case GestureTwoFingerPan:
			g.handleTouchTwoFingerPan(gesture.DeltaX, gesture.DeltaY, gesture.CenterX, gesture.CenterY)
			touchHandled = true
		}
	}

	// On finger count, not nil gesture: CDP touches arrive asynchronously with nil frames between moves.
	if globalTouchState.ActiveTouchCount() < 2 {
		g.pinchBaseScale = 0
		g.pinchBaseGestureScale = 0
		if g.drum != nil {
			g.drum.EndTwoFingerPan()
		}
	}

	g.modes.Tick()

	if g.longPressPopup {
		g.updateLongPressPopup(mx, my, left)
		g.leftPrev = left
		g.inputDispatcher.SkipFrame(left)
	} else {
		// A 3-frame cooldown covers mobile touch flicker.
		if g.drum != nil && g.drum.Capturing() {
			g.split.guardFrames = 3
		} else if g.split.guardFrames > 0 {
			g.split.guardFrames--
		}

		// Rebuild handler list only when sidebar/group-menu state changes.
		sidebarOpen := g.sidebar.IsOpen()
		groupMenuOpen := g.groupMenu.IsOpen()
		if g.dispatcherDirty || sidebarOpen != g.lastDispatcherSidebarOpen || groupMenuOpen != g.lastDispatcherGroupMenuOpen {
			g.dispatcherDirty = false
			g.lastDispatcherSidebarOpen = sidebarOpen
			g.lastDispatcherGroupMenuOpen = groupMenuOpen
			g.inputDispatcher.Clear()
			// One opaque handler at the top z, so a press anywhere in the cluster beats the grid on every platform.
			if g.gridHelpBtn != nil {
				g.inputDispatcher.Register(gridCornerButtonsHandler{g: g})
			}
			// While a menu is open, a grid press outside it only closes it.
			g.inputDispatcher.Register(popupOutsidePressHandler{g: g})
			if sidebarOpen {
				g.inputDispatcher.Register(g.sidebar)
			}
			if groupMenuOpen {
				g.inputDispatcher.Register(g.groupMenu)
			}
			g.inputDispatcher.Register(g.split)
			if g.drum != nil {
				g.inputDispatcher.Register(g.drum)
			}
			g.inputDispatcher.Sort()
		}

		// Reset before dispatch so the flag reflects only this frame.
		g.gridHelpCapturing = false

		// Skip normal input dispatch if touch gesture was fully handled
		inputHandled := touchHandled
		if !touchHandled {
			// Single dispatch - returns true if any handler consumed
			inputHandled = g.inputDispatcher.Dispatch(mx, my, left)
		} else {
			g.inputDispatcher.SkipFrame(left)
		}

		// Only when the dispatcher did not consume the input; handleEditor guards the rest.
		if !inputHandled && !g.blocksAt(mx, my) {
			g.handleEditor()
		} else {
			// An overlay may consume a marquee's release frame, leaving it active forever (dead panning, then a
			// phantom group on the next click). Cancel only.
			if g.marquee.active {
				g.marquee = marqueeDrag{}
			}
			g.leftPrev = left
		}
	}

	// Every frame, so a menu closing for any reason resets its button.
	g.reconcileGridButtonToggles(left)

	// Desktop pointer only, never gated on the perf fast path, which is on by default in the browser.
	if !Profile().IsMobile() && my >= gridTopOffset() && !g.blocksAt(mx, my) {
		g.hover = g.nodeAtScreen(mx, my)
	} else {
		g.hover = nil
	}
	g.advanceNodeHoverGlow()

	// AFTER the dispatcher moved the divider, or the drum pane trails it by a frame (a dark band).
	// SetBounds no-ops without a rect change.
	g.drum.SetBounds(g.split.DrumRect(g.winW, g.winH))

	// Run drum view logic before evaluating panning so it can capture drags.
	prevPlaying := g.Playing()
	prevPaused := g.Paused()
	prevLen := g.drum.Length
	pendingSubdiv := 0
	// drum.Update runs under seqMu, which is not reentrant: a DrumView callback that needs it deadlocks.
	// So imports queue and run after Unlock.
	g.seqMu.Lock()
	// So the drum view starts nothing new while another handler owns the input.
	if g.drum != nil && g.inputDispatcher != nil {
		// While a grid button holds the press, defer the tree so it does not close the menu just opened.
		g.drum.inputCapturedExternally = g.inputDispatcher.HasCaptureOtherThan(g.drum) || g.gridHelpCapturing
	}
	// Must always process input to maintain correct state transitions.
	g.drum.Update()
	// Only while stopped; covers harnesses that rebuilt layout after wiring.
	if !g.Playing() && g.grid != nil && g.drum != nil {
		if units := g.drum.timelineUnitsPerBeat; units > 0 && units != g.grid.MaxDiv() {
			pendingSubdiv = units
		}
	}
	added := g.drum.ConsumeAddedRows()
	for _, idx := range added {
		g.pendingStartRow = idx
	}
	for _, idx := range g.drum.ConsumeOriginRequests() {
		g.pendingStartRow = idx
	}
	deleted := g.drum.ConsumeDeletedRows()
	needsBeatInfos := len(deleted) > 0 || len(added) > 0
	for _, dr := range deleted {
		if dr.origin != model.InvalidNodeID {
			if n := g.nodeByID(dr.origin); n != nil {
				g.deleteNodeInternal(n, false)
			} else {
				g.graph.RemoveNode(dr.origin)
				g.notifyPredictorNode(dr.origin)
			}
		}
		// onDrumRowRemoved already shifted every per-row store.
		if dr.index < len(g.nextIdxSticky) {
			g.nextIdxSticky = append(g.nextIdxSticky[:dr.index], g.nextIdxSticky[dr.index+1:]...)
		}
		if len(g.nextIdxSticky) != len(g.nextBeatIdxs) {
			sticky := make([]bool, len(g.nextBeatIdxs))
			copy(sticky, g.nextIdxSticky)
			g.nextIdxSticky = sticky
		}
		if dr.index < len(g.nextIdxSticky) {
			g.nextIdxSticky[dr.index] = true
		}
		if g.pendingStartRow == dr.index {
			g.pendingStartRow = -1
		} else if g.pendingStartRow > dr.index {
			g.pendingStartRow--
		}
		out := g.activePulses[:0]
		for _, p := range g.activePulses {
			if p.row != dr.index {
				if p.row > dr.index {
					p.row--
				}
				out = append(out, p)
			}
		}
		g.activePulses = out
		if g.activePulse != nil {
			if g.activePulse.row == dr.index {
				g.activePulse = nil
			} else if g.activePulse.row > dr.index {
				g.activePulse.row--
			}
		}
	}
	g.seqMu.Unlock()

	// Subscribers run on the hooks pool; notify touches UI state, so forward here.
	g.pendingNotifyMu.Lock()
	pend := g.pendingNotifs
	g.pendingNotifs = nil
	g.pendingNotifyMu.Unlock()
	for _, p := range pend {
		if g.drum == nil {
			continue
		}
		if p.isErr {
			g.drum.notifyErrorKey(p.key, p.args...)
		} else {
			g.drum.notifyInfoKey(p.key, p.args...)
		}
	}

	// After seqMu.Unlock, so handlers cannot re-enter the frame's lock.
	g.drainPendingActions()

	// After seqMu is released.
	if g.pendingImportData != nil {
		data := g.pendingImportData
		source := g.pendingImportSource
		g.pendingImportData = nil
		g.pendingImportSource = ""
		err := g.Import(data)
		// Notify DrumView of the result so it can show success/error notification
		if err != nil {
			g.logger.Errorf("[game] Import error: %v", err)
			if g.drum != nil {
				g.drum.notifyErrorKey(i18n.KeyNotifErrLoadJSON, err.Error())
			}
		} else if g.drum != nil {
			// The generic key when the source is unknown.
			if source != "" {
				g.drum.notifyInfoKey(i18n.KeyNotifImportedNamed, importSourceName(source),
					strconv.Itoa(len(g.graph.Nodes)), strconv.Itoa(len(g.drum.Rows)))
			} else {
				g.drum.notifyInfoKey(i18n.KeyNotifImported)
			}
			emitImport(len(data), len(g.graph.Nodes), len(g.drum.Rows))
		}
	}

	if pendingSubdiv > 0 {
		prev := 0
		if g.grid != nil {
			prev = g.grid.MaxDiv()
		}
		if err := g.SetSubdivisions(pendingSubdiv); err != nil && g.drum != nil && prev > 0 {
			g.drum.timelineUnitsPerBeat = prev
			if g.drum.subdivBtn() != nil {
				g.drum.subdivBtn().Text = fmt.Sprintf("\u00f7%d", prev)
			}
		}
	}
	if needsBeatInfos {
		g.updateBeatInfos()
	}

	// Process queued UI actions from button presses before panning/zoom.
	if len(g.uiQueue) > 0 {
		q := g.uiQueue
		g.uiQueue = nil
		for _, fn := range q {
			if fn != nil {
				fn()
			}
		}
	}

	// Always: it is cheap delta math.
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	// guideAim is deliberately absent: aiming is visual only, and a drag anywhere pans (gated only by
	// the camera's tap slop). Adding it would double the slop near nodes.
	panOK := !g.linkDrag.active && !g.marquee.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds) && !g.drum.Capturing() && !g.menuHit(mx, my) && !g.gridMenuDragging() && !g.longPressPopup && !g.modalOverlayActive() && !blockLandscapeInput

	// Throttled to once per 60 frames.
	if !panOK && left && g.split.InGridPane(mx, my) && touchOverrideActive && g.frame%60 == 0 {
		g.logger.Debugf("[pan] panOK=false grid touch at (%d,%d) "+
			"linkDrag=%v splitDrag=%v shift=%v inDrum=%v capturing=%v menuHit=%v popup=%v",
			mx, my, g.linkDrag.active, g.split.dragging, shift,
			pt(mx, my, g.drum.Bounds), g.drum.Capturing(), g.menuHit(mx, my), g.longPressPopup)
		g.drum.logCapturingState()
	}

	// A consumed wheel skips camera zoom.
	wheelHandled := false
	if steps := wheelScrollSteps(); steps != 0 {
		wheelHandled = g.inputDispatcher.DispatchWheel(mx, my, steps)
	}

	var drag bool
	if panOK && !wheelHandled {
		if dz := wheelZoomDelta(); dz != 0 {
			mx, my := cursorPosition()
			g.logger.Debugf("[zoom] wheel dz=%.4f at (%d,%d) scale=%.3f", dz, mx, my, g.cam.Scale)
			// Apply a stronger, anchored zoom immediately.
			g.zoomAtScreen(float64(mx), float64(my), dz*8.0)
			// No second tiny zoom this frame.
			drag = g.cam.HandleMouse(false)
		} else {
			drag = g.cam.HandleMouse(panOK)
		}
	} else if !panOK && !wheelHandled {
		if dz := wheelZoomDelta(); dz != 0 {
			g.logger.Debugf("[zoom] ignored wheel (panOK=false)")
		}
		drag = g.cam.HandleMouse(panOK)
	} else {
		drag = g.cam.HandleMouse(panOK)
	}
	g.camDragging = drag
	if left && drag {
		g.camDragged = true
	}
	if g.camGesture == nil {
		g.camGesture = &cameraGesture{emitPan: func(dx, dy float64) { emitCameraPan(dx, dy) }, emitZoom: func(f float64) { emitCameraZoom(f) }}
	}
	g.camGesture.observe(g.camDragging, g.cam.OffsetX, g.cam.OffsetY, g.cam.Scale)

	if !fastPath {
		// edge animation progress
		for i := range g.edges {
			if g.edges[i].t < 1 {
				g.edges[i].t += 0.05
				if g.edges[i].t >= 1 {
					g.edges[i].t = 1
					g.edges[i].pulse = 0
				}
			} else if g.edges[i].pulse >= 0 {
				g.edges[i].pulse += 0.05
				if g.edges[i].pulse > 1 {
					g.edges[i].pulse = -1
				}
			}
		}
	}
	if g.Playing() {
		g.frame++
		if g.activePulse == nil && len(g.activePulses) > 0 {
			// Some tests read activePulse directly.
			g.activePulse = g.activePulses[0]
		}
		// Tests force completion with p.t=1 and Update(), without an engine tick.
		for i := 0; i < len(g.activePulses); {
			p := g.activePulses[i]
			advanced := false
			for p.t >= 1 {
				prevIdx := p.lastIdx
				g.highlightDelete(makeBeatKey(p.row, prevIdx))
				if !g.advancePulse(p) {
					if p.row == 0 {
						g.activePulse = nil
					}
					g.activePulses = append(g.activePulses[:i], g.activePulses[i+1:]...)
					g.clearRowHighlights(p.row)
					advanced = true
					goto nextPulseForced
				}
				advanced = true
				// Loop again if multiple segments were forced complete.
			}
			if !advanced {
				i++
			}
			continue
		nextPulseForced:
			// Only increment index if we didn't remove this pulse.
			if i < len(g.activePulses) && g.activePulses[i] == p {
				i++
			}
		}
		if g.activePulse == nil && len(g.activePulses) > 0 {
			g.activePulse = g.activePulses[0]
		}
		// Not on the first frame after resuming, which would jump.
		if !g.JustResumed() {
			g.syncUIToTime()
		}
		// Clear the one-frame resume guard.
		if g.JustResumed() {
			g.state.ClearJustResumed()
		}
	}

	// Under go test the background sequencer is off, so drive scheduling here deterministically.
	if runningUnderGoTest() && !g.sequencerRunning && !g.sequencerStopped && g.Playing() {
		g.seqScheduleTime()
	}

	g.drainAndDecayHighlights()

	// drum view logic already run above

	if g.drum.PlayPressed() {
		if !g.playbackReady() {
			// Consume and ignore: the toast explains why and the button reads disabled. Never queue it: a play
			// that fires seconds later, unprompted, is worse.
			done, total := g.warmup.Progress()
			g.logger.Debugf("[warmup] play ignored while warming (%d/%d)", done, total)
		} else if g.Playing() {
			g.logger.Debugf("[game] pause pressed")
			div := max1(g.grid.MaxDiv())
			g.nextBeatIdxs = append([]int(nil), startNext...)
			g.state.Pause(gamestate.PauseInput{
				Beats:      beatsAtFrameStart,
				GridDiv:    div,
				DisplayDiv: div,
			})
			g.audioGen.Add(1)
			g.resetAudioLane() // pause: cancel every lane voice not started yet
			g.setPrimaryStep(beatsAtFrameStart)
			for {
				select {
				case <-g.audioCh:
				default:
					goto pauseDrained
				}
			}
		pauseDrained:
			g.clearParityState()
		} else if g.start != nil {
			g.logger.Debugf("[game] play pressed")
			audio.Resume()
			now := time.Now()
			audioNow := audio.Now()
			resumeKind := g.state.Resume(now, audioNow)
			g.resetHighlights()
			g.activePulses = nil
			g.activePulse = nil
			// A non-zero position resumes from offset even without a prior Pause, so displayBeat stays aligned
			// after a scrub.
			resumeFromOffset := (resumeKind == gamestate.ResumeKindResume) || g.elapsedBeats > 0
			if resumeFromOffset {
				for row := range g.drum.Rows {
					g.spawnPulseFromRow(row, g.elapsedBeats)
				}
				if g.activePulse == nil && len(g.activePulses) > 0 {
					g.activePulse = g.activePulses[0]
				}
				div := max1(g.grid.MaxDiv())
				g.state.SetPosition(g.elapsedBeats, div)
				if len(g.seqNextIdxs) != len(g.drum.Rows) {
					g.seqNextIdxs = make([]int, len(g.drum.Rows))
				}
				div2 := max1(g.grid.MaxDiv())
				bpm := g.state.AppliedBPM()
				if bpm <= 0 {
					bpm = g.bpm
				}
				target := g.elapsedBeats
				if bpm > 0 {
					target = int(math.Floor(g.state.BeatBase() * float64(div2)))
				}
				for row := range g.drum.Rows {
					next := g.elapsedBeats
					if row < len(g.nextBeatIdxs) {
						next = g.nextBeatIdxs[row]
					}
					t := target + 1
					if next > t {
						t = next
					}
					g.seqNextIdxs[row] = t
				}
				g.state.SetSeekFreezeFrames(0)
				g.lastFrame = time.Now()
				g.frozenUpToByRow = make([]int, len(g.drum.Rows))
				for i := range g.frozenUpToByRow {
					g.frozenUpToByRow[i] = -1
				}
			} else {
				g.state.SetLastProg(0)
				g.syncUIToTime()
			}
		} else {
			g.logger.Warnf("[game] Play pressed but no start node; ignoring")
			g.state.SetPlaying(false)
			g.state.SetPaused(false)
		}
	}
	if g.drum.StopPressed() {
		g.logger.Debugf("[game] stop pressed")
		g.state.Stop()
		g.audioGen.Add(1)
		g.resetAudioLane() // stop: cancel every lane voice not started yet
		g.setPrimaryStep(0)
		// Even when the playing flag does not transition (paused → stopped).
		g.activePulses = nil
		g.activePulse = nil
		g.resetHighlights()
		g.lastFrame = time.Time{}
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
		g.muteUntilByRow = make([]int, len(g.drum.Rows))
		g.frozenUpToByRow = nil
		g.reconciledUpToByRow = nil
		// Clear timeline entries so stale data doesn't persist across Stop → Play.
		for row := range g.drum.Rows {
			g.timelineClearRow(row)
		}
		g.playedGraveyard = nil
		// Clean baseline for the next Play: a stale pathChangeBeatByRow makes the commit loop skip every
		// replayed cell (all grey), and stale lastTriggered entries suppress highlights.
		for i := range g.pathChangeBeatByRow {
			g.pathChangeBeatByRow[i] = -1
		}
		g.triggerMu.Lock()
		for k := range g.lastTriggeredByRow {
			delete(g.lastTriggeredByRow, k)
		}
		g.triggerMu.Unlock()
		for i := range g.lastFiredNodeByRow {
			g.lastFiredNodeByRow[i] = model.InvalidNodeID
		}
		for i := range g.lastHLIdxByRow {
			g.lastHLIdxByRow[i] = -1
		}
		g.clearParityState()
		g.parityWatch = parityWatchDefault
		parityFatalEnabled.Store(true)
	}
	g.drainSeekRequest()
	// Handle record toggle
	if g.drum.RecordPressed() {
		if audio.IsRecording() {
			g.logger.Debugf("[game] record stop pressed")
			// Returns at once; the flush runs on the lifecycle pool and the toast comes from EventRecordStop.
			result, err := audio.StopRecording()
			g.drum.SetRecording(false)
			if err != nil {
				g.logger.Errorf("[game] recording error: %v", err)
				g.drum.notifyErrorKey(i18n.KeyNotifRecordingFailed, err.Error())
			} else if result != nil {
				g.drum.notifyInfoKey(i18n.KeyNotifSavingRecording, result.SessionDir)
			}
		} else {
			g.logger.Debugf("[game] record start pressed")
			instruments := make([]audio.InstrumentMeta, 0, len(g.drum.Rows))
			for _, row := range g.drum.Rows {
				if row.Instrument != "" {
					instruments = append(instruments, audio.InstrumentMeta{
						ID:   channel.Of(row.Instrument),
						Name: row.Name,
					})
				}
			}
			opts := audio.RecordingOptions{
				Format:      audio.FormatWAV24,
				Instruments: instruments,
				BPM:         g.drum.BPM(),
			}
			if err := audio.StartRecording(opts); err != nil {
				g.logger.Errorf("[game] start recording error: %v", err)
				g.drum.notifyErrorKey(i18n.KeyNotifCannotStartRecording, err.Error())
			} else {
				g.drum.SetRecording(true)
				// Auto-start playback if not already playing
				if !g.Playing() {
					g.drum.playPressed = true
				}
			}
		}
	}
	// Compute the position at the old BPM, then re-anchor to now, so playback continues without a jump.
	prevUI := g.bpm
	newBPM := g.drum.BPM()
	if newBPM != prevUI && g.Playing() {
		g.logger.Debugf("[game] BPM change requested: %d -> %d", prevUI, newBPM)
		prev := prevUI
		if prev > 0 {
			now := time.Now()
			audioNow := audio.Now()
			// Playing set without ever anchoring a clock: start it, or AnchorForBPMChange no-ops and PlayStart
			// stays zero, stalling syncUIToTime forever.
			if g.state.PlayStart().IsZero() && g.state.AudioStart() <= 0 {
				g.state.StartClock(now, audioNow)
			}
			// Re-anchor base to preserve continuity and reset the clock.
			g.state.AnchorForBPMChange(prev, now, audioNow)
			// So the next scheduling pass has no catch-up burst.
			if len(g.seqNextIdxs) != len(g.drum.Rows) {
				g.seqNextIdxs = make([]int, len(g.drum.Rows))
			}
			for row := range g.drum.Rows {
				next := 0
				if row < len(g.nextBeatIdxs) {
					next = g.nextBeatIdxs[row]
				}
				if next <= 0 {
					next = 0
				}
				g.seqNextIdxs[row] = next
			}
		}
	}
	g.bpm = newBPM
	if g.bpm != g.state.AppliedBPM() {
		g.logger.Debugf("[game] queue BPM %d", g.bpm)
		sendLatestBPM(g.bpmCh, g.bpm)
	}

	select {
	case applied := <-g.bpmAck:
		g.logger.Debugf("[game] BPM applied: %d", applied)
		// Update pulse speeds to reflect new BPM for test expectations.
		beatDuration := int64(60.0 / float64(applied) * ebitenTPS)
		for _, p := range g.activePulses {
			p.speed = float64(g.grid.MaxDiv()) / float64(beatDuration)
		}
		g.state.SetAppliedBPM(applied)
	default:
	}

	g.handlePlaybackTransition(prevPlaying, prevPaused)
	// Some operations and tests leave pulses while fully stopped.
	if !g.Playing() && !g.Paused() && (len(g.activePulses) > 0 || g.activePulse != nil) {
		g.activePulses = nil
		g.activePulse = nil
	}

	g.updateDrumTracking()
	offsetChanged := g.drum.OffsetChanged()
	needRefresh := false
	forceRefresh := g.perfMode.ConsumeForceRefresh()
	if offsetChanged {
		needRefresh = true
		// Offset shifts allow incremental reuse; avoid forcing full redraws.
		g.drum.markRowsShiftDirty()
	}
	// If a path changed (live edit), refresh the preview window immediately.
	pathsDirty := g.pathsDirty
	if pathsDirty {
		needRefresh = true
		g.pathsDirty = false
	}
	if g.Playing() {
		needRefresh = true
	}
	if prevLen != g.drum.Length {
		maxOffset := len(g.beatInfos) - g.drum.Length
		if maxOffset < 0 {
			maxOffset = 0
		}
		if g.drum.Offset > maxOffset {
			g.drum.Offset = maxOffset
		}
		needRefresh = true
		g.drum.markAllRowsDirty()
	}
	if forceRefresh {
		needRefresh = true
	}
	if fastPath && g.Playing() && !offsetChanged && !pathsDirty && prevLen == g.drum.Length && !forceRefresh {
		mod := perfFastPathRefreshModulo
		if mod < 1 {
			mod = 1
		}
		if mod > 1 && int(g.frame)%mod != 0 {
			needRefresh = false
		}
	}
	var refreshDur time.Duration
	if needRefresh {
		start := time.Now()
		g.refreshDrumRow()
		refreshDur = time.Since(start)
	}
	g.lastRefreshMS = float64(refreshDur) / 1e6
	// So the marker does not jump forward within this frame.
	if g.state.JustPaused() {
		// Freeze highlights per row to their last subdivision index.
		for row := range g.drum.Rows {
			idx := 0
			if row < len(g.nextBeatIdxs) {
				idx = g.nextBeatIdxs[row] - 1
			}
			if idx < 0 {
				idx = 0
			}
			// Visual-only highlight; clear other markers in this row.
			info := g.beatInfoAtRow(row, idx)
			duration := int64(60.0 / float64(max1(g.state.AppliedBPM())) * ebitenTPS)
			g.highlightVisual(row, idx, info, duration)
		}
		g.state.ClearJustPaused()
	}
	if g.state.JustResumed() {
		g.state.ClearJustResumed()
	}
	// After all state updates, so the next frame renders normally.
	if g.quietFrames > 0 {
		g.quietFrames--
	}
	g.updateCursorShape()
	// WASM is single-threaded, so the sequencer goroutine starves during long frames; driving it here
	// caps fire latency at one frame. Production only: under go test the earlier drive runs before the
	// highlight drain, and a second one would break their lockstep.
	if g.Playing() && !runningUnderGoTest() {
		g.seqScheduleTime()
	}
	return nil
}

func (g *Game) updateCursorShape() {
	if Profile().IsMobile() {
		return
	}
	mx, my := cursorPosition()
	cursor := image.Pt(mx, my)

	// Active drag → keep resize cursor for the drag axis.
	if g.split.dragging {
		if g.split.horizontal {
			setCursorShape(ebiten.CursorShapeNSResize)
		} else {
			setCursorShape(ebiten.CursorShapeEWResize)
		}
		return
	}
	if g.drum != nil && g.drum.layoutHandler != nil && g.drum.layoutHandler.dragging {
		if g.drum.layoutHandler.dragAxis == "col" {
			setCursorShape(ebiten.CursorShapeEWResize)
		} else {
			setCursorShape(ebiten.CursorShapeNSResize)
		}
		return
	}

	// Hover over pill handle → show resize cursor.
	handleExpand := -SpaceSM
	if cursor.In(g.split.HandleRect().Inset(handleExpand)) {
		if g.split.horizontal {
			setCursorShape(ebiten.CursorShapeNSResize)
		} else {
			setCursorShape(ebiten.CursorShapeEWResize)
		}
		return
	}
	if g.drum != nil && g.drum.layoutHoverIdx >= 0 {
		if g.drum.layoutHoverAxis == "col" {
			setCursorShape(ebiten.CursorShapeEWResize)
		} else {
			setCursorShape(ebiten.CursorShapeNSResize)
		}
		return
	}

	setCursorShape(ebiten.CursorShapeDefault)
}
