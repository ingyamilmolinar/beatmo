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

	// Prediction buffers
	audibleByRow   [][]bool
	visibleByRow   [][]bool
	triggeredByRow [][]bool
	horizon        int

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

	logger *game_log.Logger
}

func NewPredictor(graph *model.Graph, logger *game_log.Logger) *Predictor {
	return &Predictor{
		graph:              graph,
		nodes:              make(map[model.NodeID]model.Node),
		countsByRow:        make(map[int]map[model.NodeID]int),
		triggerCountsByRow: make(map[int]map[model.NodeID]int),
		lastTrigByRow:      make(map[int]map[model.NodeID]bool),
		bgQuit:             make(chan struct{}),
		logger:             logger,
	}
}
