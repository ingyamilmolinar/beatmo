package engine

import "github.com/ingyamilmolinar/tunkul/core/model"

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
