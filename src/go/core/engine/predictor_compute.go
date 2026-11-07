package engine

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Doubles cap, so single-step extensions amortise to O(1); at max both length and cap plateau and Ensure slides in
// place. Per-advance reallocation OOMed WASM's 2 GB ceiling; geometric growth alone still kept tens of MB in long sessions.
func growBoolBuf(buf []bool, n, max int) []bool {
	if max > 0 && n > max {
		n = max
	}
	if n <= len(buf) {
		return buf
	}
	if n <= cap(buf) {
		return buf[:n]
	}
	newCap := cap(buf) * 2
	if newCap < n {
		newCap = n
	}
	if max > 0 && newCap > max {
		newCap = max
	}
	if newCap < n {
		newCap = n
	}
	tmp := make([]bool, n, newCap)
	copy(tmp, buf)
	return tmp
}

// Keeps the capacity, so later writes extend without reallocating.
func slideBoolSliceInPlace(buf []bool, delta int) []bool {
	if delta <= 0 || len(buf) == 0 {
		return buf
	}
	if delta >= len(buf) {
		// Slide consumed the entire valid range; clear and truncate.
		for i := range buf {
			buf[i] = false
		}
		return buf[:0]
	}
	copy(buf, buf[delta:])
	// So reused slots never leak stale predictions if a later horizon is shorter (defensive).
	for i := len(buf) - delta; i < len(buf); i++ {
		buf[i] = false
	}
	return buf[:len(buf)-delta]
}

// slideAllRowsLocked advances windowStart by delta, dropping the oldest delta
// entries from every per-row buffer. Caller must hold p.mu.
func (p *Predictor) slideAllRowsLocked(delta int) {
	if delta <= 0 {
		return
	}
	for row := range p.audibleByRow {
		p.audibleByRow[row] = slideBoolSliceInPlace(p.audibleByRow[row], delta)
		p.visibleByRow[row] = slideBoolSliceInPlace(p.visibleByRow[row], delta)
		p.triggeredByRow[row] = slideBoolSliceInPlace(p.triggeredByRow[row], delta)
	}
	p.windowStart += delta
	if p.windowEnd < p.windowStart {
		p.windowEnd = p.windowStart
	}
}

