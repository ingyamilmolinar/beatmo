package ui

import (
	"sync"
	"sync/atomic"
	"time"

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
	cam                         *Camera
	split                       *Splitter
	drum                        *DrumView
	i18nCancel                  func()  // cancels the i18n.OnChange listener registered in NewGame
	gridHelpBtn                 *Button // settings gear in grid pane top-right (opens settings overlay; both desktop and mobile)
	gridFileBtn                 *Button // folder button just left of the gear (opens the file menu; both desktop and mobile)
	gridLogsBtn                 *Button // logs button left of the folder button (opens the Logs Navigation Panel; both desktop and mobile)
	gridHelpCapturing           bool    // true while the grid gear/folder/logs button holds the press; defers the tree so its click-outside doesn't close the just-opened overlay
	undoManager                 *UndoManager
	keyboardRouter              *keyboardShortcutRouter // component-owned keyboard shortcut dispatch (keyboard_shortcuts.go)
	inputDispatcher             *InputDispatcher
	lastDispatcherSidebarOpen   bool
	lastDispatcherGroupMenuOpen bool
	dispatcherDirty             bool
	graph                       *model.Graph
	graphRuntime                *graphruntime.Runtime
	state                       *gamestate.State
	// nodeTimingCache memoizes the sidebar's per-node beat/last/next readout so
	// nodeLiveTiming does not rescan the circuit every frame. Derived render
	// state, never serialized. See game_node_coords.go.
	nodeTimingCache nodeTimingMemo
	engine          *engine.Engine
	engineProgress  func() float64
	logger          *game_log.Logger
	grid            *Grid
	audioCh         chan soundReq
	// audioLoopReqs and audioLoopBatch are reusable scratch buffers held
	// across audioLoop iterations so steady-state playback does not
	// allocate ~3 KB of soundReq slice + a BatchParam slice every drain
	// cycle. On WASM, where linear memory only grows, eliminating this
	// per-iteration churn keeps the allocator's peak working set low
	// enough that GC reclamation keeps up. Owned exclusively by the
	// audioLoop goroutine; no cross-goroutine access.
	audioLoopReqs  []soundReq
	audioLoopBatch []audio.BatchParam
	// rowStructGen holds a per-row STRUCTURAL generation. updateBeatInfos bumps
	// only the generation of rows whose path signature changed; the audioLoop
	// filter drops an in-flight beat when its stamped rowGen != the current
	// per-row generation, so unchanged rows keep their scheduled beats while a
	// changed row's stale (old-path) beats are dropped and rescheduled. Fixed
	// array so audioLoop reads are race-free against concurrent structural edits.
	rowStructGen [maxAudioGenRows]atomic.Uint64
	// audioQuit terminates audioLoop. audioLoop both reads from and (via the
	// opportunistic seqScheduleTime drive) writes back into audioCh, so
	// audioCh must never be closed — closing the channel a goroutine still
	// sends to panics with "send on closed channel". Close() signals exit by
	// closing audioQuit instead; audioCh is left open and GC'd.
	audioQuit chan struct{}
	// closing is set at the start of Close() so audioLoop stops its
	// opportunistic seqScheduleTime refills, guaranteeing audioCh drains and
	// the audioQuit select wins promptly during teardown.
	closing atomic.Bool
	bpmCh   chan int
	bpmAck  chan int
	playFn  func(string, float64, ...float64)
	// voicePrewarmer renders upcoming melody voices off the audioLoop dispatch
	// goroutine so dispatch never cold-renders (fixes the overdue/absmax panic).
	voicePrewarmer *voicePrewarmer
	// audioScheduler dispatches future-timestamped playFn calls onto a
	// bounded pool instead of spawning a goroutine per scheduled note.
	// Initialized by New(); tests that construct Game directly may leave
	// it nil (SetPlayFunc falls back to immediate dispatch in that case).
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
	// hlDropped records beat keys (makeBeatKey) whose highlight the sequencer
	// dropped because hlCh was full under load. The UI never paints these, so
	// highlight_vs_audio parity exempts them — a dropped highlight is a known
	// cosmetic loss, not an audio/UI desync. Value = g.frame deadline after
	// which the exemption is pruned (highlight is a transient visual effect).
	// Guarded by highlightMu (same lock as highlightedBeats).
	hlDropped       map[int]int64
	highlightMu     sync.RWMutex
	selNeighbors    map[*uiNode]bool
	clipboard       nodeClipboard     // single-node copy/cut buffer (node_clipboard.go)
	pastePending    pastePendingState // in-flight paste-overwrite confirmation (node_clipboard.go)
	pasteConfirmDlg *ConfirmDialog    // shared ConfirmDialog backing the paste-overwrite portal (node_clipboard.go)
	breakPending    breakPendingState // in-flight break-circuit confirmation (grid_move_break_confirm.go)
	breakConfirmDlg *ConfirmDialog    // shared ConfirmDialog backing the break-circuit portal (grid_move_break_confirm.go)
	hover           *uiNode
	// hoverGlowNode/hoverGlowP ease the grid-node hover halo (shared
	// button-hover-fade spring — see advanceNodeHoverGlow). hoverGlowNode is
	// the node the halo currently renders on (kept while fading out after
	// the cursor leaves); compared by identity only, never dereferenced
	// outside the live g.nodes loop.
	hoverGlowNode *uiNode
	hoverGlowP    float64
	cursorLabel   string
	nodeAnimMu    sync.RWMutex

	// divider visuals (for tests)
	dividerHover bool
	dividerThick float64

	/* editor state — node-editor + pointer/touch gesture interaction, grouped
	   into the embedded inputGestureState (see game_input_gesture_state.go).
	   Anonymous so g.sel / g.modes / g.longPressPopup / … keep resolving via
	   field promotion. */
	inputGestureState

	/* game state */
	bpm         int
	currentStep int                  // Current step in the sequence
	nodeRows    map[model.NodeID]int // nodeID -> row index
	nodeCacheMu sync.RWMutex
	nodeCache   map[model.NodeID]model.Node

	// Node-group rule evaluation: immutable index snapshot, swapped whole on
	// every group mutation. Read from the sequencer goroutine and UI thread.
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
	// Synth param-mutation storm: opt-in, off by default. Drives
	// audio.SetInstrumentParam churn against benchStormInst during the bench
	// measure window; benchStormPushes is counted into [bench_json].
	benchStorm       benchStormScenario
	benchStormInst   string
	benchStormTick   int
	benchStormPushes int
	// Record-bench mode: when true, also start recording when the bench
	// playback begins and stop+save on shutdown. Output goes to benchOutDir.
	benchRecord bool
	benchOutDir string // directory for recordings + perf snapshots
	// gcMetrics accumulates GC pause/cycle/heap stats over a window that is
	// independent of perfCounters' 2 s PERF_LOG reset (see gc_metrics.go).
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
	// prevBPM caches the last UI BPM applied so we can detect changes and
	// re-anchor the timebase to avoid jumps when BPM changes mid-play.
	prevBPM int

	// seqMu guards state shared between the background sequencer goroutine and
	// the UI thread during structural edits (row add/delete, path rebuilds).
	// It is intentionally coarse-grained to keep row indexing consistent and
	// avoid parity panics when rows shift mid-playback.
	seqMu sync.Mutex

	// playbackSlate owns the scheduling/beat-path/timeline/trigger field cluster
	// (see playback_slate.go). Anonymous value-embed so g.nextBeatIdxs / g.timeline /
	// g.beatInfosByRow / … keep resolving via field promotion.
	playbackSlate

	// enable special behavior for precise timing tests
	timingTestMode bool

	// Node property sidebar (left-anchored panel)
	sidebar *NodeSidebar

	// GroupMenu: anchored panel editing one node group (batch param edits +
	// the single v1 rule). Sibling of sidebar, not left-docked.
	groupMenu *GroupMenu

	triggerMu sync.RWMutex

	// queued UI actions to decouple button handlers from state changes
	uiQueue []func()

	// Pending import data to be processed after seqMu is released. The import
	// callback from drum.Update() sets this; game.Update() processes it after
	// seqMu.Unlock() to avoid recursive locking.
	pendingImportData []byte
	// pendingImportSource is the short label (filename / template name) of the
	// queued import, drained alongside pendingImportData to name the success
	// notification ("Loaded <source> …"). "" ⇒ generic "Imported" toast.
	pendingImportSource string
	// importAsUndoStep marks the queued import as a user-initiated load that
	// should be undoable: Import records the pre-load document as one undo step
	// instead of clearing history. Set by the drum.onImport callback (template /
	// file load); left false for demo/startup/JS loads and undo-restore
	// re-imports, which keep the clear-history behavior. Consumed once per Import.
	importAsUndoStep bool

	// pendingActions are deferred UI mutations queued by external callers
	// (e.g. JS exports invoked via page.evaluate between frames). Game.Update
	// drains them after seqMu.Unlock(), mirroring pendingImportData so JS
	// callers never collide with the per-frame seqMu held by drum.Update.
	pendingActionsMu sync.Mutex
	pendingActions   []func(*Game)

	// pendingNotifyMu guards pendingNotifyInfo / pendingNotifyError. Hook
	// subscribers (e.g., EventRecordStop) write here from a background pool
	// goroutine; game.Update drains and dispatches via DrumView's
	// notifyInfo/notifyError on the UI goroutine. Mirrors the pendingImportData
	// pattern so we never call into UI code from off-thread.
	pendingNotifyMu sync.Mutex
	pendingNotifs   []pendingNotif // off-thread → drained to the UI as keyed (localizable) notifications

	// Per-node trigger animation in [0..1], decays each frame. Set only when
	// an audible trigger occurs (after applying node logic and mute/solo).
	nodeAnim map[model.NodeID]float64

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
	// Rolling-window deadline for schedMetrics (timing-parity gate); see
	// maybeResetSchedMetricsWindow in game_parity_timing.go.
	schedMetricsNextReset time.Time
	// Per-window dedup of WarnOnly timing-gate dumps (emitTimingFatal).
	timingWarnedThisWindow map[string]struct{}

	// grid/node/edge render caches — grouped into one embedded struct (see
	// game_grid_render_cache.go). Anonymous so g.gridTileW / g.nodeLayer / … keep
	// resolving via field promotion; this only lifts the ~34 cache fields out of
	// the Game struct into a cohesive unit.
	gridRenderCache

	// gridTree owns the top grid pane's z-ordered draw + input tree
	// (sibling to drum.tree which owns the bottom pane). See grid_tree.go.
	gridTree *GridTree

	// gridDrawScreen / gridDrawCtx are transient per-frame fields set by
	// (*Game).drawGridPane before it dispatches to the drawGrid* sub-methods
	// (grid_pane_draw.go). They carry, respectively, the unclipped screen image
	// (for blocks that intentionally draw outside the grid subimage — node glow
	// blooms, the desktop cursor label, debug crosses) and the per-frame derived
	// camera/culling/edge-style locals that the sub-methods share. The
	// one-time side-effecting setup (FastPanDetect counter mutation, draw-stat
	// reset) runs exactly once in the dispatcher; the derived values below are
	// pure functions of camera/grid state stashed here so the verbatim-cut
	// sub-methods can read them without recomputation. Not persistent state —
	// overwritten every drawGridPane call.
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
	// bgWG tracks the background goroutines launched by New (audioLoop +
	// bpmLoop) so Close() can join them deterministically. Without this,
	// Close() returns before the goroutines unpark from their channel
	// receives, causing goleak.VerifyTestMain to panic on leftover
	// goroutines and the 60s -timeout to kick in mid-teardown.
	bgWG         sync.WaitGroup
	renderReady  bool
	renderOffset int
	renderLength int
	renderFrame  int64

	// Audio scheduling lookahead in seconds (web)
	audioLookaheadSec float64

	// Parity diagnostics between scheduler (audio) and DrumView slate — grouped
	// into the embedded parityTracker (see game_parity_tracker.go). Anonymous so
	// g.parityRing / g.parityGen / g.parityScan* keep resolving via field
	// promotion; this only lifts the ~20 parity fields out of the Game struct.
	parityTracker

	// Import flow state (kept on Game — not parity memoization). importPrev* save
	// parity fatal/watch across an import so it can be restored afterwards.
	importing             bool
	importDialog          bool
	importPrevParityFatal bool
	importPrevParityWatch parityWatchMode

	// edgesDirty is set on graph edits to drive an edge-cache rebuild. It stays a
	// direct Game field (a mutation flag, not render memoization); the edge cache
	// data itself lives in the embedded gridRenderCache.
	edgesDirty bool

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
// can opt in without requiring a wasm build. Snapshotted from the runtime
// profile at init; tests may override via game_test_registry_test.go.
var predictorPerfThrottleEnabled = browserProfileSnapshot()

// browserProfileSnapshot returns the IsBrowser bit. Defined as a function (not
// inline) so init order doesn't matter — RuntimeProf is safe to call before
// or after this var is initialized.
func browserProfileSnapshot() bool { return RuntimeProf().IsBrowser }

// forceAutoSize can be toggled by tests to exercise Layout's auto-sizing logic
// even when running under "go test". Default is false.
var forceAutoSize bool

// SetChainVisible marks the scope panel to be made visible on the next Update
// after the DrumView is ready. This is the entry point for the -scope CLI flag.
func (g *Game) SetChainVisible(v bool) {
	g.scopeOpen = v
}
