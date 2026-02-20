package ui

import (
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/graphruntime"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

type Game struct {
	/* subsystems */
	cam                       *Camera
	split                     *Splitter
	drum                      *DrumView
	inputDispatcher           *InputDispatcher
	lastDispatcherSidebarOpen bool
	dispatcherDirty           bool
	graph           *model.Graph
	graphRuntime    *graphruntime.Runtime
	state           *gamestate.State
	engine          *engine.Engine
	engineProgress  func() float64
	logger          *game_log.Logger
	grid            *Grid
	audioCh         chan soundReq
	audioGen        atomic.Uint64
	bpmCh           chan int
	bpmAck          chan int
	playFn          func(string, float64, ...float64)

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
	highlightMu         sync.RWMutex
	selNeighbors        map[*uiNode]bool
	hover               *uiNode
	cursorLabel         string
	nodeAnimMu          sync.RWMutex

	// divider visuals (for tests)
	dividerHover bool
	dividerThick float64

	/* editor state */
	sel            *uiNode
	linkDrag       dragLink
	camDragging    bool
	camDragged     bool
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int
	clickNode      *uiNode

	// Move node mode: MOVE button sets moveMode, next grid click places node
	moveMode        bool    // MOVE mode active, next click places node
	movingNode      *uiNode // node being moved
	moveConfirm     bool    // confirmation dialog showing
	moveConfirmI    int     // target coordinates for pending move
	moveConfirmJ    int
	moveEdgeLoss    int  // number of edges that would be dropped
	moveSkipRelease bool // skip the first mouse release after entering move mode

	// Long-press quick-action popup (mobile)
	longPressPopup          bool
	longPressPopupNode      *uiNode
	longPressPopupRect      image.Rectangle
	longPressPopupMove      image.Rectangle
	longPressPopupConn      image.Rectangle
	longPressPopupDel       image.Rectangle
	longPressPopupHover     string // "move", "connect", "delete", or ""
	longPressPopupLastHover string // hover from previous frame (for release detection)

	// Connect mode: next node tap creates edge from connectFromNode → target
	connectMode     bool
	connectFromNode *uiNode

	// Coordinate badge above selected/created node
	coordBadgeNode  *uiNode // node to show badge for
	coordBadgeFrame int64   // frame when badge was set

	pinchBaseScale        float64 // camera scale when pinch started
	pinchBaseGestureScale float64 // gesture scale value on first pinch event

	/* game state */
	bpm                int
	currentStep        int                // Current step in the sequence
	beatInfos          []model.BeatInfo   // Full traversal path for primary start
	drumBeatInfos      []model.BeatInfo   // BeatInfos sized to drum view
	beatInfosByRow     [][]model.BeatInfo // Per-row traversal paths
	isLoop             bool               // Indicates if the current graph forms a loop
	loopStartIndex     int                // The index in beatInfos where the loop segment begins
	isLoopByRow        []bool
	loopStartByRow     []int
	loopLenByRow       []int
	originIdxsByRow    [][]int
	nextOriginIdxByRow []int
	nextBeatIdxs       []int                // Absolute beat index per row
	nextIdxSticky      []bool               // Preserve nextBeatIdxs on row shifts
	startNextBuf       []int                // Reusable buffer for frame-start snapshot of nextBeatIdxs
	nodeRows           map[model.NodeID]int // nodeID -> row index
	elapsedBeats       int
	nodeCacheMu        sync.RWMutex
	nodeCache          map[model.NodeID]model.Node

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
	// Screenshot mode: capture screen after N draws and exit
	screenshotPath  string
	screenshotDraws int
	// animBeatPrev was used for time-based advancement; unused now
	highlightHook func(row, idx int)

	lastFrame        time.Time
	samplesScheduled bool
	// prevBPM caches the last UI BPM applied so we can detect changes and
	// re-anchor the timebase to avoid jumps when BPM changes mid-play.
	prevBPM int

	// sequencer (engine-driven audio independent from UI)
	seqCh       <-chan engine.Event
	seqNextIdxs []int
	seqQuit     chan struct{}
	// seqMu guards state shared between the background sequencer goroutine and
	// the UI thread during structural edits (row add/delete, path rebuilds).
	// It is intentionally coarse-grained to keep row indexing consistent and
	// avoid parity panics when rows shift mid-playback.
	seqMu sync.Mutex
	// seqPathSnap holds an immutable snapshot of per-row beat paths for the
	// background sequencer so path rebuilds do not race with scheduling.
	seqPathSnap atomic.Pointer[seqPathSnapshot]

	// lastHLIdxByRow tracks the last highlighted absolute subdivision index
	// per row to prevent duplicate highlight hooks when the same cell is
	// refreshed across frames.
	lastHLIdxByRow []int

	// highlight scheduling from sequencer goroutine
	hlCh chan struct {
		row, idx int
		info     model.BeatInfo
	}

	// enable special behavior for precise timing tests
	timingTestMode bool

	// Per-row trigger counters for node-level logic evaluation.
	nodeTriggerCountsByRow map[int]map[model.NodeID]int
	// Guard to avoid double-incrementing trigger counters when the same
	// (row, idx, node) is evaluated multiple times within a frame.
	lastEvalIdxByRowNode map[int]map[model.NodeID]int
	// Per-row trigger counters for NodeLogic callbacks (separate from built-in logic).
	nodeLogicTriggerCountsByRow map[int]map[model.NodeID]int

	// Node property sidebar (left-anchored panel)
	sidebar *NodeSidebar

	// Last trigger state for each node per row: true if the last evaluation
	// for that node resulted in an audible trigger. Used by advanced logic
	// rules that depend on previous node trigger/skip.
	lastTriggeredByRow map[int]map[model.NodeID]bool

	triggerMu sync.RWMutex

	// queued UI actions to decouple button handlers from state changes
	uiQueue []func()

	// Pending import data to be processed after seqMu is released. The import
	// callback from drum.Update() sets this; game.Update() processes it after
	// seqMu.Unlock() to avoid recursive locking.
	pendingImportData []byte

	// Per-node trigger animation in [0..1], decays each frame. Set only when
	// an audible trigger occurs (after applying node logic and mute/solo).
	nodeAnim map[model.NodeID]float64

	// Last audible node fired on each row; used by built-in logic rules.
	lastFiredNodeByRow []model.NodeID
	// Per-row mute gate (absolute subdivision index, exclusive) set by
	// mute nodes to temporarily silence playback.
	muteUntilByRow []int
	// Loop completion counts per row.
	loopCountByRow []int

	// Debug: emit per-node draw logs when true (set via env DEBUG_DRAW_NODES=1).
	logDrawNodes bool

	// After certain UI operations like setting an origin or creating a node
	// as part of origin selection, we suppress transient visuals (edge pulses,
	// per-node trigger border flashes) for a few frames to guarantee that
	// edits in one circuit never create flickers in other circuits.
	quietFrames int

	// Test-only: called synchronously when the scheduler decides to play a
	// particular (row, idx). Not used in production.
	scheduleHook func(row, idx int)

	// Params dirty flag to avoid per-frame hashing in Update.
	paramsDirty bool

	// perf metrics (opt-in logging via PERF_LOG=1)
	perf          perfCounters
	schedMetrics  scheduleMetrics
	perfMode      PerfMode
	perfDrawMuted bool

	// grid render cache
	gridTile       *ebiten.Image
	gridTileStepPx int
	gridTileSubSig uint64
	// grid layer cache (screen-space)
	gridCache       *ebiten.Image
	gridCacheW      int
	gridCacheH      int
	gridCacheScale  float64
	gridCacheOffX   float64
	gridCacheOffY   float64
	gridCachePad    int
	gridCacheStepPx int
	gridCacheSubSig uint64

	// draw stats (for tests and diagnostics)
	lastDrawEdges  int
	lastDrawNodes  int
	lastDrawPulses int
	lastDrawGridMS float64
	lastDrawDrumMS float64
	lastUpdateMS   float64
	lastRefreshMS  float64
	// Seeking freeze: pause auto-tracking for a short window after user scrubs
	// Pan velocity detection for WASM perf gating
	lastCamOffX   float64
	lastCamOffY   float64
	panFastFrames int

	// simplified rendering toggle for web builds
	simpleDraw                  bool
	simpleDrawSavedFollow       bool //nolint:unused // used in WASM js_exports
	simpleDrawSavedFollowValid  bool //nolint:unused // used in WASM js_exports
	simpleDrawAutoDisableFrames int
	predictorBackgroundStopped  bool
	sequencerStopped            bool
	sequencerRunning            bool
	closed                      bool
	renderReady                 bool
	renderOffset                int
	renderLength                int
	renderFrame                 int64

	// Draw throttle (web): minimum wall-clock interval between Draw calls
	drawMinInterval time.Duration
	lastDrawAt      time.Time

	// Audio scheduling lookahead in seconds (web)
	audioLookaheadSec float64

	// Parity diagnostics between scheduler (audio) and DrumView slate.
	parityRing            mismatchRing
	parityStreak          map[string]int
	parityWatch           parityWatchMode
	parityMu              sync.Mutex
	parityAudio           []parityAudioEvent
	parityAudioMaxIdx     []int
	paritySeqDecisions    map[int]map[int]paritySeqDecision
	importing             bool
	importDialog          bool
	importPrevParityFatal bool
	importPrevParityWatch parityWatchMode
	parityScanEvery       int
	parityScanStride      int
	parityScanPhase       int
	parityScanLastFrame   int64
	parityScanSumNS       int64
	parityScanMaxNS       int64
	parityScanCount       int64

	// Cached frame buffer drawn into before copying to screen; reused when draw
	// throttling skips a frame so the browser does not flash blank.
	frameBuffer        *ebiten.Image
	frameBufferW       int
	frameBufferH       int
	drawThrottleCopies int

	// Per-frame pre-computed node radii (avoids O(n²) neighbor checks in draw loop)
	nodeRadiiCache []float64

	// Static node layer cache: all nodes in default (non-highlighted) state.
	// Rebuilt only when camera moves beyond pad, graph changes, or scale changes.
	nodeLayer         *ebiten.Image
	nodeLayerDirty    bool
	nodeLayerCamOffX  float64
	nodeLayerCamOffY  float64
	nodeLayerCamScale float64
	nodeLayerGraphSig uint64 // hash of node positions + colors + types
	nodeLayerPad      int    // reuse tolerance for camera pans

	// node sprite cache (screen-space) keyed by radius px + colors
	nodeSpriteCache map[spriteKey]*ebiten.Image

	// edge static cache (screen-space)
	edgeCache         *ebiten.Image
	edgeCacheW        int
	edgeCacheH        int
	edgeCacheScale    float64
	edgeCacheOffX     float64
	edgeCacheOffY     float64
	edgeCacheCount    int
	edgesDirty        bool
	edgeCachePad      int
	edgeCacheColorSig uint64

	// Per-row signature of the current rendered row window (Steps + CellTypes)
	// used to invalidate only changed row caches when circuits are edited
	// during active playback.
	rowRenderSig []uint64
	// Scratch buffers used by refreshDrumRow so published Steps/CellTypes are not
	// mutated in place. When rowSnapshotMode is enabled (debug), refreshDrumRow
	// allocates fresh slices instead of reusing these buffers.
	rowStepsScratch [][]bool
	rowTypesScratch [][]model.NodeType
	rowSnapshotMode bool
	// Offsets of the last DrumView window used to populate step/cell metadata.
	// Helps preserve past cells across refreshes when the visible window stays
	// stationary; rewound windows must recompute from predictor snapshots.
	lastStepsOffset     int
	lastCellTypesOffset int

	// Segmented drum timeline; stores committed past beats per row.
	timeline *timeline.Service
	// Highest absolute subdivision index frozen per row (inclusive). Starts
	// at -1; grows monotonically while playing. Reset on Stop.
	frozenUpToByRow []int

	// Path signatures per row to detect circuit shape changes. Used to avoid
	// recomputing stable rows during active playback.
	pathSigByRow        []uint64
	rowsPathChanged     []bool
	pathChangeBeatByRow []int
	pathsDirty          bool

	// lastNodeHL tracks which nodes were drawn highlighted in the last Draw() call
	lastNodeHL   map[model.NodeID]bool
	lastNodeHLMu sync.RWMutex
	nodeHLUntil  map[model.NodeID]highlightWindow
	lastGlowScr  float64
}

// bpmOwner points to the Game instance that currently owns BPM application.
// Older Game instances created by earlier tests yield and do not call the
// global audio.SetBPM while a newer Game exists.
var bpmOwner atomic.Pointer[Game]

// predictorPerfThrottleEnabled gates draw-based background throttling so tests
// can opt in without requiring a wasm build.
var predictorPerfThrottleEnabled = (runtime.GOARCH == "wasm")

// forceAutoSize can be toggled by tests to exercise Layout's auto-sizing logic
// even when running under "go test". Default is false.
var forceAutoSize bool
