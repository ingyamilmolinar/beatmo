package ui

import (
	"fmt"
	"image"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// benchFmtMS formats a seconds value as milliseconds for human-readable
// log output, returning "N/A" for NaN/Inf.
func benchFmtMS(sec float64) string {
	if math.IsNaN(sec) || math.IsInf(sec, 0) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", sec*1000)
}

// benchJsonMS formats a seconds value as milliseconds for JSON output,
// returning "null" for NaN/Inf to produce valid JSON.
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
				g.logger.Debugf("[perf] ui: fps=%.1f upd_avg=%.3fms upd_max=%.3fms refresh=%.3fms draw_grid=%.3fms draw_drum=%.3fms rows_repaint=%d row_cache(full=%d shift=%d patch=%d) a_enq=%d a_deq=%d qlat_avg=%.3fms qlat_max=%.3fms acall_avg=%.3fms acall_max=%.3fms par_avg=%.3fms par_max=%.3fms par_n=%d live_images=%d images_alloc=%d images_by_tag=[%s]",
					s.FPSAvg, s.UpdateAvgMS, s.UpdateMaxMS, g.lastRefreshMS, g.lastDrawGridMS, g.lastDrawDrumMS, rowsRepaints, rowCacheFull, rowCacheShift, rowCachePatch, s.AudioEnq, s.AudioDeq, s.AudioQLatAvg, s.AudioQLatMax, s.AudioCallAvg, s.AudioCallMax, parAvg, parMax, parCount, g.snapshotLiveImages(), MetricImagesAllocatedTotal(), imagesByTagSnapshot())
				g.perf.reset()
				g.schedMetrics.Reset()
				g.resetParityPerf()
				g.perf.nextLog = now.Add(2 * time.Second)
			}
		}
	}()
	// Per-frame undo bracket: every recorded mutation produced by this frame's
	// single user gesture (a click that adds + auto-stitches, a drag release,
	// etc.) collapses into ONE atomic undo step. endGroup is a cheap no-op when
	// nothing was recorded (it never calls capture on an idle frame). This is the
	// safety net beneath the per-operation groups in game_graph_nodes.go.
	if g.undoManager != nil {
		g.undoManager.beginGroup("")
		defer g.undoManager.endGroup()
	}
	// ── Screenshot mode ──
	if g.screenshotReady() {
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
			g.schedMetrics.Reset()
			ResetImageMetrics()
			g.logger.Infof("[BENCH] Started: bpm=%d duration=%s", g.benchBPM, g.benchDuration)
			if g.benchRecord {
				g.startBenchRecording()
			}
		} else if g.benchStarted && time.Since(g.benchStart) >= g.benchDuration {
			// Duration elapsed → log stats, stop, exit cleanly
			s := g.PerfSnapshot()
			sm := s.SchedMetrics
			g.logger.Infof("[BENCH] Complete: frames=%d fps=%.1f upd=%.2f/%.2fms draw=%.2f/%.2fms audio_enq=%d audio_deq=%d qlat=%.2f/%.2fms sched(count=%d overdue=%d minLead=%sms lagP90=%sms lagP99=%sms) heap=%dKB goroutines=%d",
				s.Frames, s.FPSAvg,
				s.UpdateAvgMS, s.UpdateMaxMS,
				s.DrawAvgMS, s.DrawMaxMS,
				s.AudioEnq, s.AudioDeq,
				s.AudioQLatAvg, s.AudioQLatMax,
				sm.Count, sm.Overdue,
				benchFmtMS(sm.MinLead), benchFmtMS(sm.LagP90), benchFmtMS(sm.LagP99),
				s.HeapAllocKB, s.Goroutines)
			pAvg, pMax, pCount := g.parityScanStats()
			g.logger.Infof("[BENCH] Parity: scans=%d avg=%.3fms max=%.3fms", pCount, pAvg, pMax)
			// Machine-readable JSON line for scripts/bench-desktop.sh.
			// Uses benchJsonMS() to emit "null" for NaN values (valid JSON).
			jm := benchJsonMS
			g.logger.Debugf("[bench_json] {\"bpm\":%d,\"frames\":%d,\"fps\":%.2f,\"updateAvgMS\":%.3f,\"updateMaxMS\":%.3f,\"drawAvgMS\":%.3f,\"drawMaxMS\":%.3f,\"audioEnq\":%d,\"audioDeq\":%d,\"audioQLatAvgMS\":%.3f,\"audioQLatMaxMS\":%.3f,\"audioCallAvgMS\":%.3f,\"audioCallMaxMS\":%.3f,\"schedCount\":%d,\"schedOverdue\":%d,\"schedMinLeadMS\":%s,\"schedMaxLeadMS\":%s,\"schedAvgLeadMS\":%s,\"schedAvgLagMS\":%s,\"schedMaxLagMS\":%s,\"schedLagP90MS\":%s,\"schedLagP99MS\":%s,\"schedSmallLeadCount\":%d,\"heapAllocKB\":%d,\"heapSysKB\":%d,\"heapObjects\":%d,\"goroutines\":%d,\"parityScans\":%d,\"parityAvgMS\":%.3f,\"parityMaxMS\":%.3f,\"liveImages\":%d,\"imagesAllocatedTotal\":%d,\"imagesByTag\":\"%s\"}",
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
				pCount, pAvg, pMax,
				g.snapshotLiveImages(), MetricImagesAllocatedTotal(), imagesByTagSnapshot())
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
			g.logger.Debugf("[GAME] auto-disabling simple draw for interactive session")
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

	// Detect node parameter edits (e.g., probability changes) and rebase
	// prediction contexts at the current position to avoid phase drift.
	// This applies to the engine-backed predictor path used in WASM/desktop.
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
	// Process engine events without blocking. If playback is stopped,
	// drain any pending ticks without advancing the timeline so beat and
	// time counters freeze immediately when the user hits Stop.
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
	// splitter (resize only - input handled via dispatcher)
	// UpdateResize must run here (before the dispatcher) so the splitter's
	// winW/totalH are current for in-drag clamping. The drum bounds are
	// propagated *after* the dispatcher moves the divider — see SetBounds below.
	g.split.UpdateResize(g.winH, g.winW)

	// Decay popup-close guard so taps that closed a popup on a prior frame
	// don't create nodes underneath (touch-to-mouse / gesture dual processing).
	// Only decrement when no touch is active: globalTouchState.Update() hasn't
	// run yet at this point, so ActiveTouchCount() still reflects the previous
	// frame. This freezes the guard while the finger is down, preventing it
	// from expiring before the GestureTap fires on the frame the touch ends.
	// Desktop is unaffected (ActiveTouchCount is always 0).
	if g.sidebar.ClosedGuard() > 0 && globalTouchState.ActiveTouchCount() == 0 {
		g.sidebar.DecrementClosedGuard()
	}
	// Per-frame sidebar scroll momentum (must run even when cursor is away
	// from the sidebar so momentum decays after touch ends).
	if g.sidebar.IsOpen() {
		g.sidebar.UpdateScroll()
	}
	// Tie node-scoped UI (coordinate badge, open node menu) to node lifecycle:
	// drop anything pointing at a node that has left the graph, whatever the
	// cause (undo/redo restore, direct delete, import).
	g.pruneDanglingNodeRefs()

	// === TOUCH INPUT ===
	// Poll touch state FIRST (before any cursor reads) so that the
	// touch-to-mouse override is set for the rest of the frame.
	gesture := globalTouchState.Update()
	touchHandled := false

	// Inject a 2-frame tap for drum-area taps (where touch has already
	// ended by the time the gesture fires, so the override won't see it).
	// Skip if a multi-touch gesture just ended — the tap is spurious.
	if gesture != nil && gesture.Kind == GestureTap && !g.split.InGridPane(gesture.X, gesture.Y) && g.drum != nil && !globalTouchState.RecentMultiTouch() {
		injectTouchTap(gesture.X, gesture.Y)
		// Reset the tree's wasPressed so it sees the injected tap as a
		// fresh press. Without this, wasPressed carries over from the
		// touch-override hold (where touchDeadZone blocked dispatch),
		// and the tree never dispatches the tap injection.
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

	// Set frame-level touch override so cursorPosition() and
	// isMouseButtonPressed() return touch data for all existing handlers.
	updateTouchOverride()

	// === SINGLE INPUT ENTRY POINT ===
	// ALWAYS poll input - input state transitions cannot be skipped or state corrupts.
	// This is critical for WASM where fastPath is enabled by default.
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// Global undo/redo shortcuts are dispatched here, unconditionally, BEFORE any
	// editor/dispatcher/popup gating. Ctrl/Cmd+Z must work everywhere — including
	// while a popup or dropdown is open, during a drag capture, or with the mouse
	// held over a panel — not only when the cursor happens to be over the grid.
	g.handleUndoRedoKeys()
	g.handleGlobalShortcuts()

	// Handle gestures that are NOT mappable to mouse (multi-touch + grid tap/long-press).
	// Single-finger drag is now handled by the override → cam.HandleMouse / DrumView.Update.
	if gesture != nil {
		switch gesture.Kind {
		case GestureTap:
			if gesture.Y >= gridTopOffset() && g.split.InGridPane(gesture.X, gesture.Y) && !globalTouchState.RecentMultiTouch() {
				// Grid area tap — direct handler (node create/select).
				g.handleTapInGrid(gesture.X, gesture.Y)
				touchHandled = true
			}
			// Drum-area taps are injected above and handled via dispatcher.
		case GestureLongPress:
			g.handleTouchLongPress(gesture.X, gesture.Y)
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
		// GestureSingleFingerDrag is intentionally NOT handled here —
		// the touch override maps it to mouse, so cam.HandleMouse()
		// and DrumView.Update() process it automatically.
	}

	// Reset pinch baseline when fingers lift (not just when gesture is nil,
	// since CDP touch events arrive asynchronously and there may be frames
	// between touchMove events where gesture detection returns nil).
	if globalTouchState.ActiveTouchCount() < 2 {
		g.pinchBaseScale = 0
		g.pinchBaseGestureScale = 0
		if g.drum != nil {
			g.drum.EndTwoFingerPan()
		}
	}

	// Long-press popup intercept: when visible, it exclusively handles input
	// so no other dispatch runs.
	if g.longPressPopup {
		g.updateLongPressPopup(mx, my, left)
		g.leftPrev = left
	} else {
		// Set/decay splitter guard: while the drum view is capturing,
		// keep a 3-frame cooldown that prevents the splitter from
		// initiating a new drag (covers mobile touch flicker).
		if g.drum != nil && g.drum.Capturing() {
			g.split.guardFrames = 3
		} else if g.split.guardFrames > 0 {
			g.split.guardFrames--
		}

		// Rebuild handler list only when sidebar state changes.
		sidebarOpen := g.sidebar.IsOpen()
		if g.dispatcherDirty || sidebarOpen != g.lastDispatcherSidebarOpen {
			g.dispatcherDirty = false
			g.lastDispatcherSidebarOpen = sidebarOpen
			g.inputDispatcher.Clear()
			if sidebarOpen {
				g.inputDispatcher.Register(g.sidebar)
			}
			g.inputDispatcher.Register(g.split)
			if g.drum != nil {
				g.inputDispatcher.Register(g.drum)
			}
			g.inputDispatcher.Sort()
		}

		// Skip normal input dispatch if touch gesture was fully handled
		inputHandled := touchHandled
		if !touchHandled {
			// Single dispatch - returns true if any handler consumed
			inputHandled = g.inputDispatcher.Dispatch(mx, my, left)
		}

		// Grid-pane "?" help button: reuse the Button widget's click+anim. Driven
		// only on the mouse path (touch is handled in handleTapInGrid). Consuming
		// the press marks inputHandled so the grid editor doesn't also react, and
		// sets gridHelpCapturing so the DrumViewTree defers this press (below) —
		// otherwise the same press that opens the overlay is seen by the tree's
		// click-outside logic (the button is outside all tree hit areas) and
		// immediately closes it.
		g.gridHelpCapturing = false
		if !touchHandled && !Profile().IsMobile() && g.gridHelpBtn != nil {
			if g.gridHelpBtn.HandleInputResult(mx, my, left) == InputConsumed {
				inputHandled = true
				g.gridHelpCapturing = true
			}
		}

		// (Removed: the legacy BPM-box focus-nudge. The BPM readout is now a
		// display-only box; tapping it opens the shared ParamValueEditor via
		// bpmOpenAdapter through the tree dispatcher — it must never be focused.)
		// Only handle editor if input not consumed by dispatcher
		// The function has internal guards for blocking conditions (splitter, menus, bounds).
		if !inputHandled && !g.blocksAt(mx, my) {
			g.handleEditor()
		} else {
			g.leftPrev = left
		}
	}

	if fastPath {
		g.hover = nil
	} else {
		if my >= gridTopOffset() && !g.blocksAt(mx, my) {
			g.hover = g.nodeAtScreen(mx, my)
		} else {
			g.hover = nil
		}
	}

	// Propagate the (possibly just-dragged) splitter position to the drum pane
	// *after* the dispatcher has moved the divider this frame. The grid pane
	// reads the splitter live at draw time, so baking the drum bounds from an
	// earlier (pre-dispatch) splitter Y left the drum one frame behind — an
	// uncovered "dark band" trailing the divider during a fast resize drag.
	// SetBounds is gated on an actual rect change, so a non-resize frame no-ops;
	// the cascade still runs at most once per frame while dragging.
	g.drum.SetBounds(g.split.DrumRect(g.winW, g.winH))

	// Run drum view logic before evaluating panning so it can capture drags.
	prevPlaying := g.Playing()
	prevPaused := g.Paused()
	prevLen := g.drum.Length
	pendingSubdiv := 0
	// ── Concurrency: seqMu protects structural edits ──
	// Game.Update() holds seqMu while calling drum.Update(). Any callback
	// from DrumView that calls back into Game code requiring seqMu will
	// DEADLOCK — Go's sync.Mutex is NOT reentrant.
	//
	// Pattern: onImport queues data to pendingImportData; actual import
	// runs after seqMu.Unlock() (see below). lastTriggeredByRow is also
	// mutex-protected — never read/write it directly in tests; use the
	// thread-safe test helpers (setLastTriggeredForTest, etc.).
	g.seqMu.Lock()
	// Tell the drum view whether another component holds the dispatcher's
	// capture so it can avoid starting new interactions (e.g. touch scroll)
	// when the splitter or another handler owns the input.
	if g.drum != nil && g.inputDispatcher != nil {
		// gridHelpCapturing: while the grid "?" button holds the press, defer the
		// tree so its click-outside doesn't close the overlay the button just opened.
		g.drum.inputCapturedExternally = g.inputDispatcher.HasCaptureOtherThan(g.drum) || g.gridHelpCapturing
	}
	// Must always process input to maintain correct state transitions.
	g.drum.Update()
	// Keep grid subdivisions in sync with the drum view selection even if the
	// callback was skipped (e.g., in test harnesses where layout rebuilt after
	// wiring). Apply only while stopped to honor runtime safety.
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
		if dr.index < len(g.nextBeatIdxs) {
			g.nextBeatIdxs = append(g.nextBeatIdxs[:dr.index], g.nextBeatIdxs[dr.index+1:]...)
		}
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

	// Drain notifications queued by hook subscribers running on the
	// hooks fan-out pool (e.g., audio.RecordStopPayload). DrumView's
	// notify funcs touch UI state, so we forward them on the game-thread
	// goroutine, mirroring the pendingImportData pattern.
	g.pendingNotifyMu.Lock()
	infos := g.pendingNotifyInfo
	errs := g.pendingNotifyError
	g.pendingNotifyInfo = nil
	g.pendingNotifyError = nil
	g.pendingNotifyMu.Unlock()
	for _, m := range infos {
		if g.drum != nil {
			g.drum.notifyInfo(m)
		}
	}
	for _, m := range errs {
		if g.drum != nil {
			g.drum.notifyError(m)
		}
	}

	// Drain externally queued actions (e.g. JS-driven openers via QueueAction).
	// Runs after seqMu.Unlock so handlers that touch UI/predictor state cannot
	// reenter the per-frame lock. Mirrors the pendingImportData pattern below.
	g.drainPendingActions()

	// Process pending import after seqMu is released to avoid recursive locking.
	// The import callback from drum.Update() queues data here; we process it now
	// that the lock is free.
	if g.pendingImportData != nil {
		data := g.pendingImportData
		g.pendingImportData = nil
		err := g.Import(data)
		// Notify DrumView of the result so it can show success/error notification
		if err != nil {
			g.logger.Errorf("[GAME] Import error: %v", err)
			if g.drum != nil {
				g.drum.notifyError(i18n.Tf(i18n.KeyNotifErrLoadJSON, err.Error()))
			}
		} else if g.drum != nil {
			g.drum.notifyInfo(i18n.T(i18n.KeyNotifImported))
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

	// Camera panning always runs - essential for grid interaction on all platforms.
	// Not guarded by fastPath since it's just mouse delta math (not expensive).
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	panOK := !g.linkDrag.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds) && !g.drum.Capturing() && !g.menuHit(mx, my) && !g.longPressPopup

	// Diagnostic: log why panOK is false for grid touches on mobile.
	// Throttled to once per 60 frames to avoid log spam.
	if !panOK && left && g.split.InGridPane(mx, my) && touchOverrideActive && g.frame%60 == 0 {
		g.logger.Debugf("[pan] panOK=false grid touch at (%d,%d) "+
			"linkDrag=%v splitDrag=%v shift=%v inDrum=%v capturing=%v menuHit=%v popup=%v",
			mx, my, g.linkDrag.active, g.split.dragging, shift,
			pt(mx, my, g.drum.Bounds), g.drum.Capturing(), g.menuHit(mx, my), g.longPressPopup)
		g.drum.logCapturingState()
	}

	// Dispatch wheel to registered handlers (sidebar, drumview) first.
	// If consumed, skip camera zoom so scrolling doesn't also zoom.
	wheelHandled := false
	if steps := wheelScrollSteps(); steps != 0 {
		wheelHandled = g.inputDispatcher.DispatchWheel(mx, my, steps)
	}

	// Handle wheel zoom with debug logs. Read the wheel delta here so we can
	// log it and then let Camera.HandleMouse process drag only.
	var drag bool
	if panOK && !wheelHandled {
		if dz := wheelZoomDelta(); dz != 0 {
			mx, my := cursorPosition()
			g.logger.Debugf("[zoom] wheel dz=%.4f at (%d,%d) scale=%.3f", dz, mx, my, g.cam.Scale)
			// Apply a stronger, anchored zoom immediately.
			g.zoomAtScreen(float64(mx), float64(my), dz*8.0)
			// Prevent a second tiny zoom this frame but keep panning disabled
			// only for this event; camera drag resumes when no wheel.
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
			// Ensure legacy tests that reference activePulse directly can
			// observe the current primary-row pulse.
			g.activePulse = g.activePulses[0]
		}
		// Allow tests to force segment completion by setting p.t=1 and
		// calling Update() without waiting for an engine tick.
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
		// After any pulse advancement, align to current timeline so UI
		// reflects playback precisely even under heavy UI work. Skip the very
		// first frame after resuming to avoid visual jumps.
		if !g.JustResumed() {
			g.syncUIToTime()
		}
		// Clear the one-frame resume guard.
		if g.JustResumed() {
			g.state.ClearJustResumed()
		}
	}

	// In unit tests, the background sequencer goroutine is disabled by default
	// to avoid racy concurrent writes to UI state. Drive scheduling here so
	// audio/parity counters still advance deterministically.
	if runningUnderGoTest() && !g.sequencerRunning && !g.sequencerStopped && g.Playing() {
		g.seqScheduleTime()
	}

	g.drainAndDecayHighlights()

	// drum view logic already run above

	if g.drum.PlayPressed() {
		if g.Playing() {
			g.logger.Debugf("[game] pause pressed")
			div := max1(g.grid.MaxDiv())
			g.nextBeatIdxs = append([]int(nil), startNext...)
			g.state.Pause(gamestate.PauseInput{
				Beats:      beatsAtFrameStart,
				GridDiv:    div,
				DisplayDiv: div,
			})
			g.audioGen.Add(1)
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
			// Treat any non-zero seek position as a resume-from-offset so the
			// counters re-anchor even when the paused flag was cleared (e.g.,
			// sequencing without prior Pause). This keeps displayBeat aligned
			// after manual scrubs before resuming.
			resumeFromOffset := (resumeKind == gamestate.ResumeKindResume) || g.elapsedBeats > 0
			if resumeFromOffset {
				for row := range g.drum.Rows {
					g.spawnPulseFromRow(row, g.elapsedBeats)
				}
				if g.activePulse == nil && len(g.activePulses) > 0 {
					g.activePulse = g.activePulses[0]
				}
				div := max1(g.grid.MaxDiv())
				beat := float64(g.elapsedBeats) / float64(div)
				g.state.SetBeatBase(beat)
				g.state.SetLastDisplayBeat(beat)
				g.state.SetLastBeat(beat)
				g.state.SetLastStep(g.elapsedBeats)
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
				g.state.SetPausedBeats(g.elapsedBeats)
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
			g.logger.Warnf("[GAME] Play pressed but no start node; ignoring")
			g.state.SetPlayingForTest(false)
			g.state.SetPausedForTest(false)
		}
	}
	if g.drum.StopPressed() {
		g.logger.Debugf("[game] stop pressed")
		g.state.Stop()
		g.audioGen.Add(1)
		g.setPrimaryStep(0)
		// Ensure any paused playback visuals (pulses/highlights) are cleared even
		// when the playing flag doesn't transition (paused -> stopped).
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
		// Reset playback-derived state so the next Play starts from a clean
		// baseline. pathChangeBeatByRow stores elapsedBeats at the moment a
		// mid-play edit happened; carrying that watermark across Stop makes
		// refreshDrumRow's safety-net commit loop skip every freshly-played
		// cell on replay (j < pathChangeBeat ⇒ continue), which leaves the
		// drum row slate all-grey and hides highlight follow-through.
		// lastTriggeredByRow holds per-row/per-node "did this fire last time"
		// state used by highlightVisual; stale entries from the previous run
		// can suppress the highlight fallback for nodes whose lastTriggered=false.
		// lastFiredNodeByRow / lastHLIdxByRow are cheap dedupe caches that
		// must also start fresh.
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
	// Handle record toggle
	if g.drum.RecordPressed() {
		if audio.IsRecording() {
			g.logger.Debugf("[game] record stop pressed")
			// StopRecording returns immediately; the actual file flush
			// + close + session.json write happens on the recording
			// lifecycle pool. The "Recording saved" toast fires from
			// the EventRecordStop subscriber wired in game_new.go.
			result, err := audio.StopRecording()
			g.drum.SetRecording(false)
			if err != nil {
				g.logger.Errorf("[game] recording error: %v", err)
				g.drum.notifyError(i18n.Tf(i18n.KeyNotifRecordingFailed, err.Error()))
			} else if result != nil {
				g.drum.notifyInfo(i18n.Tf(i18n.KeyNotifSavingRecording, result.SessionDir))
			}
		} else {
			g.logger.Debugf("[game] record start pressed")
			instruments := make([]audio.InstrumentMeta, 0, len(g.drum.Rows))
			for _, row := range g.drum.Rows {
				if row.Instrument != "" {
					instruments = append(instruments, audio.InstrumentMeta{
						ID:   row.Instrument,
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
				g.drum.notifyError(i18n.Tf(i18n.KeyNotifCannotStartRecording, err.Error()))
			} else {
				g.drum.SetRecording(true)
				// Auto-start playback if not already playing
				if !g.Playing() {
					g.drum.playPressed = true
				}
			}
		}
	}
	// Detect BPM changes from the UI and re-anchor the timebase so that
	// playback position remains continuous without a jump. We must compute
	// the current absolute beat using the previous BPM, then reset the base
	// to "now" so future scheduling with the new BPM continues seamlessly.
	prevUI := g.bpm
	newBPM := g.drum.BPM()
	if newBPM != prevUI && g.Playing() {
		g.logger.Debugf("[game] BPM change requested: %d -> %d", prevUI, newBPM)
		prev := prevUI
		if prev > 0 {
			// Compute elapsed seconds on the current timeline.
			var dtSec float64
			audioNow := audio.Now()
			if audioNow > 0 && g.state.AudioStart() > 0 {
				dtSec = audioNow - g.state.AudioStart()
			} else if !g.state.PlayStart().IsZero() {
				dtSec = time.Since(g.state.PlayStart()).Seconds()
			}
			// Absolute beat position at this instant using the previous BPM.
			beatsNow := g.state.BeatBase() + dtSec*float64(prev)/60.0
			// Re-anchor base to preserve continuity and reset the clock.
			g.state.SetBeatBase(beatsNow)
			g.state.SetPlayStart(time.Now())
			if audioNow > 0 {
				g.state.SetAudioStart(audioNow)
			}
			// Align sequencer counters with current next-beat indices to avoid
			// any catch-up burst the next time scheduling runs.
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
	// Defensive cleanup: certain UI operations/tests can leave pulses allocated
	// while transport is fully stopped. Clear them without touching transport.
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
	// Keep preview window fresh while playing so DrumView reflects the live
	// sequencer progression without lag after freezes/highlights.
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
	// If we just paused, re-assert the current visual highlight locations so
	// the marker does not jump forward within this frame due to earlier sync.
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
	// Decay transient-visuals suppression after all state updates so the
	// next frame resumes normal rendering.
	if g.quietFrames > 0 {
		g.quietFrames--
	}
	g.updateCursorShape()
	// Synchronously drive the sequencer once per frame. The background
	// sequencer goroutine cannot run while Update() executes (WASM is
	// single-threaded cooperative), so the goroutine-driven 4ms ticker is
	// starved across long frames and Stage A latency spikes into the
	// hundreds of milliseconds (see webaudio_three_stage_latency).
	// Driving here caps the worst-case sequencer-fire latency at one
	// frame. The 4ms ticker still runs and catches sub-frame slots
	// whenever goroutines actually get scheduled.
	//
	// Production-only: under go test the background sequencer goroutine is
	// never started and scheduling is driven deterministically by the
	// earlier in-frame drive (see the runningUnderGoTest() block above),
	// which runs BEFORE drainAndDecayHighlights so seqNextIdxs and the UI
	// highlight indices stay in lockstep. A second drive here would advance
	// seqNextIdxs past the highlights already drained this frame, breaking
	// that parity (TestHighlightAudioSync25ms).
	if g.Playing() && !runningUnderGoTest() {
		g.seqScheduleTime()
	}
	return nil
}

// updateCursorShape sets the mouse cursor to a resize arrow when hovering
// over a splitter pill handle, or restores the default cursor otherwise.
// Skipped on mobile where cursor shapes are irrelevant.
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
