package engine

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Predictor is the AUTHORITATIVE SOURCE OF TRUTH for what notes fire. The UI
// reads predictions via VisibleAt/TriggeredAt/AudibleAt; it never mutates
// predictor state directly.
//
// Key behaviors:
//   - Ensure(horizon): compute predictions up to an absolute index
//   - Mute gate: only gates the mute subdivision (idx+hold+1); audio resumes
//     immediately on the following beat. UI/audio mute tests depend on this.
//   - Probability uses a deterministic hash on (row, idx, nodeID) for
//     reproducible results. Skip/every-N uses per-node trigger counts.
//     Previous-trigger logic checks lastFiredByRow.
//   - predDirty flag: set by UI/game when node parameters change, triggers a
//     full context rebuild (RebaseAt) before extending predictions.
//
// Concurrency: all public methods are protected by mu (RWMutex). Background
// precompute runs in a separate goroutine controlled by bgQuit/bgDone.
type Predictor struct {
	mu sync.RWMutex

	// Static graph reference (legacy: will be removed once setters provide snapshots).
	graph *model.Graph

	// Snapshot of graph nodes (params/types) to avoid touching the graph maps from background workers.
	nodes map[model.NodeID]model.Node

	// Paths/state provided by UI (or computed externally) per row
	beatInfosByRow [][]model.BeatInfo
	isLoopByRow    []bool
	loopStartByRow []int
	loopLenByRow   []int

	// Prediction buffers — sliding window per session. The three slices share a
	// single windowStart (abs of slot 0) and windowEnd (exclusive abs upper
	// bound). Once windowEnd-windowStart reaches windowCap, further Ensure
	// calls slide the window forward in place rather than growing — bounding
	// retained memory at O(rows × 3 × windowCap) regardless of session length.
	// Reads at idx < windowStart return false (evicted; the timeline cold
	// archive at internal/timeline/archive.go owns historical playback).
	audibleByRow   [][]bool
	visibleByRow   [][]bool
	triggeredByRow [][]bool
	windowStart    int // abs of slot 0 in every per-row buffer
	windowEnd      int // exclusive abs upper bound (== windowStart + buffer length when extended)
	windowCap      int // max retained subdivisions; 0 → defaultPredictorWindowCap on first Ensure
	// visibleMinAbs anchors the lower edge of the retained window to the UI's
	// currently-displayed region: Ensure never slides windowStart past this
	// value, and re-extends backward when visibleMinAbs falls below windowStart
	// (the scroll-back case). The sentinel -1 means "no anchor declared";
	// in that state the predictor falls back to its standalone cap-only
	// eviction policy so engine-only tests and headless callers behave as
	// before. Reset to -1 by resetBuffersLocked because a path change
	// invalidates whatever the UI declared previously.
	visibleMinAbs int

	// Incremental contexts at current horizon
	countsByRow        map[int]map[model.NodeID]int
	triggerCountsByRow map[int]map[model.NodeID]int
	lastTrigByRow      map[int]map[model.NodeID]bool
	lastFiredByRow     []model.NodeID
	gateUntilByRow     []int
	visGateUntilByRow  []int

	// Background precompute. bgRunning gates StartBackground so concurrent
	// or repeat calls don't leak goroutines. Cleared by StopBackground after
	// the worker exits, allowing a clean restart.
	bgQuit    chan struct{}
	bgDone    chan struct{}
	bgStopped bool
	bgRunning atomic.Bool
	// Function returning target horizon to aim for in background
	targetFn func() int

	// predDirty signals that contexts must be rebuilt from scratch before
	// extending predictions. UI/game mark this flag when node parameters change.
	predDirty bool

	// forceFullRebuild disables the bounded dirty-rebuild optimization (always
	// re-walk [0, windowStart) instead of the last few loops). Test-only seam
	// used by the equivalence test to prove the bounded path is byte-identical
	// to the full walk.
	forceFullRebuild bool

	logger *game_log.Logger
}

// SetForceFullRebuildForTest forces the dirty rebuild to re-walk the entire
// history (the pre-optimization path). Test-only.
func (p *Predictor) SetForceFullRebuildForTest(v bool) {
	p.mu.Lock()
	p.forceFullRebuild = v
	p.mu.Unlock()
}

// defaultPredictorWindowCap is the per-row sliding-window ceiling when
// SetWindowCap has not been called. ~8 visible windows of 512 subdivisions
// gives reconcileFrozen + parity scan + visible-window draw all the lookback
// they need; older abs queries are answered by the timeline cold archive.
const defaultPredictorWindowCap = 4096

func NewPredictor(graph *model.Graph, logger *game_log.Logger) *Predictor {
	return &Predictor{
		graph:              graph,
		nodes:              make(map[model.NodeID]model.Node),
		countsByRow:        make(map[int]map[model.NodeID]int),
		triggerCountsByRow: make(map[int]map[model.NodeID]int),
		lastTrigByRow:      make(map[int]map[model.NodeID]bool),
		bgQuit:             make(chan struct{}),
		logger:             logger,
		windowCap:          defaultPredictorWindowCap,
		visibleMinAbs:      -1, // no UI anchor declared yet
	}
}

// SetWindowCap overrides the per-row sliding-window ceiling. Values <= 0 are
// ignored. Idempotent. Internal/UI calls this after engine.New to thread the
// RuntimeProfile.PredictorWindowCap value; tests call it directly to exercise
// small caps without driving abs into the millions.
func (p *Predictor) SetWindowCap(n int) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	p.windowCap = n
	p.mu.Unlock()
}

// SetVisibleMinAbs anchors the retained window's lower edge: Ensure will not
// slide windowStart past this abs. UI calls this each refreshDrumRow with the
// current scroll origin so the displayed range is never evicted. Negative
// values are clamped to 0.
func (p *Predictor) SetVisibleMinAbs(abs int) {
	if abs < 0 {
		abs = 0
	}
	p.mu.Lock()
	p.visibleMinAbs = abs
	p.mu.Unlock()
}
