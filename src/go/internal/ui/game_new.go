package ui

import (
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
	// Statements: promoted fields cannot be keyed in a composite literal, and the struct holds atomics
	// that must not be copied.
	g.playbackSlate.g = g
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
	// g.frame stays 0 while paused, so -1 makes the first lookup miss.
	g.guideMemoFrame = -1
	// Promoted fields cannot be keyed in a composite literal.
	g.parityRing = newMismatchRing(256)
	g.parityStreak = make(map[string]int)
	g.parityWatch = parityWatchDefault
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

	// Deferred until after the first Update, so the demo prefers built-ins and the UI responds.

	// bottom drum-machine view
	audio.InitDefaultCatalog()
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)
	// Before any Draw that resolves a callback; every call site guards nil.
	g.drum.game = g
	// The cancel lets tests drop the listener.
	g.i18nCancel = i18n.OnChange(g.drum.OnLocaleChanged)
	g.themeCancel = OnThemeChange(g.OnThemeChanged)
	themeWarnf = logger.Warnf
	// Visibility is CSS; only the text needs the locale.
	registerLandscapeNoticeLocaleSync()
	g.gridHelpBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleSettingsOverlay)
	g.gridHelpBtn.Icon = string(IconSettings)
	g.gridFileBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleFileMenu)
	g.gridFileBtn.Icon = string(IconFolder)
	g.gridLogsBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleLogsPanel)
	g.gridLogsBtn.Icon = string(IconLogs)
	g.gridThemeBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleThemeOverlay)
	g.gridThemeBtn.Icon = string(IconPalette)
	// One opaque cluster handler routes all four buttons: OnClick once per press edge, and the cluster
	// owns the gesture, so the release-time tap never reaches the grid.
	g.undoManager = NewUndoManager(g.undoCapture, g.undoRestore)
	// Session state beside the document: an undo across a circuit open restores them.
	g.undoManager.SetCircuitState(
		func() any { return audio.CircuitSnapshot() },
		func(v any) {
			s, _ := v.(audio.CircuitState)
			audio.RestoreCircuit(s)
		})
	registerUndoObserver(g.undoManager)
	// At least four beats in the initial window.
	units := g.grid.MaxDiv()
	if units <= 0 {
		units = 1
	}
	g.drum.timelineUnitsPerBeat = units
	minLen := 4 * units
	if g.drum.Length < minLen {
		g.drum.SetLength(minLen)
	}
	// Queued until seqMu is released: drum.Update runs under it.
	g.drum.onImport = func(data []byte, source string) error {
		g.pendingImportData = data
		g.pendingImportSource = source
		// A user load is undoable, so a mistap cannot destroy the circuit; startup and JS loads clear history.
		g.importAsUndoStep = true
		return nil // actual result will be notified via pendingImportCB
	}
	g.drum.onImportDialogStart = g.startImportDialog
	g.drum.onImportDialogEnd = g.endImportDialog
	// Reachable under seqMu, so it must not re-acquire it.
	g.drum.onStructuralMutation = func(reason string) {
		g.bumpParityGen(reason, structuralMutationOptions{})
	}
	g.drum.onRowRemoved = g.onDrumRowRemoved

	// Subscribers run off the UI thread, so queue for Update. Keep the unsubscribe: the bus is
	// process-global and would otherwise retain every closed Game.
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
	// One worker that only dispatches; queue 64 matches audioCh. Registry first, private pool
	// fallback, so constrained CI still boots.
	timerOpts := async.Options{MaxConcurrent: 1, QueueSize: 64, Name: "audio.timers"}
	timerPool, terr := async.DefaultRegistry().Get("audio.timers", timerOpts)
	if terr != nil {
		timerPool = async.NewPool(nil, timerOpts)
	}
	g.audioScheduler = async.NewScheduler(timerPool)
	g.voicePrewarmer = newVoicePrewarmer(g)
	// Warms every voice the circuit can trigger after a cache invalidation, before Play unlocks.
	g.warmup = newWarmupController(g, newWarmBackend(g))
	g.wireWarmupNotifications()
	// Before the loops that select on them; initAudioLane runs later.
	g.laneWake = make(chan struct{}, 1)
	g.laneKick = make(chan struct{}, 1)
	g.bgWG.Add(2)
	go g.audioLoop()
	go g.bpmLoop()
	// Subscribe to engine ticks for scheduler-driven audio sequencing.
	g.seqCh = eng.Subscribe()
	g.seqQuit = make(chan struct{})
	// Tests drive scheduling from Update, deterministically.
	startSequencer := !runningUnderGoTest()
	if startSequencer {
		go g.sequencerLoop()
	}
	g.sequencerRunning = startSequencer
	g.timingTestMode = (os.Getenv("BPM_TIMING_TEST") == "1")
	// So older Games in a test process stop applying BPM.
	bpmOwner.Store(g)
	if os.Getenv("DEBUG_DRAW_NODES") == "1" || os.Getenv("DEBUG_GEOM") == "1" {
		g.logDrawNodes = true
	}
	g.initJS()
	// Low priority, independent of rendering; demand is coalesced through a target-horizon function.
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

	// The browser needs more cushion for Go↔JS jitter.
	g.audioLookaheadSec = RuntimeProf().AudioLookaheadSec
	g.initAudioLane()

	// Built on the first layout, not in the constructor.

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
			// Drop in-flight audio for the affected row only (its structural generation) and advance parity:
			// the global audioGen would drop beats on other rows too, an audible gap.
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
