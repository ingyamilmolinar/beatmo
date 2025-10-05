package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Predictor centralizes prediction buffers and contexts. It is concurrency-safe
// and owned by the engine so sequencing and UI can consult it without races.
type Predictor struct {
	mu sync.RWMutex

	// Static graph reference
	graph *model.Graph

	// Snapshot of graph nodes (params/types) to avoid touching the graph
	// maps from background workers.
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
	countsByRow       map[int]map[model.NodeID]int
	lastTrigByRow     map[int]map[model.NodeID]bool
	lastFiredByRow    []model.NodeID
	gateUntilByRow    []int
	visGateUntilByRow []int

	// Background precompute
	bgQuit chan struct{}
	// Function returning target horizon to aim for in background
	targetFn func() int

	// predDirty signals that contexts must be rebuilt from scratch before
	// extending predictions. UI/game mark this flag when node parameters change.
	predDirty bool
}

func NewPredictor(graph *model.Graph) *Predictor {
	return &Predictor{
		graph:          graph,
		nodes:          make(map[model.NodeID]model.Node),
		countsByRow:    make(map[int]map[model.NodeID]int),
		lastTrigByRow:  make(map[int]map[model.NodeID]bool),
		lastFiredByRow: nil,
		bgQuit:         make(chan struct{}),
	}
}

// SetPaths replaces the per-row traversal info. Call when the graph or row
// origins change. Slices are copied for thread-safety.
func (p *Predictor) SetPaths(paths [][]model.BeatInfo, isLoop []bool, loopStart []int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(paths)
	p.beatInfosByRow = make([][]model.BeatInfo, n)
	for i := 0; i < n; i++ {
		p.beatInfosByRow[i] = append([]model.BeatInfo(nil), paths[i]...)
	}
	p.isLoopByRow = append([]bool(nil), isLoop...)
	p.loopStartByRow = append([]int(nil), loopStart...)
	p.loopLenByRow = make([]int, n)
	for i := 0; i < n; i++ {
		if i < len(paths) && i < len(isLoop) && isLoop[i] {
			start := 0
			if i < len(loopStart) {
				start = loopStart[i]
			}
			p.loopLenByRow[i] = loopSegmentLen(paths[i], start)
		}
	}
	if p.graph != nil {
		nodes := make(map[model.NodeID]model.Node, len(p.graph.Nodes))
		for id, node := range p.graph.Nodes {
			nodes[id] = node
		}
		p.nodes = nodes
	} else {
		p.nodes = make(map[model.NodeID]model.Node)
	}
	p.resetBuffersLocked(n)
}

// resetBuffersLocked clears predictions and contexts. Caller must hold p.mu.
func (p *Predictor) resetBuffersLocked(n int) {
	p.audibleByRow = make([][]bool, n)
	p.visibleByRow = make([][]bool, n)
	p.triggeredByRow = make([][]bool, n)
	p.horizon = 0
	p.countsByRow = make(map[int]map[model.NodeID]int)
	p.lastTrigByRow = make(map[int]map[model.NodeID]bool)
	p.lastFiredByRow = make([]model.NodeID, n)
	p.gateUntilByRow = make([]int, n)
	p.visGateUntilByRow = make([]int, n)
	for i := range p.visGateUntilByRow {
		p.visGateUntilByRow[i] = -1
	}
}

