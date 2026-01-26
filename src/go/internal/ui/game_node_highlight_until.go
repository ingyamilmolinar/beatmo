package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// highlightWindow represents a time window during which a node highlight is active.
// The highlight should only be visible when start <= audio.Now() < end.
type highlightWindow struct {
	start float64
	end   float64
}

func (g *Game) setNodeHighlightUntil(id model.NodeID, start, end float64) {
	g.triggerMu.Lock()
	if g.nodeHLUntil == nil {
		g.nodeHLUntil = make(map[model.NodeID]highlightWindow)
	}
	g.nodeHLUntil[id] = highlightWindow{start: start, end: end}
	g.triggerMu.Unlock()
}

func (g *Game) nodeHighlightUntil(id model.NodeID) (start, end float64, ok bool) {
	g.triggerMu.RLock()
	defer g.triggerMu.RUnlock()
	if g.nodeHLUntil == nil {
		return 0, 0, false
	}
	val, ok := g.nodeHLUntil[id]
	return val.start, val.end, ok
}

func (g *Game) clearNodeHighlight(id model.NodeID) {
	g.triggerMu.Lock()
	if g.nodeHLUntil != nil {
		delete(g.nodeHLUntil, id)
	}
	g.triggerMu.Unlock()
}
