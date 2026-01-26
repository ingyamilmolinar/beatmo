package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func (g *Game) rowIndexForNode(id model.NodeID) int {
	if g.nodeRows != nil {
		if r, ok := g.nodeRows[id]; ok {
			return r
		}
	}
	for r := range g.beatInfosByRow {
		for _, bi := range g.beatInfosByRow[r] {
			if bi.NodeID == id {
				return r
			}
		}
	}
	return -1
}

func (g *Game) resetLogicStateForNode(id model.NodeID) {
	row := g.rowIndexForNode(id)
	if row >= 0 {
		g.resetLogicStateForRow(row)
	}
}

func (g *Game) resetLogicStateForRow(row int) {
	if row < 0 {
		return
	}
	if g.nodeTriggerCountsByRow == nil {
		g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	}
	g.nodeTriggerCountsByRow[row] = make(map[model.NodeID]int)
	if g.nodeLogicTriggerCountsByRow == nil {
		g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	}
	g.nodeLogicTriggerCountsByRow[row] = make(map[model.NodeID]int)
	if g.lastEvalIdxByRowNode != nil {
		delete(g.lastEvalIdxByRowNode, row)
	}
	g.triggerMu.Lock()
	if g.lastTriggeredByRow == nil {
		g.lastTriggeredByRow = make(map[int]map[model.NodeID]bool)
	}
	g.lastTriggeredByRow[row] = make(map[model.NodeID]bool)
	g.triggerMu.Unlock()
	if row < len(g.lastFiredNodeByRow) {
		g.lastFiredNodeByRow[row] = model.InvalidNodeID
	}
}

func (g *Game) clampRowFreeze(row int, limit int) {
	if row < 0 {
		return
	}
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
		for i := range g.frozenUpToByRow {
			g.frozenUpToByRow[i] = -1
		}
	}
	if row >= len(g.frozenUpToByRow) {
		return
	}
	if limit < -1 {
		limit = -1
	}
	if g.frozenUpToByRow[row] > limit {
		g.frozenUpToByRow[row] = limit
	}
}