// Ensure grows prediction buffers to at least horizon.
func (p *Predictor) Ensure(horizon int) {
	if horizon < 0 {
		horizon = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.predDirty && horizon <= p.horizon {
		return
	}
	// Ensure row containers
	n := len(p.beatInfosByRow)
	if len(p.audibleByRow) != n {
		p.resetBuffersLocked(n)
	}
	for row := 0; row < n; row++ {
		// Grow capacity then length
		if cap(p.audibleByRow[row]) < horizon {
			tmp := make([]bool, len(p.audibleByRow[row]), horizon)
			copy(tmp, p.audibleByRow[row])
			p.audibleByRow[row] = tmp
		}
		if cap(p.visibleByRow[row]) < horizon {
			tmp := make([]bool, len(p.visibleByRow[row]), horizon)
			copy(tmp, p.visibleByRow[row])
			p.visibleByRow[row] = tmp
		}
		if cap(p.triggeredByRow[row]) < horizon {
			tmp := make([]bool, len(p.triggeredByRow[row]), horizon)
			copy(tmp, p.triggeredByRow[row])
			p.triggeredByRow[row] = tmp
		}
		if len(p.audibleByRow[row]) < horizon {
			p.audibleByRow[row] = p.audibleByRow[row][:horizon]
		}
		if len(p.visibleByRow[row]) < horizon {
			p.visibleByRow[row] = p.visibleByRow[row][:horizon]
		}
		if len(p.triggeredByRow[row]) < horizon {
			p.triggeredByRow[row] = p.triggeredByRow[row][:horizon]
		}
	}
	// Compute from current horizon to requested horizon.
	startIdx := p.horizon
	for row := 0; row < n; row++ {
		counts := p.countsByRow[row]
		if counts == nil {
			counts = make(map[model.NodeID]int)
		}
		lastTrig := p.lastTrigByRow[row]
		if lastTrig == nil {
			lastTrig = make(map[model.NodeID]bool)
		}
		lastFired := model.InvalidNodeID
		if row < len(p.lastFiredByRow) {
			lastFired = p.lastFiredByRow[row]
		}
		gate := 0
		if row < len(p.gateUntilByRow) {
			gate = p.gateUntilByRow[row]
		}
		visGate := -1
		if row < len(p.visGateUntilByRow) {
			visGate = p.visGateUntilByRow[row]
		}
		if row < len(p.visGateUntilByRow) {
			visGate = p.visGateUntilByRow[row]
		}

		loop := row < len(p.isLoopByRow) && p.isLoopByRow[row]
		start := 0
		seg := 0
		if loop {
			if row < len(p.loopStartByRow) {
				start = p.loopStartByRow[row]
			}
			seg = p.loopLenByRow[row]
		}
		if p.predDirty {
			counts = make(map[model.NodeID]int)
			lastTrig = make(map[model.NodeID]bool)
			lastFired = model.InvalidNodeID
			gate = 0
			visGate = -1
			for i := 0; i < startIdx; i++ {
				bi := p.beatInfoAtRow(row, i)
				if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
					continue
				}
				_, triggered := p.evalAudible(row, i, bi, counts, &lastFired, lastTrig, &gate)
				if bi.NodeType == model.NodeTypeMute && triggered {
					if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
						visGate = i + 1
					}
				}
				if bi.NodeType == model.NodeTypeRegular {
					if triggered {
						lastFired = bi.NodeID
						lastTrig[bi.NodeID] = true
					} else {
						lastTrig[bi.NodeID] = false
					}
				} else if bi.NodeType == model.NodeTypeMute {
					lastTrig[bi.NodeID] = triggered
				}
			}
		}
		for idx := startIdx; idx < horizon; idx++ {
			bi := p.beatInfoAtRow(row, idx)
			audible, triggered := p.evalAudible(row, idx, bi, counts, &lastFired, lastTrig, &gate)
			p.audibleByRow[row][idx] = audible
			p.triggeredByRow[row][idx] = triggered
			vis := audible
			if bi.NodeType == model.NodeTypeMute && triggered {
				vis = true
				if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
					visGate = idx + 1
				}
			} else if bi.NodeType == model.NodeTypeRegular {
				if visGate >= 0 && idx <= visGate {
					vis = false
				}
			}
			if loop && seg > 0 && idx >= start+1 {
				if (idx-(start+1))%seg == 0 {
					if bi.NodeType == model.NodeTypeInvisible {
						vis = false
					}
				}
			}
			p.visibleByRow[row][idx] = vis
			if bi.NodeType == model.NodeTypeRegular {
				if audible {
					lastFired = bi.NodeID
					lastTrig[bi.NodeID] = true
				} else {
					lastTrig[bi.NodeID] = false
				}
			} else if bi.NodeType == model.NodeTypeMute {
				lastTrig[bi.NodeID] = triggered
			}
		}
		p.countsByRow[row] = counts
		p.lastTrigByRow[row] = lastTrig
		if row < len(p.lastFiredByRow) {
			p.lastFiredByRow[row] = lastFired
		}
		if row < len(p.gateUntilByRow) {
			p.gateUntilByRow[row] = gate
		}
		if row < len(p.visGateUntilByRow) {
			p.visGateUntilByRow[row] = visGate
		}
	}
	p.horizon = horizon
}

