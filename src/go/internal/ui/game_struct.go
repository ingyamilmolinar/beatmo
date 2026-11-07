package ui

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/graphruntime"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type Game struct {
	/* subsystems */
	tour                        tourDriver // scripted showcase tour (-tour); nil in normal runs (tour_hook.go)
	cam                         *Camera
	split                       *Splitter
	drum                        *DrumView
	i18nCancel                  func()        // cancels the i18n.OnChange listener registered in NewGame
	themeCancel                 func()        // cancels the OnThemeChange listener registered in NewGame
	recordStopUnsub             func()        // releases the hooks.EventRecordStop subscription taken in New
	userTheme                   theme.UserDoc // per-user profile theme + override flag (persisted via ThemeSaveSink)
	circuitTheme                theme.Doc     // the open circuit's own theme (exported in beatmo.json)
	gridHelpBtn                 *Button       // settings gear in grid pane top-right (opens settings overlay; both desktop and mobile)
	gridFileBtn                 *Button       // folder button just left of the gear (opens the file menu; both desktop and mobile)
	gridLogsBtn                 *Button       // logs button left of the folder button (opens the Logs Navigation Panel; both desktop and mobile)
	gridThemeBtn                *Button       // theme button left of the logs button (opens the Theme editor; both desktop and mobile)
	gridHelpCapturing           bool          // true while the grid gear/folder/logs button holds the press; defers the tree so its click-outside doesn't close the just-opened overlay
	undoManager                 *UndoManager
	keyboardRouter              *keyboardShortcutRouter // component-owned keyboard shortcut dispatch (keyboard_shortcuts.go)
	inputDispatcher             *InputDispatcher
	lastDispatcherSidebarOpen   bool
	lastDispatcherGroupMenuOpen bool
	dispatcherDirty             bool
	graph                       *model.Graph
	graphRuntime                *graphruntime.Runtime
	state                       *gamestate.State
	// So nodeLiveTiming does not rescan the circuit every frame.
	nodeTimingCache nodeTimingMemo
	engine          *engine.Engine
	engineProgress  func() float64
	logger          *game_log.Logger
	grid            *Grid
	audioCh         chan soundReq
	// Reused across iterations: on WASM linear memory only grows, so per-drain allocations raise the
	// peak. audioLoop-owned.
	audioLoopReqs  []soundReq
	audioLoopBatch []audio.BatchParam
	// Per-row structural generation: only rows whose path changed drop their in-flight beats. A fixed
	// array, so audioLoop reads race-free against edits.
	rowStructGen [maxAudioGenRows]atomic.Uint64
	// audioCh is never closed: audioLoop sends into it too. Close() closes this instead.
	audioQuit chan struct{}
	// Set at the start of Close() so audioLoop stops refilling and the audioQuit select wins.
	closing atomic.Bool
	bpmCh   chan int
	bpmAck  chan int
	playFn  func(instrument.ID, float64, ...float64)
	// Renders upcoming voices off the dispatch goroutine, so dispatch never cold-renders.
	voicePrewarmer  *voicePrewarmer
	warmup          *warmupController
	heldInstruments []instrument.ID
	heldSynced      bool

	// The voice cache key carries the rate, so a change re-keys everything.
	warmupSampleRate             int
	warmupSampleRateCheckCounter int

	// A bounded pool instead of a goroutine per note; nil in hand-built tests (immediate dispatch).
	audioScheduler *async.Scheduler

	/* graph data */
	nodes           []*uiNode
	nodesByID       map[model.NodeID]*uiNode
	edges           []uiEdge
	pendingStartRow int

	/* visuals */
	activePulses        []*pulse
	activePulse         *pulse // primary row pulse for legacy tests
	frame               int64
	renderedPulsesCount int
	highlightedBeats    map[int]int64 // Expiration frames with mute flag encoded in high bit
	// Highlights dropped because hlCh was full; parity exempts them until the frame deadline.
	// Guarded by highlightMu.
	hlDropped       map[int]int64
	highlightMu     sync.RWMutex
	selNeighbors    map[*uiNode]bool
	clipboard       nodeClipboard     // single-node copy/cut buffer (node_clipboard.go)
	pastePending    pastePendingState // in-flight paste-overwrite confirmation (node_clipboard.go)
	pasteConfirmDlg *ConfirmDialog    // shared ConfirmDialog backing the paste-overwrite portal (node_clipboard.go)
	breakPending    breakPendingState // in-flight break-circuit confirmation (grid_move_break_confirm.go)
	breakConfirmDlg *ConfirmDialog    // shared ConfirmDialog backing the break-circuit portal (grid_move_break_confirm.go)
	hover           *uiNode
	// Kept while fading out; compared by identity only, never dereferenced outside the g.nodes loop.
	hoverGlowNode *uiNode
	hoverGlowP    float64
	cursorLabel   string
	nodeAnimMu    sync.RWMutex

	// divider visuals (for tests)
	dividerHover bool
	dividerThick float64

	// Embedded; fields resolve by promotion.
	inputGestureState

	/* game state */
	bpm         int
	currentStep int                  // Current step in the sequence
	nodeRows    map[model.NodeID]int // nodeID -> row index
	nodeCacheMu sync.RWMutex
	nodeCache   map[model.NodeID]model.Node

	// An immutable snapshot swapped whole on every group mutation; read by the sequencer and UI.
	groupIdxMu sync.RWMutex
	groupIdx   *model.GroupIndex

	/* misc */
	winW, winH      int
	start           *uiNode // explicit “root/start” node (⇧S to set)
	centered        bool    // camera centered on first layout
	lastHorizontal  *bool   // track previous orientation for change detection
	lastSmallScreen *bool   // track previous Profile().IsMobile() for mode transition detection
	demoBuilt       bool    // demo circuit built once
	demoScheduled   bool    // build demo on next update
	// Benchmark mode fields (set via RunBenchmark)
	benchBPM      int
	benchDuration time.Duration
	benchStarted  bool
	benchStart    time.Time
	// Opt-in bench scenario.
	benchStorm       benchStormScenario
	benchStormInst   instrument.ID
	benchStormTick   int
	benchStormPushes int
	benchRecord      bool
	benchOutDir      string // directory for recordings + perf snapshots
	// Its own window, independent of the PERF_LOG reset.
	gcMetrics gcMetrics
	// Screenshot mode: capture screen after N draws and exit
	screenshotPath         string
	screenshotDraws        int
	screenshotSettleFrames int     // override for the default 90-frame wait; 0 = use default
	screenshotCaptured     bool    // set once captureScreen has fired; gates termination
	screenshotSubject      Subject // when non-empty, captureScreen crops to this subject's bounds
	// Scope panel: open scope on first Update after flag is set
	scopeOpen    bool
	scopeApplied bool
	// animBeatPrev was used for time-based advancement; unused now
	highlightHook func(row, idx int)

	lastFrame time.Time
	// Detects BPM changes so the timebase re-anchors instead of jumping.
	prevBPM int

	// Deliberately coarse: keeps row indexing consistent while rows shift mid-playback.
	seqMu sync.Mutex

	// Embedded; fields resolve by promotion.
	playbackSlate

	// enable special behavior for precise timing tests
	timingTestMode bool

	// Node property sidebar (left-anchored panel)
	sidebar *NodeSidebar

	groupMenu *GroupMenu

	triggerMu sync.RWMutex

	// queued UI actions to decouple button handlers from state changes
	uiQueue []func()

	// Processed after seqMu.Unlock(): drum.Update runs under it.
	pendingImportData []byte
	// "" shows the generic toast.
	pendingImportSource string
	// A user load records the previous document as one undo step; startup, JS and undo re-imports
	// clear history instead. Consumed once per Import.
	importAsUndoStep bool
	// Import then restores the snapshot's documents verbatim instead of opening a circuit.
	undoRestoring bool

	// Drained after seqMu.Unlock(), so JS callers never collide with the frame's lock.
	pendingActionsMu sync.Mutex
	pendingActions   []func(*Game)

	// Hook subscribers write from a pool goroutine; Update drains on the UI goroutine.
	pendingNotifyMu sync.Mutex
	pendingNotifs   []pendingNotif // off-thread → drained to the UI as keyed (localizable) notifications

	// Set only on an audible trigger.
	nodeAnim map[model.NodeID]float64

	// Debug: emit per-node draw logs when true (set via env DEBUG_DRAW_NODES=1).
	logDrawNodes bool

	// So an edit in one circuit never flickers another.
	quietFrames int

	scheduleHook func(row, idx int)

	// Params dirty flag to avoid per-frame hashing in Update.
	paramsDirty bool

	// perf metrics (opt-in logging via PERF_LOG=1)
	perf                  perfCounters
	schedMetrics          scheduleMetrics
	perfMode              PerfMode
	perfDrawMuted         bool
	schedMetricsNextReset time.Time
	// Per-window dedup of WarnOnly timing-gate dumps (emitTimingFatal).
	timingWarnedThisWindow map[string]struct{}

	// Embedded; fields resolve by promotion.
	gridRenderCache

	gridTree *GridTree

	// Per-frame scratch written by drawGridPane: the unclipped screen and the derived camera/culling
	// values the drawGrid* methods share. Not persistent state.
	gridDrawScreen *ebiten.Image
	gridDrawCtx    gridPaneDrawCtx

	// draw stats (for tests and diagnostics)
	lastDrawEdges  int
	lastDrawNodes  int
	lastDrawPulses int
	lastDrawGridMS float64
	lastDrawDrumMS float64
	lastUpdateMS   float64
	lastRefreshMS  float64
	lastCamOffX    float64
	lastCamOffY    float64
	panFastFrames  int

	// simplified rendering toggle for web builds
	simpleDraw                  bool
	simpleDrawSavedFollow       bool //nolint:unused // used in WASM js_exports
	simpleDrawSavedFollowValid  bool //nolint:unused // used in WASM js_exports
	simpleDrawAutoDisableFrames int
	predictorBackgroundStopped  bool
	sequencerStopped            bool
	sequencerRunning            bool
	closed                      bool
	// Close() joins these, or goleak fails and teardown hits the test timeout.
	bgWG         sync.WaitGroup
	renderReady  bool
	renderOffset int
	renderLength int
	renderFrame  int64

	// Audio scheduling lookahead in seconds (web)
	audioLookaheadSec float64

	// Off under go test unless enableAudioLaneForTest. laneGraphVersion moves on node and group changes.
	audioLane        *audioLane
	laneSrc          *gameLaneSource
	laneOn           atomic.Bool
	laneGraphVersion atomic.Uint64
	laneSink         *outboxSink
	laneWake         chan struct{}
	laneKick         chan struct{} // kickLane → sequencerLoop: run a pass now (edits outside Update)
	laneBatchScratch []audio.BatchParam
	laneOldScratch   []uint64
	laneFlushMu      sync.Mutex // serialises flushLaneOps (audioLoop vs Close)
	// Cells the UI lane passed before the lane committed them, observed after laneTick. seqMu-guarded.
	lanePendingObs [][2]int

	// Embedded; fields resolve by promotion.
	parityTracker

	// importPrev* hold parity fatal/watch across an import.
	importing             bool
	importDialog          bool
	importPrevParityFatal bool
	importPrevParityWatch parityWatchMode

	// A mutation flag, so it stays on Game; the edge cache itself is in gridRenderCache.
	edgesDirty bool

	// lastNodeHL tracks which nodes were drawn highlighted in the last Draw() call
	lastNodeHL   map[model.NodeID]bool
	lastNodeHLMu sync.RWMutex
	nodeHLUntil  map[model.NodeID]highlightWindow
	lastGlowScr  float64
}

// Older Games from earlier tests stop applying BPM while a newer one exists.
var bpmOwner atomic.Pointer[Game]

var predictorPerfThrottleEnabled = browserProfileSnapshot()

// A function so init order does not matter.
func browserProfileSnapshot() bool { return RuntimeProf().IsBrowser }

var forceAutoSize bool

// Applied on the next Update, once the DrumView is ready.
func (g *Game) SetChainVisible(v bool) {
	g.scopeOpen = v
}
