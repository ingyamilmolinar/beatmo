package ui

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/graphruntime"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

/* ───────────────────── constructor & layout ─────────────────── */

func New(logger *game_log.Logger) *Game {
	// Reset global button press guard for new instances to avoid test leakage.
	suppressClicksUntilRelease = false
	if logger != nil && runtime.GOARCH == "wasm" && logger.Level() < game_log.LevelInfo {
		logger.SetLevel(game_log.LevelInfo)
	}
	eng := engine.New(logger)
	g := &Game{
		cam:             NewCamera(),
		inputDispatcher: NewInputDispatcher(),
		dispatcherDirty: true,
		logger:             logger,
		graph:              eng.Graph,
		graphRuntime:       graphruntime.NewRuntime(eng.Graph),
		state:              gamestate.New(120),
		timeline:           timeline.NewService(),
		engine:             eng,
		engineProgress:     eng.Progress,
		split:              NewSplitter(720), // real height set in Layout below
		highlightedBeats:   make(map[int]int64),
		bpm:                120, // Default BPM
		beatInfos:          []model.BeatInfo{},
		drumBeatInfos:      []model.BeatInfo{},
		beatInfosByRow:     [][]model.BeatInfo{},
		isLoopByRow:        []bool{},
		loopStartByRow:     []int{},
		loopLenByRow:       []int{},
		originIdxsByRow:    [][]int{},
		nextOriginIdxByRow: []int{},
		nextBeatIdxs:       []int{},
		nodeRows:           make(map[model.NodeID]int),
		nodesByID:          make(map[model.NodeID]*uiNode),
		activePulses:       []*pulse{},
		pendingStartRow:    -1,
		grid:               NewGrid(DefaultGridStep),
		audioCh:            make(chan soundReq, 128),
		bpmCh:              make(chan int, 1),
		bpmAck:             make(chan int, 1),
		playFn:             nil,
		lastHLIdxByRow:     nil,
		hlCh: make(chan struct {
			row, idx int
			info     model.BeatInfo
		}, 64),
		nodeTriggerCountsByRow:      make(map[int]map[model.NodeID]int),
		lastEvalIdxByRowNode:        make(map[int]map[model.NodeID]int),
		nodeLogicTriggerCountsByRow: make(map[int]map[model.NodeID]int),
		lastTriggeredByRow:          make(map[int]map[model.NodeID]bool),
		nodeAnim:                    make(map[model.NodeID]float64),
		lastFiredNodeByRow:          []model.NodeID{},
		muteUntilByRow:              []int{},
		loopCountByRow:              []int{},
		lastNodeHL:                  map[model.NodeID]bool{},
		nodeHLUntil:                 map[model.NodeID]highlightWindow{},
		lastStepsOffset:             -1,
		lastCellTypesOffset:         -1,
		parityRing:                  newMismatchRing(256),
		parityStreak:                make(map[string]int),
		parityWatch:                 parityWatchDefault,
		parityAudioMaxIdx:           []int{},
		paritySeqDecisions:          make(map[int]map[int]paritySeqDecision),
		parityScanEvery:             parityScanEveryFrames,
		parityScanStride:            parityScanStride,
		importPrevParityFatal:       true,
		importPrevParityWatch:       parityWatchDefault,
	}
	g.sidebar = NewNodeSidebar(g)
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
	g.drum.onImport = func(data []byte) error {
		g.pendingImportData = data
		return nil // actual result will be notified via pendingImportCB
	}
	g.drum.onImportDialogStart = g.startImportDialog
	g.drum.onImportDialogEnd = g.endImportDialog
	// provide current MaxDiv for export JSON
	currentMaxDiv = func() int { return g.grid.MaxDiv() }
	// allow DrumView to request subdivision changes
	g.drum.onChangeSubdiv = g.validateSubdivisions
	if g.drum.subdivBtn() != nil {
		g.drum.subdivBtn().Text = fmt.Sprintf("\u00f7%d", g.grid.MaxDiv())
	}
	g.state.SetAppliedBPM(g.drum.BPM())
	g.prevBPM = g.state.AppliedBPM()
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
	g.simpleDraw = simpleDrawDefault
	g.simpleDrawAutoDisableFrames = simpleDrawAutoDisableFramesDefault

	// Web-specific defaults: throttle Draw to ~30 FPS and schedule audio
	// slightly in the future to avoid main-thread jank affecting starts.
	if runtime.GOOS == "js" {
		g.drawMinInterval = 40 * time.Millisecond
		g.audioLookaheadSec = 0.04
	} else {
		// Desktop baseline lookahead: 20ms absorbs seqMu contention jitter.
		// Less than WASM's 40ms because the dedicated 1ms sequencer goroutine
		// needs less buffer. Combined with runtimeAudioLookahead() (+30ms
		// dynamic, 60ms cap), this eliminates the zero-tolerance scheduling
		// that caused overdue audio events.
		g.audioLookaheadSec = 0.02
	}

	// Defer demo construction to the first layout/update to avoid blocking
	// constructor time. Layout will build it once when not under tests.

	// default cache pads (pixels)
	g.edgeCachePad = defaultEdgeCachePad
	g.gridCachePad = defaultGridCachePad
	g.graph.SetNodeChangedHook(func(id model.NodeID) {
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
			g.audioGen.Add(1)
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
	g.rebuildNodeCache()
	g.storeSeqPathSnapshot()
	if runningUnderGoTest() {
		registerGameForTest(g)
	}
	return g
}