// RebaseAt trims predictions from idx onward and rebuilds contexts up to idx
// so future Ensure() calls extend from that point under current rules.
func (p *Predictor) RebaseAt(idx int) {
	if idx < 0 {
		idx = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.beatInfosByRow)
	if len(p.audibleByRow) != n {
		p.resetBuffersLocked(n)
	}
	if len(p.lastFiredByRow) != n {
		p.lastFiredByRow = make([]model.NodeID, n)
	}
	if len(p.gateUntilByRow) != n {
		p.gateUntilByRow = make([]int, n)
	}
	if len(p.visGateUntilByRow) != n {
		p.visGateUntilByRow = make([]int, n)
		for i := range p.visGateUntilByRow {
			p.visGateUntilByRow[i] = -1
		}
	}
	for row := 0; row < n; row++ {
		if len(p.audibleByRow[row]) > idx {
			p.audibleByRow[row] = p.audibleByRow[row][:idx]
		}
		if len(p.visibleByRow[row]) > idx {
			p.visibleByRow[row] = p.visibleByRow[row][:idx]
		}
		if len(p.triggeredByRow[row]) > idx {
			p.triggeredByRow[row] = p.triggeredByRow[row][:idx]
		}
		if cap(p.audibleByRow[row]) < idx {
			tmp := make([]bool, len(p.audibleByRow[row]), idx)
			copy(tmp, p.audibleByRow[row])
			p.audibleByRow[row] = tmp
		}
		if cap(p.visibleByRow[row]) < idx {
			tmp := make([]bool, len(p.visibleByRow[row]), idx)
			copy(tmp, p.visibleByRow[row])
			p.visibleByRow[row] = tmp
		}
		if cap(p.triggeredByRow[row]) < idx {
			tmp := make([]bool, len(p.triggeredByRow[row]), idx)
			copy(tmp, p.triggeredByRow[row])
			p.triggeredByRow[row] = tmp
		}
		if len(p.audibleByRow[row]) < idx {
			p.audibleByRow[row] = p.audibleByRow[row][:idx]
		}
		if len(p.visibleByRow[row]) < idx {
			p.visibleByRow[row] = p.visibleByRow[row][:idx]
		}
		if len(p.triggeredByRow[row]) < idx {
			p.triggeredByRow[row] = p.triggeredByRow[row][:idx]
		}
		counts := make(map[model.NodeID]int)
		lastTrig := make(map[model.NodeID]bool)
		lastFired := model.InvalidNodeID
		gate := 0
		visGate := -1
		loop := row < len(p.isLoopByRow) && p.isLoopByRow[row]
		start := 0
		seg := 0
		if loop {
			if row < len(p.loopStartByRow) {
				start = p.loopStartByRow[row]
			}
			seg = p.loopLenByRow[row]
		}
		for i := 0; i < idx; i++ {
			bi := p.beatInfoAtRow(row, i)
			if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
				if i < len(p.audibleByRow[row]) {
					p.audibleByRow[row][i] = false
				}
				if i < len(p.visibleByRow[row]) {
					p.visibleByRow[row][i] = false
				}
				if i < len(p.triggeredByRow[row]) {
					p.triggeredByRow[row][i] = false
				}
				continue
			}
			audible, triggered := p.evalAudible(row, i, bi, counts, &lastFired, lastTrig, &gate)
			if i < len(p.audibleByRow[row]) {
				p.audibleByRow[row][i] = audible
			}
			if i < len(p.triggeredByRow[row]) {
				p.triggeredByRow[row][i] = triggered
			}
			vis := audible
			if bi.NodeType == model.NodeTypeMute && triggered {
				vis = true
				if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
					visGate = i + 1
				}
			} else if bi.NodeType == model.NodeTypeRegular {
				if visGate >= 0 && i <= visGate {
					vis = false
				}
			}
			if loop && seg > 0 && i >= start+1 {
				if (i-(start+1))%seg == 0 {
					if bi.NodeType == model.NodeTypeInvisible {
						vis = false
					}
				}
			}
			if i < len(p.visibleByRow[row]) {
				p.visibleByRow[row][i] = vis
			}
			if bi.NodeType == model.NodeTypeRegular {
				if audible {
					lastFired = bi.NodeID
					lastTrig[bi.NodeID] = true
				} else {
					lastTrig[bi.NodeID] = false
				}
			} else if bi.NodeType == model.NodeTypeMute {
				lastTrig[bi.NodeID] = triggered
			}
		}
		p.countsByRow[row] = counts
		p.lastTrigByRow[row] = lastTrig
		if row < len(p.lastFiredByRow) {
			p.lastFiredByRow[row] = lastFired
		}
		if row < len(p.gateUntilByRow) {
			p.gateUntilByRow[row] = gate
		}
		if row < len(p.visGateUntilByRow) {
			p.visGateUntilByRow[row] = visGate
		}
	}
	p.horizon = idx
}

func muteHoldStepsFromNode(n model.Node) int {
	// Mute nodes no longer carry a duration-based gate.
	return 0
}

func shouldGateMute(n model.Node) bool {
	if n.Type != model.NodeTypeMute {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(n.Params.LogicKind))
	if kind != "" && kind != "none" {
		return true
	}
	if n.Params.SkipEveryN > 0 {
		return true
	}
	return false
}