// Past windowCap subdivisions per row, advances slide the window in place instead of growing: memory stays
// O(rows × 3 × windowCap) however long the session.
func (p *Predictor) Ensure(horizon int) {
	if horizon < 0 {
		horizon = 0
	}
	var ensureStart time.Time
	logLargeHorizon := horizon > 500 && p.logger != nil
	if logLargeHorizon {
		ensureStart = time.Now()
		p.logger.Debugf("[predictor] Ensure called with large horizon=%d predDirty=%v windowStart=%d windowEnd=%d windowCap=%d rows=%d",
			horizon, p.predDirty, p.windowStart, p.windowEnd, p.windowCap, len(p.beatInfosByRow))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.windowCap <= 0 {
		p.windowCap = defaultPredictorWindowCap
	}
	// Not when the UI scrolled below windowStart: then the LOWER edge is missing and the backward extend must run.
	if !p.predDirty && horizon <= p.windowEnd && (p.visibleMinAbs < 0 || p.visibleMinAbs >= p.windowStart) {
		if logLargeHorizon {
			p.logger.Debugf("[predictor] Ensure early-exit (not dirty, horizon satisfied) elapsed=%v", time.Since(ensureStart))
		}
		return
	}
	// Ensure row containers
	n := len(p.beatInfosByRow)
	if len(p.audibleByRow) != n {
		p.resetBuffersLocked(n)
	}

	// The visible region scrolled below the window: restart it at visibleMinAbs and force a dirty rebuild, or a scroll-back
	// after long playback reads false everywhere and the grid renders grey.
	if p.visibleMinAbs >= 0 && p.visibleMinAbs < p.windowStart {
		for row := 0; row < n; row++ {
			if len(p.audibleByRow[row]) > 0 {
				p.audibleByRow[row] = p.audibleByRow[row][:0]
			}
			if len(p.visibleByRow[row]) > 0 {
				p.visibleByRow[row] = p.visibleByRow[row][:0]
			}
			if len(p.triggeredByRow[row]) > 0 {
				p.triggeredByRow[row] = p.triggeredByRow[row][:0]
			}
		}
		p.windowStart = p.visibleMinAbs
		p.windowEnd = p.visibleMinAbs
		p.predDirty = true
	}

	// A dirty rebuild covers the whole previously valid range, or stale entries past horizon would survive the early exit.
	if p.predDirty && horizon < p.windowEnd {
		horizon = p.windowEnd
	}

	// Before choosing mainStart, so the rebuild uses the post-slide windowStart. Clamped at visibleMinAbs so the window never
	// evicts cells on screen (the buffer may exceed windowCap for the active region).
	if horizon-p.windowStart > p.windowCap {
		newStart := horizon - p.windowCap
		if p.visibleMinAbs >= 0 && newStart > p.visibleMinAbs {
			newStart = p.visibleMinAbs
		}
		if delta := newStart - p.windowStart; delta > 0 {
			p.slideAllRowsLocked(delta)
		}
	}

	// Dirty: the per-row context resets and re-walks [0, windowStart) so the main loop sees correct cumulative state.
	startIdx := p.windowEnd
	if p.predDirty {
		startIdx = p.windowStart
	}
	// The bounded rebuild needs no custom Logic funcs: their triggerCounts depend on real trigger history, while built-in
	// rules use only counts (loop geometry) and last-loop state.
	hasCustomLogic := false
	for _, nd := range p.nodes {
		if nd.Params.Logic != nil {
			hasCustomLogic = true
			break
		}
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
			// A full re-walk is O(session) per edit, which starved the scheduler mid-song. With built-in logic on a looping row,
			// counts are loop geometry (seeded analytically) and last-loop state settles within a couple of loops, so only those are
			// re-walked; boundedRewalkStart returns 0 (full walk) when unsafe.
			rewalkFrom := 0
			if !p.forceFullRebuild {
				rewalkFrom = p.boundedRewalkStart(row, startIdx, loop, start, seg, hasCustomLogic, counts)
			}
			for i := rewalkFrom; i < startIdx; i++ {
				bi := p.beatInfoAtRow(row, i)
				if !bi.NodeType.Fires() && bi.NodeType != model.NodeTypeMute {
					continue
				}
				_, triggered := p.evalAudible(row, i, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
				if bi.NodeType == model.NodeTypeMute && triggered {
					if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
						visGate = i + 1
					}
				}
				switch bi.NodeType {
				case model.NodeTypeRegular, model.NodeTypeChord:
					if triggered {
						lastFired = bi.NodeID
						lastTrig[bi.NodeID] = true
					} else {
						lastTrig[bi.NodeID] = false
					}
				case model.NodeTypeMute:
					lastTrig[bi.NodeID] = triggered
				}
			}
		}

		// Grow buffer to cover [windowStart, horizon). Length tracks the valid
		// range; capacity plateaus at windowCap.
		need := horizon - p.windowStart
		if need < 0 {
			need = 0
		}
		p.audibleByRow[row] = growBoolBuf(p.audibleByRow[row], need, p.windowCap)
		p.visibleByRow[row] = growBoolBuf(p.visibleByRow[row], need, p.windowCap)
		p.triggeredByRow[row] = growBoolBuf(p.triggeredByRow[row], need, p.windowCap)

		for idx := startIdx; idx < horizon; idx++ {
			rel := idx - p.windowStart
			if rel < 0 || rel >= len(p.audibleByRow[row]) {
				continue
			}
			bi := p.beatInfoAtRow(row, idx)
			audible, triggered := p.evalAudible(row, idx, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
			p.audibleByRow[row][rel] = audible
			p.triggeredByRow[row][rel] = triggered
			vis := audible
			if bi.NodeType == model.NodeTypeMute && triggered {
				vis = true
				if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
					visGate = idx + 1
				}
			} else if bi.NodeType.Fires() {
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
			p.visibleByRow[row][rel] = vis
			switch bi.NodeType {
			case model.NodeTypeRegular, model.NodeTypeChord:
				if audible {
					lastFired = bi.NodeID
					lastTrig[bi.NodeID] = true
				} else {
					lastTrig[bi.NodeID] = false
				}
			case model.NodeTypeMute:
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
	if horizon > p.windowEnd {
		p.windowEnd = horizon
	}
	// Clear dirty flag after bringing predictions current. Subsequent Ensure
	// calls at the same horizon should be O(1) unless inputs change again.
	p.predDirty = false
	if logLargeHorizon {
		p.logger.Debugf("[predictor] Ensure completed horizon=%d windowStart=%d windowEnd=%d elapsed=%v",
			horizon, p.windowStart, p.windowEnd, time.Since(ensureStart))
	}
}

// Below the window start the buffers reset to start at idx: evicted predictions cannot be recovered.
func (p *Predictor) RebaseAt(idx int) {
	if idx < 0 {
		idx = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epoch.Add(1)
	if p.windowCap <= 0 {
		p.windowCap = defaultPredictorWindowCap
	}
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
	// If the rebase target is below windowStart, the predictor cannot
	// reconstruct evicted state; reset the window to start at idx.
	if idx < p.windowStart {
		for row := 0; row < n; row++ {
			if len(p.audibleByRow[row]) > 0 {
				for i := range p.audibleByRow[row] {
					p.audibleByRow[row][i] = false
				}
				p.audibleByRow[row] = p.audibleByRow[row][:0]
			}
			if len(p.visibleByRow[row]) > 0 {
				for i := range p.visibleByRow[row] {
					p.visibleByRow[row][i] = false
				}
				p.visibleByRow[row] = p.visibleByRow[row][:0]
			}
			if len(p.triggeredByRow[row]) > 0 {
				for i := range p.triggeredByRow[row] {
					p.triggeredByRow[row][i] = false
				}
				p.triggeredByRow[row] = p.triggeredByRow[row][:0]
			}
		}
		p.windowStart = idx
		p.windowEnd = idx
	} else {
		// Trim buffers to [windowStart, idx).
		newLen := idx - p.windowStart
		if newLen < 0 {
			newLen = 0
		}
		for row := 0; row < n; row++ {
			if len(p.audibleByRow[row]) > newLen {
				for i := newLen; i < len(p.audibleByRow[row]); i++ {
					p.audibleByRow[row][i] = false
				}
				p.audibleByRow[row] = p.audibleByRow[row][:newLen]
			}
			if len(p.visibleByRow[row]) > newLen {
				for i := newLen; i < len(p.visibleByRow[row]); i++ {
					p.visibleByRow[row][i] = false
				}
				p.visibleByRow[row] = p.visibleByRow[row][:newLen]
			}
			if len(p.triggeredByRow[row]) > newLen {
				for i := newLen; i < len(p.triggeredByRow[row]); i++ {
					p.triggeredByRow[row][i] = false
				}
				p.triggeredByRow[row] = p.triggeredByRow[row][:newLen]
			}
		}
	}
	for row := 0; row < n; row++ {
		// Grow capacity geometrically — same rationale as Ensure.
		need := idx - p.windowStart
		if need < 0 {
			need = 0
		}
		p.audibleByRow[row] = growBoolBuf(p.audibleByRow[row], need, p.windowCap)
		p.visibleByRow[row] = growBoolBuf(p.visibleByRow[row], need, p.windowCap)
		p.triggeredByRow[row] = growBoolBuf(p.triggeredByRow[row], need, p.windowCap)
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
		// Walk [0, idx) to reconstruct per-row context. The buffer writes only
		// land inside the retained window [windowStart, idx).
		for i := 0; i < idx; i++ {
			bi := p.beatInfoAtRow(row, i)
			rel := i - p.windowStart
			inWindow := rel >= 0 && rel < len(p.audibleByRow[row])
			if !bi.NodeType.Fires() && bi.NodeType != model.NodeTypeMute {
				if inWindow {
					p.audibleByRow[row][rel] = false
					p.visibleByRow[row][rel] = false
					p.triggeredByRow[row][rel] = false
				}
				continue
			}
			audible, triggered := p.evalAudible(row, i, bi, counts, triggerCounts, &lastFired, lastTrig, &gate)
			if inWindow {
				p.audibleByRow[row][rel] = audible
				p.triggeredByRow[row][rel] = triggered
			}
			vis := audible
			if bi.NodeType == model.NodeTypeMute && triggered {
				vis = true
				if n, ok := p.nodes[bi.NodeID]; ok && shouldGateMute(n) {
					visGate = i + 1
				}
			} else if bi.NodeType.Fires() {
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
			if inWindow {
				p.visibleByRow[row][rel] = vis
			}
			switch bi.NodeType {
			case model.NodeTypeRegular, model.NodeTypeChord:
				if audible {
					lastFired = bi.NodeID
					lastTrig[bi.NodeID] = true
				} else {
					lastTrig[bi.NodeID] = false
				}
			case model.NodeTypeMute:
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
	p.windowEnd = idx
	if p.windowEnd < p.windowStart {
		p.windowEnd = p.windowStart
	}
	// Rebase reconstructs contexts up to idx; future Ensure() should extend
	// incrementally from here under current rules.
	p.predDirty = false
}
