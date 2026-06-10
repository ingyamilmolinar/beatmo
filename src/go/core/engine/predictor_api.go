package engine

import "github.com/ingyamilmolinar/beatmo/core/model"

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

// PredDirtyForTest exposes the predictor dirty flag to tests. It is a no-op in
// production paths but helps verify rebuild behaviour from external packages.
func (p *Predictor) PredDirtyForTest() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.predDirty
}

// Horizon returns the highest absolute index for which predictions have been
// computed (the exclusive upper bound of the sliding window).
func (p *Predictor) Horizon() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.windowEnd
}

// AudibleAt returns predicted audible state for row/index. Returns false for
// idx outside the current sliding window (idx < windowStart is evicted history;
// idx >= windowEnd is unwritten future).
func (p *Predictor) AudibleAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.audibleByRow) {
		return false
	}
	j := idx - p.windowStart
	if j < 0 || j >= len(p.audibleByRow[row]) {
		return false
	}
	return p.audibleByRow[row][j]
}

// VisibleAt returns predicted visible state (seam-masked) for row/index.
// Returns false for idx outside the current sliding window.
func (p *Predictor) VisibleAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.visibleByRow) {
		return false
	}
	j := idx - p.windowStart
	if j < 0 || j >= len(p.visibleByRow[row]) {
		return false
	}
	return p.visibleByRow[row][j]
}

// TriggeredAt reports whether the node at row/index fired (including mute
// nodes). Returns false for idx outside the current sliding window.
func (p *Predictor) TriggeredAt(row, idx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if row < 0 || row >= len(p.triggeredByRow) {
		return false
	}
	j := idx - p.windowStart
	if j < 0 || j >= len(p.triggeredByRow[row]) {
		return false
	}
	return p.triggeredByRow[row][j]
}

// BufferCapsForTest returns per-row [cap, len] pairs for each of the three
// prediction buffers (audible, visible, triggered). Soak tests use this to
// verify Ensure grows geometrically rather than per-step. With per-step
// regression cap == len for every buffer; with geometric growth cap > len
// for some buffers once horizon has advanced past the initial doubling.
func (p *Predictor) BufferCapsForTest() [][3][2]int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := len(p.audibleByRow)
	out := make([][3][2]int, n)
	for r := 0; r < n; r++ {
		out[r][0] = [2]int{cap(p.audibleByRow[r]), len(p.audibleByRow[r])}
		out[r][1] = [2]int{cap(p.visibleByRow[r]), len(p.visibleByRow[r])}
		out[r][2] = [2]int{cap(p.triggeredByRow[r]), len(p.triggeredByRow[r])}
	}
	return out
}

// Snapshot returns deep copies of the prediction buffers, for tests. The
// returned buffers cover [windowStart, windowEnd) — callers that key on absolute
// indices must offset by Snapshot's third return value's windowStart equivalent
// or use AudibleAt/VisibleAt for absolute-index lookups.
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
	horizon = p.windowEnd
	return
}

// WindowBoundsForTest returns the current sliding-window absolute-index bounds
// (start inclusive, end exclusive) and the configured per-row cap. Only used
// by tests asserting the windowing/eviction policy.
func (p *Predictor) WindowBoundsForTest() (start, end, cap int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.windowStart, p.windowEnd, p.windowCap
}