func (p *Predictor) shouldTriggerNode(row, idx int, bi model.BeatInfo, n model.Node, counts map[model.NodeID]int, lastFired *model.NodeID, lastTrig map[model.NodeID]bool) bool {
	if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
		return false
	}
	if n.Params.SkipEveryN > 0 && n.Params.LogicKind != "skip_every_n" && n.Params.LogicKind != "every_n_triggers" {
		counts[bi.NodeID] = counts[bi.NodeID] + 1
		if counts[bi.NodeID]%n.Params.SkipEveryN == 0 {
			return false
		}
	}
	kind := strings.ToLower(n.Params.LogicKind)
	switch kind {
	case "", "none":
		return true
	case "prev_fired":
		j := idx - 1
		for k := 0; k < 64; k++ {
			prev := p.beatInfoAtRow(row, j)
			if prev.NodeType == model.NodeTypeRegular {
				return lastFired != nil && *lastFired == prev.NodeID
			}
			j--
		}
		return false
	case "every_n_loops", "every_n_triggers":
		if n.Params.LogicN > 0 {
			if n.Type == model.NodeTypeMute {
				cycleLen := len(p.beatInfosByRow[row])
				if cycleLen <= 0 {
					cycleLen = 1
				}
				cycle := idx / cycleLen
				if (cycle+1)%n.Params.LogicN != 0 {
					return false
				}
			} else {
				counts[bi.NodeID] = counts[bi.NodeID] + 1
				if counts[bi.NodeID]%n.Params.LogicN != 0 {
					return false
				}
			}
		}
	case "skip_every_n":
		if n.Params.LogicN > 0 {
			counts[bi.NodeID] = counts[bi.NodeID] + 1
			if counts[bi.NodeID]%n.Params.LogicN == 0 {
				return false
			}
		}
	case "probability":
		pval := n.Params.LogicP
		if pval <= 0 {
			return false
		}
		if pval < 1 {
			if p.deterministicRoll(row, idx, bi.NodeID) > pval {
				return false
			}
		}
	case "trigger_if_prev_skipped", "trigger_if_prev_triggered", "skip_if_prev_skipped", "skip_if_prev_triggered":
		j := idx - 1
		prevID := model.InvalidNodeID
		for k := 0; k < 64; k++ {
			prev := p.beatInfoAtRow(row, j)
			if prev.NodeType == model.NodeTypeRegular {
				prevID = prev.NodeID
				break
			}
			j--
		}
		prevTrig := false
		if prevID != model.InvalidNodeID {
			if v, ok := lastTrig[prevID]; ok {
				prevTrig = v
			} else if lastFired != nil {
				prevTrig = (*lastFired == prevID)
			}
		}
		switch kind {
		case "trigger_if_prev_skipped":
			if prevID == model.InvalidNodeID || prevTrig {
				return false
			}
		case "trigger_if_prev_triggered":
			if prevID == model.InvalidNodeID || !prevTrig {
				return false
			}
		case "skip_if_prev_skipped":
			if prevID != model.InvalidNodeID && !prevTrig {
				return false
			}
		case "skip_if_prev_triggered":
			if prevID != model.InvalidNodeID && prevTrig {
				return false
			}
		}
		counts[bi.NodeID] = counts[bi.NodeID] + 1
	}
	return true
}

