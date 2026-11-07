package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (s *playbackSlate) setLastTriggered(row int, id model.NodeID, val bool) {
	g := s.g
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

func (s *playbackSlate) cacheNode(id model.NodeID) {
	g := s.g
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

func (s *playbackSlate) removeNodeCache(id model.NodeID) {
	g := s.g
	g.nodeCacheMu.Lock()
	if g.nodeCache != nil {
		delete(g.nodeCache, id)
	}
	g.nodeCacheMu.Unlock()
}

func (s *playbackSlate) rebuildNodeCache() {
	g := s.g
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

func (s *playbackSlate) nodeSnapshot(id model.NodeID) (model.Node, bool) {
	g := s.g
	g.nodeCacheMu.RLock()
	node, ok := g.nodeCache[id]
	g.nodeCacheMu.RUnlock()
	return node, ok
}

func (s *playbackSlate) lastTriggered(row int, id model.NodeID) (bool, bool) {
	g := s.g
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

func (s *playbackSlate) setLastTriggeredForTest(row int, id model.NodeID, v bool) {
	g := s.g
	g.setLastTriggered(row, id, v)
}

func (s *playbackSlate) lastTriggeredForTest(row int, id model.NodeID) (bool, bool) {
	g := s.g
	return g.lastTriggered(row, id)
}

func (g *Game) setLastTriggered(row int, id model.NodeID, val bool) {
	g.playbackSlate.setLastTriggered(row, id, val)
}

func (g *Game) cacheNode(id model.NodeID) { g.playbackSlate.cacheNode(id) }

func (g *Game) removeNodeCache(id model.NodeID) { g.playbackSlate.removeNodeCache(id) }

func (g *Game) rebuildNodeCache() { g.playbackSlate.rebuildNodeCache() }

func (g *Game) nodeSnapshot(id model.NodeID) (model.Node, bool) {
	return g.playbackSlate.nodeSnapshot(id)
}

func (g *Game) lastTriggered(row int, id model.NodeID) (bool, bool) {
	return g.playbackSlate.lastTriggered(row, id)
}

func (g *Game) setLastTriggeredForTest(row int, id model.NodeID, v bool) {
	g.playbackSlate.setLastTriggeredForTest(row, id, v)
}

func (g *Game) lastTriggeredForTest(row int, id model.NodeID) (bool, bool) {
	return g.playbackSlate.lastTriggeredForTest(row, id)
}
