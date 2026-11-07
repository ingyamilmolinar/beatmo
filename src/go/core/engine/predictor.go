package engine

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The AUTHORITATIVE source of what fires; the UI only reads it. The mute gate covers only the mute subdivision
// (idx+hold+1), audio resumes on the next beat; probability is a deterministic hash on (row, idx, nodeID). mu (RWMutex)
// guards every public method; the background precompute runs in its own goroutine.
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

	// A sliding window per session sharing windowStart / windowEnd: past windowCap, Ensure slides forward instead of
	// growing, bounding memory at O(rows × 3 × windowCap). Reads below windowStart return false; the timeline archive owns
	// history.
	audibleByRow   [][]bool
	visibleByRow   [][]bool
	triggeredByRow [][]bool
	windowStart    int // abs of slot 0 in every per-row buffer
	windowEnd      int // exclusive abs upper bound (== windowStart + buffer length when extended)
	windowCap      int // max retained subdivisions; 0 → defaultPredictorWindowCap on first Ensure
	// The UI's displayed lower edge: Ensure never slides past it and re-extends backward when it falls below windowStart
	// (scroll-back). -1 = no anchor (cap-only eviction for headless callers); reset on a path change.
	visibleMinAbs int

	// Incremental contexts at current horizon
	countsByRow        map[int]map[model.NodeID]int
	triggerCountsByRow map[int]map[model.NodeID]int
	lastTrigByRow      map[int]map[model.NodeID]bool
	lastFiredByRow     []model.NodeID
	gateUntilByRow     []int
	visGateUntilByRow  []int

	// bgRunning gates StartBackground so repeat or concurrent calls spawn one worker; StopBackground clears it.
	bgQuit    chan struct{}
	bgDone    chan struct{}
	bgStopped bool
	bgRunning atomic.Bool
	// Function returning target horizon to aim for in background
	targetFn func() int

	// predDirty signals that contexts must be rebuilt from scratch before
	// extending predictions. UI/game mark this flag when node parameters change.
	predDirty bool

	// Test-only: always re-walk [0, windowStart), to prove the bounded dirty rebuild byte-identical to the full walk.
	forceFullRebuild bool

	logger *game_log.Logger

	// epoch counts every change that can alter an ALREADY-computed cell
	// (paths, node params, rebase, buffer reset). Extending the horizon does
	// not bump it. The audio lane fingerprints the predictor with it.
	epoch atomic.Uint64

	// Custom-logic decisions for audible cells, so the audio lane's pure note function applies them without re-running
	// stateful logic. Guarded by mu.
	decisions map[[2]int]model.NodeDecision
}

// Epoch returns the predictor's change epoch (see the epoch field).
func (p *Predictor) Epoch() uint64 { return p.epoch.Load() }

// DecisionAt returns the custom Params.Logic decision recorded for an audible
// cell; ok is false for cells without custom logic (or not audible).
func (p *Predictor) DecisionAt(row, idx int) (model.NodeDecision, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	d, ok := p.decisions[[2]int{row, idx}]
	return d, ok
}

// recordDecisionLocked stores (ok) or clears (!ok) a cell's custom-logic
// decision. Caller holds mu (evalAudible runs under the compute lock).
func (p *Predictor) recordDecisionLocked(row, idx int, d model.NodeDecision, ok bool) {
	if !ok {
		if p.decisions != nil {
			delete(p.decisions, [2]int{row, idx})
		}
		return
	}
	if p.decisions == nil {
		p.decisions = make(map[[2]int]model.NodeDecision)
	}
	p.decisions[[2]int{row, idx}] = d
}

// SetForceFullRebuildForTest forces the dirty rebuild to re-walk the entire
// history (the pre-optimization path). Test-only.
func (p *Predictor) SetForceFullRebuildForTest(v bool) {
	p.mu.Lock()
	p.forceFullRebuild = v
	p.mu.Unlock()
}

// Enough lookback for reconcileFrozen, the parity scan and the visible window; older queries go to the timeline
// archive.
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

// <= 0 is ignored. The UI threads RuntimeProfile.PredictorWindowCap through it; tests set small caps.
func (p *Predictor) SetWindowCap(n int) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	p.windowCap = n
	p.mu.Unlock()
}

// Called each refreshDrumRow with the scroll origin, so the displayed range is never evicted. Negative clamps to 0.
func (p *Predictor) SetVisibleMinAbs(abs int) {
	if abs < 0 {
		abs = 0
	}
	p.mu.Lock()
	p.visibleMinAbs = abs
	p.mu.Unlock()
}