func (p *Predictor) evalAudible(row, idx int, bi model.BeatInfo, counts map[model.NodeID]int, lastFired *model.NodeID, lastTrig map[model.NodeID]bool, gate *int) (bool, bool) {
	if bi.NodeType == model.NodeTypeMute {
		nNode, ok := p.nodes[bi.NodeID]
		if !ok {
			if lastTrig != nil {
				lastTrig[bi.NodeID] = false
			}
			return false, false
		}
		trigger := p.shouldTriggerNode(row, idx, bi, nNode, counts, lastFired, lastTrig)
		if lastTrig != nil {
			lastTrig[bi.NodeID] = trigger
		}
		if trigger && gate != nil {
			next := idx + muteHoldStepsFromNode(nNode) + 1
			if next > *gate {
				*gate = next
			}
		}
		return false, trigger
	}
	if bi.NodeType != model.NodeTypeRegular {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if gate != nil && idx < *gate {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	nNode, ok := p.nodes[bi.NodeID]
	if !ok {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if !p.shouldTriggerNode(row, idx, bi, nNode, counts, lastFired, lastTrig) {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if lastTrig != nil {
		lastTrig[bi.NodeID] = true
	}
	if lastFired != nil {
		*lastFired = bi.NodeID
	}
	return true, true
}

// UpdateNode refreshes the cached node parameters for the given ID.
func (p *Predictor) UpdateNode(id model.NodeID, node model.Node) {
	p.mu.Lock()
	if p.nodes == nil {
		p.nodes = make(map[model.NodeID]model.Node)
	}
	p.nodes[id] = node
	p.predDirty = true
	p.mu.Unlock()
}

// DeleteNode removes a node from the cached snapshot.
func (p *Predictor) DeleteNode(id model.NodeID) {
	p.mu.Lock()
	delete(p.nodes, id)
	p.predDirty = true
	p.mu.Unlock()
}

// StartBackground spawns a low-priority goroutine to keep predictions ahead.
// target returns a desired horizon; the predictor will gradually converge.
func (p *Predictor) StartBackground(target func() int) {
	p.mu.Lock()
	p.targetFn = target
	quit := p.bgQuit
	p.mu.Unlock()
	go func() {
		// Low priority loop; ~8ms cadence keeps ahead without impacting UI.
		ticker := time.NewTicker(8 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				p.mu.RLock()
				tf := p.targetFn
				p.mu.RUnlock()
				if tf == nil {
					continue
				}
				want := tf()
				p.mu.RLock()
				cur := p.horizon
				p.mu.RUnlock()
				if want > cur {
					step := cur + 256
					if step > want {
						step = want
					}
					p.Ensure(step)
				}
			}
		}
	}()
}

// StopBackground stops the background precompute loop.
func (p *Predictor) StopBackground() { close(p.bgQuit) }

// AudibleAt returns predicted audible state for row/index.
func (p *Predictor) AudibleAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.audibleByRow) {
		return false
	}
	if idx < 0 || idx >= len(p.audibleByRow[row]) {
		return false
	}
	return p.audibleByRow[row][idx]
}

// VisibleAt returns predicted visible state (seam-masked) for row/index.
func (p *Predictor) VisibleAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.visibleByRow) {
		return false
	}
	if idx < 0 || idx >= len(p.visibleByRow[row]) {
		return false
	}
	return p.visibleByRow[row][idx]
}

// TriggeredAt reports whether the node at row/index fired (including mute nodes).
func (p *Predictor) TriggeredAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.triggeredByRow) {
		return false
	}
	if idx < 0 || idx >= len(p.triggeredByRow[row]) {
		return false
	}
	return p.triggeredByRow[row][idx]
}

// Snapshot returns deep copies of the prediction buffers, for tests.
func (p *Predictor) Snapshot() (aud [][]bool, vis [][]bool, horizon int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := len(p.audibleByRow)
	aud = make([][]bool, n)
	vis = make([][]bool, n)
	for i := 0; i < n; i++ {
		aud[i] = append([]bool(nil), p.audibleByRow[i]...)
		vis[i] = append([]bool(nil), p.visibleByRow[i]...)
	}
	horizon = p.horizon
	return
}

// beatInfoAtRow returns the BeatInfo for absolute idx on a row.
func (p *Predictor) beatInfoAtRow(row, idx int) model.BeatInfo {
	if row < 0 || row >= len(p.beatInfosByRow) {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	infos := p.beatInfosByRow[row]
	if len(infos) == 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx >= 0 && idx < len(infos) {
		return infos[idx]
	}
	if row < len(p.isLoopByRow) && !p.isLoopByRow[row] {
		if idx < 0 {
			return infos[0]
		}
		return infos[len(infos)-1]
	}
	loopLen := len(infos)
	start := 0
	if row < len(p.loopStartByRow) {
		start = p.loopStartByRow[row]
	}
	loopLen = len(infos) - start
	if loopLen <= 0 {
		if idx < 0 {
			return infos[start]
		}
		return infos[len(infos)-1]
	}
	rel := idx - start
	rel = rel % loopLen
	if rel < 0 {
		rel += loopLen
	}
	return infos[start+rel]
}

func loopSegmentLen(path []model.BeatInfo, start int) int {
	if start < 0 || start >= len(path) {
		return 0
	}
	origin := path[start].NodeID
	for i := start + 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i - start
		}
	}
	return len(path) - start
}

// deterministicRoll mirrors UI's hash-based roll for probability logic.
func (p *Predictor) deterministicRoll(row, idx int, id model.NodeID) float64 {
	x := uint64(uint32(row))<<32 ^ uint64(uint32(idx)) ^ (uint64(uint32(id)) << 16) ^ 0x9E3779B97F4A7C15
	x += 0x9E3779B97F4A7C15
	z := x
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= (z >> 31)
	return float64(z&((1<<53)-1)) / float64(1<<53)
}

// no custom tickers; standard time.Ticker used
