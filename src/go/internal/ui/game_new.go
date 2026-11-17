package ui

import (
	"context"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/graphruntime"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

/* ───────────────────── constructor & layout ─────────────────── */

func New(logger *game_log.Logger) *Game {
	// Reset global button press guard for new instances to avoid test leakage.
	suppressClicksUntilRelease = false
	if logger != nil && RuntimeProf().ForceInfoLog && logger.Level() < game_log.LevelInfo {
		logger.SetLevel(game_log.LevelInfo)
	}
	eng := engine.New(logger)
	eng.Predictor.SetWindowCap(RuntimeProf().PredictorWindowCap)
	g := &Game{
		cam:                   NewCamera(),
		inputDispatcher:       NewInputDispatcher(),
		dispatcherDirty:       true,
		logger:                logger,
		graph:                 eng.Graph,
		graphRuntime:          graphruntime.NewRuntime(eng.Graph),
		state:                 gamestate.New(120),
		engine:                eng,
		engineProgress:        eng.Progress,
		split:                 NewSplitter(720), // real height set in Layout below
		highlightedBeats:      make(map[int]int64),
		hlDropped:             make(map[int]int64),
		bpm:                   120, // Default BPM
		nodeRows:              make(map[model.NodeID]int),
		nodesByID:             make(map[model.NodeID]*uiNode),
		activePulses:          []*pulse{},
		pendingStartRow:       -1,
		grid:                  NewGrid(DefaultGridStep),
		audioCh:               make(chan soundReq, 128),
		audioQuit:             make(chan struct{}),
		bpmCh:                 make(chan int, 1),
		bpmAck:                make(chan int, 1),
		playFn:                nil,
		nodeAnim:              make(map[model.NodeID]float64),
		lastNodeHL:            map[model.NodeID]bool{},
		nodeHLUntil:           map[model.NodeID]highlightWindow{},
		importPrevParityFatal: true,
		importPrevParityWatch: parityWatchDefault,
	}
	// playbackSlate owns the scheduling/beat-path/timeline/trigger field
	// cluster (see playback_slate.go); set as statements because promoted
	// (embedded) fields can't be keyed in a composite literal, and the
	// struct carries atomics that must never be copied.
	g.g = g
	g.timeline = timeline.NewService()
	g.beatInfos = []model.BeatInfo{}
	g.drumBeatInfos = []model.BeatInfo{}
	g.beatInfosByRow = [][]model.BeatInfo{}
	g.isLoopByRow = []bool{}
	g.loopStartByRow = []int{}
	g.loopLenByRow = []int{}
	g.originIdxsByRow = [][]int{}
	g.nextOriginIdxByRow = []int{}
	g.nextBeatIdxs = []int{}
	g.lastHLIdxByRow = nil
	g.hlCh = make(chan struct {
		row, idx int
		info     model.BeatInfo
	}, 64)
	g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.lastEvalIdxByRowNode = make(map[int]map[model.NodeID]int)
	g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.lastTriggeredByRow = make(map[int]map[model.NodeID]bool)
	g.lastFiredNodeByRow = []model.NodeID{}
	g.muteUntilByRow = []int{}
	g.loopCountByRow = []int{}
	g.lastStepsOffset = -1
	g.lastCellTypesOffset = -1
	// g.frame only increments while Playing() (see game_update.go), so it can
	// stay at its zero value for many paused draws; seed the memo frame to -1
	// (never equal to g.frame) so the very first guideTargetCached call always
	// misses instead of matching a coincidental zero/zero.
	g.guideMemoFrame = -1
	// Parity diagnostics state lives in the embedded parityTracker; set as
	// statements because promoted (embedded) fields can't be keyed in a
	// composite literal.
	g.parityRing = newMismatchRing(256)
	g.parityStreak = make(map[string]int)
	g.parityWatch.Store(parityWatchDefault)
	g.parityTimingWatch = parityTimingWatchDefault
	g.parityAudioMaxIdx = []int{}
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.parityScanEvery = parityScanEveryFrames
	g.parityScanStride = parityScanStride
	g.timingTraces = make(map[int64]timingTrace)
	g.emissionReportBuf = make([]audio.EmitReport, 128)
	g.sidebar = NewNodeSidebar(g)
	g.modes.Register(sidebarModeObserver{g: g})
	g.modes.Register(breakConfirmModeObserver{g: g})
	g.groupMenu = NewGroupMenu(g)
	g.perfMode.SetFastPath(defaultPerfFastPath)
	if runningUnderGoTest() {
		g.parityScanEvery = 1
		g.parityScanStride = 1
	}
	if g.parityScanEvery < 1 {
		g.parityScanEvery = 1
	}
	if g.parityScanStride < 1 {
		g.parityScanStride = 1
	}
	g.parityScanLastFrame = -int64(g.parityScanEvery)
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BEATMO_ROW_SNAPSHOTS"))); v == "1" || v == "true" {
		g.rowSnapshotMode = true
	}

	// Defer embedded WAV autoload; we'll schedule it after the first Update so
	// the demo instrument picks prefer built-ins and the UI becomes responsive.

	// bottom drum-machine view
	audio.InitDefaultCatalog()
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)
	// Weak back-reference so EQCallbacks closures (e.g. BeatGridFrac) can
	// reach Game state. Must be set before any Draw that resolves the
	// callback — every call site in drumview_ctor.go guards on nil.
	g.drum.game = g
	// Re-localize the UI on a language switch: invalidate text-baking caches so
	// labels re-measure at the new locale's widths. Stored cancel lets tests
	// drop the listener (the production Game is a process-lifetime singleton).
	g.i18nCancel = i18n.OnChange(g.drum.OnLocaleChanged)
	// Runtime theme: invalidate every color-baking cache after ApplyTheme so
	// the next frame repaints in the new palette (game_theme.go).
	g.themeCancel = OnThemeChange(g.OnThemeChanged)
	themeWarnf = logger.Warnf
	// Keep the browser landscape-block overlay's text in the active language
	// (no-op off the browser). Visibility is pure CSS (orientation media query).
	registerLandscapeNoticeLocaleSync()
	// Grid-pane settings gear (desktop AND mobile): opens the same settings
	// overlay as the "?"/"/" key. Drawn in the grid pane's top-right corner, NOT
	// in the transport bar. Reuses the shared Button widget.
	g.gridHelpBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleSettingsOverlay)
	g.gridHelpBtn.Icon = string(IconSettings)
	// Folder button, one gap left of the gear: opens the file menu (Upload /
	// Import / Export / Load Template). Both platforms.
	g.gridFileBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleFileMenu)
	g.gridFileBtn.Icon = string(IconFolder)
	// Logs button, one gap left of the folder button: opens the Logs Navigation
	// Panel (event stream + notifications). Both platforms.
	g.gridLogsBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleLogsPanel)
	g.gridLogsBtn.Icon = string(IconLogs)
	// Theme button, one gap left of the logs button: opens the Theme editor
	// (runtime UI color scheme). Both platforms.
	g.gridThemeBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleThemeOverlay)
	g.gridThemeBtn.Icon = string(IconPalette)
	// All four are rows of gridCornerButtons (grid_corner_buttons.go): input is
	// routed through ONE opaque cluster handler in the grid pane's
	// inputDispatcher (z=gridCornerButtonsInputZ). A button fires OnClick once
	// per press edge; the cluster owns the whole gesture, so the release-time
	// GestureTap never reaches the grid beneath (no double-fire, no stray node).
	g.undoManager = NewUndoManager(g.undoCapture, g.undoRestore)
	// The open circuit's instrument documents (Reset's target) are session
	// state beside the document: an undo across a circuit open restores them.
	g.undoManager.SetCircuitState(
		func() any { return audio.CircuitSnapshot() },
		func(v any) {
			s, _ := v.(audio.CircuitState)
			audio.RestoreCircuit(s)
		})
	registerUndoObserver(g.undoManager)
	// Ensure the timeline header interprets offsets/length in beats while we
	// track them internally in subdivision steps. Make the initial drum
	// window span at least four full beats so the default view shows a
	// meaningful portion of the circuit before any demo/import tweaks it.
	units := g.grid.MaxDiv()
	if units <= 0 {
		units = 1
	}
	g.drum.timelineUnitsPerBeat = units
	minLen := 4 * units
	if g.drum.Length < minLen {
		g.drum.SetLength(minLen)
	}
	// wire import handler - queue data for processing after seqMu is released
	// to avoid recursive locking (drum.Update is called while holding seqMu)
	g.drum.onImport = func(data []byte, source string) error {
		g.pendingImportData = data
		g.pendingImportSource = source
		// User-initiated load (template row / file picker): make it undoable so
		// a mistap can't silently destroy the current circuit. Demo/startup/JS
		// loads call g.Import directly and keep the clear-history behavior.
		g.importAsUndoStep = true
		return nil // actual result will be notified via pendingImportCB
	}
	g.drum.onImportDialogStart = g.startImportDialog
	g.drum.onImportDialogEnd = g.endImportDialog
	// Parity coordination: DrumView mutations route through this hook so the
	// parity generation, grace window, and buffer hygiene are kept in sync
	// with state changes. The hook is reachable under seqMu (DrumView.Update
	// runs while Game holds seqMu), so it must not re-acquire the lock.
	g.drum.onStructuralMutation = func(reason string) {
		g.bumpParityGen(reason, structuralMutationOptions{})
	}
	g.drum.onRowRemoved = g.onDrumRowRemoved

	// Subscribe to recording lifecycle hook events. Subscribers run on
	// the hooks fan-out pool (off the UI thread); we queue notifications
	// to be drained by Update so DrumView state is only touched on the
	// game-thread goroutine.
	// The bus is process-global: keep the unsubscribe so Close releases the
	// closure (and with it the whole Game) — a test suite that builds
	// hundreds of Games otherwise retains every one of them.
	g.recordStopUnsub = hooks.Subscribe(hooks.EventRecordStop, func(e hooks.Event) {
		p, ok := e.Payload.(audio.RecordStopPayload)
		if !ok {
			return
		}
		g.pendingNotifyMu.Lock()
		defer g.pendingNotifyMu.Unlock()
		if p.Err != nil {
			g.pendingNotifs = append(g.pendingNotifs,
				pendingNotif{key: i18n.KeyNotifRecordingSaveFailed, args: []string{p.Err.Error()}, isErr: true})
		} else {
			g.pendingNotifs = append(g.pendingNotifs,
				pendingNotif{key: i18n.KeyNotifRecordingSaved, args: []string{strconv.FormatInt(p.Drops, 10), p.Dir}})
		}
	})

	// provide current MaxDiv for export JSON
	currentMaxDiv = func() int { return g.grid.MaxDiv() }
	// allow DrumView to request subdivision changes
	g.drum.onChangeSubdiv = g.validateSubdivisions
	if g.drum.subdivBtn() != nil {
		g.drum.subdivBtn().Text = fmt.Sprintf("\u00f7%d", g.grid.MaxDiv())
	}
	g.state.SetAppliedBPM(g.drum.BPM())
	g.prevBPM = g.state.AppliedBPM()
	// Bounded scheduler for future-timestamped playFn calls. One worker is
	// enough — the scheduler hands off to audio.PlayBatch via the override
	// fn passed to SetPlayFunc, so the worker just dispatches; queue 64
	// matches audioCh capacity (~64ms tolerance at 1kHz dispatch). Mirror
	// hooks.fanout's pattern: prefer the global registry, fall back to a
	// private pool if the budget is already claimed by other subsystems
	// so the game still boots on resource-constrained CI.
	timerOpts := async.Options{MaxConcurrent: 1, QueueSize: 64, Name: "audio.timers"}
	timerPool, terr := async.DefaultRegistry().Get("audio.timers", timerOpts)
	if terr != nil {
		timerPool = async.NewPool(context.Background(), timerOpts)
	}
	g.audioScheduler = async.NewScheduler(timerPool)
	g.voicePrewarmer = newVoicePrewarmer(g)
	// Playback readiness gate: warms every voice the loaded circuit can
	// trigger after a wholesale cache invalidation (import / sample-rate
	// change) before Play unlocks. g.drum already exists at this point, so
	// wireWarmupNotifications can bind the progress toast immediately.
	g.warmup = newWarmupController(g, newWarmBackend(g))
	g.wireWarmupNotifications()
	// The audio lane's channels exist BEFORE the loops that select on them
	// (audioLoop: laneWake; sequencerLoop: laneKick) — initAudioLane runs later.
	g.laneWake = make(chan struct{}, 1)
	g.laneKick = make(chan struct{}, 1)
	g.bgWG.Add(2)
	go g.audioLoop()
	go g.bpmLoop()
	// Subscribe to engine ticks for scheduler-driven audio sequencing.
	g.seqCh = eng.Subscribe()
	g.seqQuit = make(chan struct{})
	// Under tests we drive scheduling from Update() to keep state deterministic
	// (no concurrent writes to UI state). Desktop/WASM runs use the background
	// sequencer goroutine.
	startSequencer := !runningUnderGoTest()
	if startSequencer {
		go g.sequencerLoop()
	}
	g.sequencerRunning = startSequencer
	g.timingTestMode = (os.Getenv("BPM_TIMING_TEST") == "1")
	// Set this Game as the active BPM owner so previous instances stop
	// applying audio.SetBPM in background loops during tests.
	bpmOwner.Store(g)
	// Enable detailed node draw logs via environment variable for diagnostics.
	// Enable draw logs when either DEBUG_DRAW_NODES or DEBUG_GEOM is set so
	// a single env var (DEBUG_GEOM=1) is enough for real-time geometry debug.
	if os.Getenv("DEBUG_DRAW_NODES") == "1" || os.Getenv("DEBUG_GEOM") == "1" {
		g.logDrawNodes = true
	}
	g.initJS()
	// Keep prediction buffer ahead in background with low priority. This runs
	// independent of UI rendering and coalesces demand using a target-horizon
	// function that considers the visible window, current next-beat indices,
	// and grid lookahead.
	if g.engine != nil && g.engine.Predictor != nil {
		targetFn := func() int {
			look := 32
			if g.grid != nil {
				look = g.grid.MaxDiv() * 16
			}
			// Back off lookahead when draw is heavy to reduce background pressure.
			if predictorPerfThrottleEnabled {
				s := g.perf.snapshot()
				if s.DrawAvgMS > 18.0 {
					// Under heavy draw pressure, avoid background expansion.
					look = 0
				} else if s.DrawAvgMS > 14.0 {
					look /= 4
				} else if s.DrawAvgMS > 10.0 {
					look /= 2
				}
			}
			need := g.drum.Offset + g.drum.Length
			if len(g.nextBeatIdxs) > 0 {
				for _, v := range g.nextBeatIdxs {
					if v+look > need {
						need = v + look
					}
				}
			}
			if need < g.drum.Length {
				need = g.drum.Length
			}
			return need + look
		}
		if runningUnderGoTest() {
			g.engine.Predictor.SetBackgroundTarget(targetFn)
			g.predictorBackgroundStopped = true
		} else {
			g.engine.Predictor.StartBackground(targetFn)
		}
	}
	// Initialize perf counters.
	g.perf.reset()

	// Default to simplified draw on web builds for better browser perf.
	g.simpleDraw = RuntimeProf().SimpleDrawDefault
	g.simpleDrawAutoDisableFrames = RuntimeProf().SimpleDrawAutoDisableFramesDefault

	// Browser uses a longer audio lookahead (~40 ms) to absorb Go↔JS jitter;
	// desktop's dedicated 1ms sequencer goroutine needs less buffer (20 ms).
	// Combined with runtimeAudioLookahead() (+30ms dynamic, 60ms cap), this
	// eliminates the zero-tolerance scheduling that caused overdue audio.
	g.audioLookaheadSec = RuntimeProf().AudioLookaheadSec
	g.initAudioLane()

	// Defer demo construction to the first layout/update to avoid blocking
	// constructor time. Layout will build it once when not under tests.

	// default cache pads (pixels)
	g.edgeCachePad = RuntimeProf().EdgeCachePad
	g.gridCachePad = RuntimeProf().GridCachePad
	g.graph.SetNodeChangedHook(func(id model.NodeID) {
		g.laneGraphVersion.Add(1) // audio lane fingerprint (audio_lane_source.go)
		g.cacheNode(id)
		g.paramsDirty = true
		if g.importing {
			g.pathsDirty = true
			return
		}
		row := g.rowIndexForNode(id)
		g.resetLogicStateForRow(row)
		g.notifyPredictorNode(id)
		g.pathsDirty = true
		if g.Playing() {
			// A node-param change (volume/pitch/logic, or an edge add/remove that
			// rewrites a node's input/output lists) updates predictor truth
			// (notifyPredictorNode, above) without necessarily changing path
			// topology, so updateBeatInfos's pathsChanged invalidation may not run.
			// Drop in-flight audio for the AFFECTED ROW only (per-row structural
			// generation) AND advance the parity generation (+grace) so the scan's
			// parityGen filter excludes any decision/audio recorded under the
			// pre-change params. Using the global audioGen here dropped in-flight
			// beats on OTHER rows too — an audible gap when a node/edge on one row
			// is touched. The explicit buffer wipe below is the memory-hygiene step
			// (redundant with the gen filter). Skip the path-dirty/freeze/clear side
			// effects — this hook owns paths-dirty itself.
			g.bumpRowStructGen(row)
			g.bumpParityGen("node-param-change", structuralMutationOptions{
				SkipPathsDirty:     true,
				SkipPathChangeMark: true,
				SkipBufferClear:    true,
			})
			g.parityMu.Lock()
			g.parityAudio = nil
			g.parityAudioMaxIdx = nil
			g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
			g.parityMu.Unlock()
			g.ClearParityMismatches()
		}
		g.perfMode.ForceRefresh()
		if g.drum != nil && row >= 0 {
			g.drum.markRowDirty(row)
		}
		if g.drum != nil && !g.Playing() {
			g.refreshDrumRow()
		}
	})
	g.graph.SetGroupChangedHook(func(model.GroupID) { g.onGroupsChanged() })
	g.refreshGroupIndex()
	g.rebuildNodeCache()
	g.storeSeqPathSnapshot()
	if runningUnderGoTest() {
		registerGameForTest(g)
	}
	g.gridTree = NewGridTree()
	g.registerGridTree()
	g.keyboardRouter = newKeyboardShortcutRouter(g)
	return g
}
