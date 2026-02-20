package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (g *Game) notifyPredictorNode(id model.NodeID) {
	if g.engine == nil || g.engine.Predictor == nil {
		return
	}
	if node, ok := g.nodeSnapshot(id); ok {
		g.engine.Predictor.UpdateNode(id, node)
		return
	}
	if node, ok := g.graph.GetNodeByID(id); ok {
		g.cacheNode(id)
		g.engine.Predictor.UpdateNode(id, node)
		return
	}
	g.engine.Predictor.DeleteNode(id)
	g.removeNodeCache(id)
}
