package engine

import "github.com/ingyamilmolinar/tunkul/core/model"

// SetPaths replaces the per-row traversal info. Call when the graph or row
// origins change. The optional nodes snapshot lets callers avoid touching the
// live graph from background predictor goroutines. Slices are copied for
// thread-safety.
func (p *Predictor) SetPaths(paths [][]model.BeatInfo, isLoop []bool, loopStart []int, nodes map[model.NodeID]model.Node) {
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
	if nodes != nil {
		snapshot := make(map[model.NodeID]model.Node, len(nodes))
		for id, node := range nodes {
			snapshot[id] = node
		}
		p.nodes = snapshot
	} else if p.graph != nil {
		fallback := make(map[model.NodeID]model.Node, len(p.graph.Nodes))
		for id, node := range p.graph.Nodes {
			fallback[id] = node
		}
		p.nodes = fallback
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
	p.triggerCountsByRow = make(map[int]map[model.NodeID]int)
	p.lastTrigByRow = make(map[int]map[model.NodeID]bool)
	p.lastFiredByRow = make([]model.NodeID, n)
	p.gateUntilByRow = make([]int, n)
	p.visGateUntilByRow = make([]int, n)
	for i := range p.visGateUntilByRow {
		p.visGateUntilByRow[i] = -1
	}
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
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
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
