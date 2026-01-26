package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func (g *Game) setLastTriggered(row int, id model.NodeID, val bool) {
	g.triggerMu.Lock()
	if g.lastTriggeredByRow == nil {
		g.lastTriggeredByRow = make(map[int]map[model.NodeID]bool)
	}
	if g.lastTriggeredByRow[row] == nil {
		g.lastTriggeredByRow[row] = make(map[model.NodeID]bool)
	}
	g.lastTriggeredByRow[row][id] = val
	g.triggerMu.Unlock()
}

func (g *Game) cacheNode(id model.NodeID) {
	if g.graph == nil {
		return
	}
	node, ok := g.graph.Nodes[id]
	g.nodeCacheMu.Lock()
	if g.nodeCache == nil {
		g.nodeCache = make(map[model.NodeID]model.Node)
	}
	if ok {
		g.nodeCache[id] = node
	} else {
		delete(g.nodeCache, id)
	}
	g.nodeCacheMu.Unlock()
}

func (g *Game) removeNodeCache(id model.NodeID) {
	g.nodeCacheMu.Lock()
	if g.nodeCache != nil {
		delete(g.nodeCache, id)
	}
	g.nodeCacheMu.Unlock()
}

func (g *Game) rebuildNodeCache() {
	if g.graph == nil {
		return
	}
	g.nodeCacheMu.Lock()
	if g.nodeCache == nil {
		g.nodeCache = make(map[model.NodeID]model.Node, len(g.graph.Nodes))
	} else {
		for k := range g.nodeCache {
			delete(g.nodeCache, k)
		}
	}
	for id, node := range g.graph.Nodes {
		g.nodeCache[id] = node
	}
	g.nodeCacheMu.Unlock()
}

func (g *Game) nodeSnapshot(id model.NodeID) (model.Node, bool) {
	g.nodeCacheMu.RLock()
	node, ok := g.nodeCache[id]
	g.nodeCacheMu.RUnlock()
	return node, ok
}

func (g *Game) lastTriggered(row int, id model.NodeID) (bool, bool) {
	g.triggerMu.RLock()
	defer g.triggerMu.RUnlock()
	if g.lastTriggeredByRow == nil {
		return false, false
	}
	if m := g.lastTriggeredByRow[row]; m != nil {
		v, ok := m[id]
		return v, ok
	}
	return false, false
}

func (g *Game) copyLastTriggeredRow(row int, dst map[model.NodeID]bool) map[model.NodeID]bool {
	g.triggerMu.RLock()
	defer g.triggerMu.RUnlock()
	if g.lastTriggeredByRow == nil {
		if dst != nil {
			clear(dst)
		}
		return dst
	}
	if m := g.lastTriggeredByRow[row]; m != nil {
		if dst == nil {
			dst = make(map[model.NodeID]bool, len(m))
		} else {
			clear(dst)
		}
		for id, v := range m {
			dst[id] = v
		}
		return dst
	}
	if dst != nil {
		clear(dst)
	}
	return dst
}

func (g *Game) clearLastTriggeredRow(row int) {
	g.triggerMu.Lock()
	if g.lastTriggeredByRow != nil {
		delete(g.lastTriggeredByRow, row)
	}
	g.triggerMu.Unlock()
}

// Test-only helpers to interact with lastTriggeredByRow without racing the
// sequencer goroutines. They simply wrap the locked versions above.
func (g *Game) setLastTriggeredForTest(row int, id model.NodeID, v bool) {
	g.setLastTriggered(row, id, v)
}

func (g *Game) lastTriggeredForTest(row int, id model.NodeID) (bool, bool) {
	return g.lastTriggered(row, id)
}

func (g *Game) lastTriggeredRowSnapshotForTest(row int) map[model.NodeID]bool {
	return g.copyLastTriggeredRow(row, nil)
}
