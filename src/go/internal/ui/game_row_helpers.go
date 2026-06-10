package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
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
	// Any clamp invalidates that row's reconciliation watermark — entries
	// strictly above the new clamp are no longer frozen, and entries at or
	// below the clamp may have been mutated by the trim/release path that
	// triggered this clamp. Force a re-scan from futureReleaseStart next
	// refresh.
	g.invalidateReconciledRow(row)
}

// invalidateReconciledRow resets row r's reconciliation watermark so the next
// refresh's reconcileFrozen scan starts from futureReleaseStart. Out-of-range
// row values are silently ignored.
func (g *Game) invalidateReconciledRow(row int) {
	if row < 0 || row >= len(g.reconciledUpToByRow) {
		return
	}
	g.reconciledUpToByRow[row] = -1
}

// invalidateReconciledAll resets every row's reconciliation watermark. Used
// when an event affects all rows globally (predictor SetPaths, Stop reset,
// row count grow).
func (g *Game) invalidateReconciledAll() {
	for i := range g.reconciledUpToByRow {
		g.reconciledUpToByRow[i] = -1
	}
}
