package engine

import (
	"sync"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Predictor centralizes prediction buffers and contexts. It is concurrency-safe
// and owned by the engine so sequencing and UI can consult it without races.
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

	// Background precompute
	bgQuit    chan struct{}
	bgDone    chan struct{}
	bgStopped bool
	// Function returning target horizon to aim for in background
	targetFn func() int

	// predDirty signals that contexts must be rebuilt from scratch before
	// extending predictions. UI/game mark this flag when node parameters change.
	predDirty bool
}

func NewPredictor(graph *model.Graph) *Predictor {
	return &Predictor{
		graph:              graph,
		nodes:              make(map[model.NodeID]model.Node),
		countsByRow:        make(map[int]map[model.NodeID]int),
		triggerCountsByRow: make(map[int]map[model.NodeID]int),
		lastTrigByRow:      make(map[int]map[model.NodeID]bool),
		bgQuit:             make(chan struct{}),
	}
}
