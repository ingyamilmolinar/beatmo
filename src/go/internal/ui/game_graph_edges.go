package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// addEdgeNoRefresh records an orthogonal edge without triggering expensive
// rebuilds. It returns true when a new edge was added.
func (g *Game) addEdgeNoRefresh(a, b *uiNode) bool {
	if !(a.I == b.I || a.J == b.J) { // only orthogonal
		return false
	}
	for _, e := range g.edges { // avoid exact duplicate in same direction; allow opposite direction
		if e.A == a && e.B == b {
			return false
		}
	}

	// Record UI edge and single graph edge between regular endpoints only.
	g.edges = append(g.edges, uiEdge{A: a, B: b, t: 0, pulse: -1})
	g.graph.Edges[[2]model.NodeID{a.ID, b.ID}] = struct{}{}
	g.logger.Debugf("[GAME] Added edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.logger.Infof("[GAME] Edge created from id=%d grid=(%d,%d) to id=%d grid=(%d,%d)", a.ID, a.I, a.J, b.ID, b.I, b.J)
	g.edgesDirty = true
	return true
}

func (g *Game) addEdge(a, b *uiNode) {
	if !g.addEdgeNoRefresh(a, b) {
		return
	}
	if g.importing {
		return
	}
	if g.graph.StartNodeID != model.InvalidNodeID {
		g.updateBeatInfos()
	}
	g.computeSelNeighbors()
}

// deleteEdgeNoRefresh removes an edge without rebuilding paths. Returns true
// when an edge was removed.
func (g *Game) deleteEdgeNoRefresh(a, b *uiNode) bool {
	removed := false
	for i := 0; i < len(g.edges); {
		e := g.edges[i]
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			g.edges[i] = g.edges[len(g.edges)-1]
			g.edges = g.edges[:len(g.edges)-1]
			removed = true
		} else {
			i++
		}
	}
	if removed {
		delete(g.graph.Edges, [2]model.NodeID{a.ID, b.ID})
		g.logger.Debugf("[GAME] Deleted edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
		g.logger.Infof("[GAME] Edge deleted from id=%d grid=(%d,%d) to id=%d grid=(%d,%d)", a.ID, a.I, a.J, b.ID, b.I, b.J)
		g.edgesDirty = true
	}
	return removed
}

func (g *Game) deleteEdge(a, b *uiNode) {
	if !g.deleteEdgeNoRefresh(a, b) {
		return
	}
	if g.importing {
		return
	}
	g.updateBeatInfos()
	g.computeSelNeighbors()
}
