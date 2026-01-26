package engine

import (
	"fmt"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure grows prediction buffers to at least horizon.
func (p *Predictor) Ensure(horizon int) {
	if horizon < 0 {
		horizon = 0
	}
	var ensureStart time.Time
	logLargeHorizon := horizon > 500
	if logLargeHorizon {
		ensureStart = time.Now()
		fmt.Printf("[PREDICTOR] Ensure called with large horizon=%d predDirty=%v currentHorizon=%d rows=%d\n",
			horizon, p.predDirty, p.horizon, len(p.beatInfosByRow))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.predDirty && horizon <= p.horizon {
		if logLargeHorizon {
			fmt.Printf("[PREDICTOR] Ensure early-exit (not dirty, horizon satisfied) elapsed=%v\n", time.Since(ensureStart))
		}
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
	if p.predDirty {
		startIdx = 0
	}
	for row := 0; row < n; row++ {
		counts := p.countsByRow[row]
		if counts == nil {
			counts = make(map[model.NodeID]int)
		}
		triggerCounts := p.triggerCountsByRow[row]
		if triggerCounts == nil {
			triggerCounts = make(map[model.NodeID]int)
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
			triggerCounts = make(map[model.NodeID]int)
			lastTrig = make(map[model.NodeID]bool)
			lastFired = model.InvalidNodeID
			gate = 0
			visGate = -1
			for i := 0; i < startIdx; i++ {
				bi := p.beatInfoAtRow(row, i)
				if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
					continue
				}
				_, triggered := p.evalAudible(row, i, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
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
			audible, triggered := p.evalAudible(row, idx, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
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
		p.triggerCountsByRow[row] = triggerCounts
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
	// Clear dirty flag after bringing predictions current. Subsequent Ensure
	// calls at the same horizon should be O(1) unless inputs change again.
	p.predDirty = false
	if logLargeHorizon {
		fmt.Printf("[PREDICTOR] Ensure completed horizon=%d elapsed=%v\n", horizon, time.Since(ensureStart))
	}
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
		triggerCounts := make(map[model.NodeID]int)
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
			audible, triggered := p.evalAudible(row, i, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
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
		p.triggerCountsByRow[row] = triggerCounts
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
	// Rebase reconstructs contexts up to idx; future Ensure() should extend
	// incrementally from here under current rules.
	p.predDirty = false
}
