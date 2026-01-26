package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func (g *Game) highlightSet(key int, val int64) {
	g.highlightMu.Lock()
	g.highlightedBeats[key] = val
	g.highlightMu.Unlock()
}

func (g *Game) highlightDelete(key int) {
	g.highlightMu.Lock()
	delete(g.highlightedBeats, key)
	g.highlightMu.Unlock()
}

func (g *Game) highlightSnapshot() map[int]int64 {
	g.highlightMu.RLock()
	if len(g.highlightedBeats) == 0 {
		g.highlightMu.RUnlock()
		return nil
	}
	copy := make(map[int]int64, len(g.highlightedBeats))
	for k, v := range g.highlightedBeats {
		copy[k] = v
	}
	g.highlightMu.RUnlock()
	return copy
}

func (g *Game) hasHighlight(row, idx int) bool {
	key := makeBeatKey(row, idx)
	g.highlightMu.RLock()
	_, ok := g.highlightedBeats[key]
	g.highlightMu.RUnlock()
	return ok
}

func (g *Game) lastNodeHLReset() {
	g.lastNodeHLMu.Lock()
	if g.lastNodeHL == nil {
		g.lastNodeHL = make(map[model.NodeID]bool)
	} else {
		for k := range g.lastNodeHL {
			delete(g.lastNodeHL, k)
		}
	}
	g.lastNodeHLMu.Unlock()
}

func (g *Game) lastNodeHLMark(id model.NodeID) {
	g.lastNodeHLMu.Lock()
	if g.lastNodeHL == nil {
		g.lastNodeHL = make(map[model.NodeID]bool)
	}
	g.lastNodeHL[id] = true
	g.lastNodeHLMu.Unlock()
}

func (g *Game) lastNodeHLHas(id model.NodeID) bool {
	g.lastNodeHLMu.RLock()
	ok := g.lastNodeHL != nil && g.lastNodeHL[id]
	g.lastNodeHLMu.RUnlock()
	return ok
}

func (g *Game) nodeAnimSet(id model.NodeID, val float64) {
	g.nodeAnimMu.Lock()
	g.nodeAnim[id] = val
	g.nodeAnimMu.Unlock()
}

func (g *Game) nodeAnimGet(id model.NodeID) float64 {
	g.nodeAnimMu.RLock()
	val := g.nodeAnim[id]
	g.nodeAnimMu.RUnlock()
	return val
}

func (g *Game) clearExpiredHighlights() {
	g.highlightMu.Lock()
	if len(g.highlightedBeats) == 0 {
		g.highlightMu.Unlock()
		return
	}
	for key, val := range g.highlightedBeats {
		until := highlightUntil(val)
		if g.frame > until {
			delete(g.highlightedBeats, key)
			row, idx := splitBeatKey(key)
			g.logger.Debugf("[GAME] Cleared expired highlight for beat %d row %d. highlightedBeats: %v", idx, row, g.highlightedBeats)
		}
	}
	g.highlightMu.Unlock()
}

// clearRowHighlights removes all highlight entries for the given row.
func (g *Game) clearRowHighlights(row int) {
	g.highlightMu.Lock()
	if len(g.highlightedBeats) == 0 {
		g.highlightMu.Unlock()
		return
	}
	for key := range g.highlightedBeats {
		if r, _ := splitBeatKey(key); r == row {
			delete(g.highlightedBeats, key)
		}
	}
	g.highlightMu.Unlock()
}

func (g *Game) resetHighlights() {
	g.highlightMu.Lock()
	g.highlightedBeats = map[int]int64{}
	g.highlightMu.Unlock()
}

// highlightVisual mirrors highlightBeat but never queues audio. It only
// updates the highlight map and optional test hook.

func (g *Game) highlightVisual(row, idx int, info model.BeatInfo, duration int64) {
	key := makeBeatKey(row, idx)
	triggered, stateKnown := g.nodeTriggeredState(row, idx, info)
	if !triggered {
		if v, _, ok := g.timelineCommitted(row, idx); ok && v {
			triggered = true
		}
		if !triggered && row >= 0 && row < len(g.drum.Rows) {
			j := idx - g.drum.Offset
			if j >= 0 && j < len(g.drum.Rows[row].Steps) && g.drum.Rows[row].Steps[j] {
				triggered = true
			}
		}
	}
	hasState := stateKnown
	if row >= 0 && row < len(g.drum.Rows) {
		if v, ok := g.lastTriggered(row, info.NodeID); ok {
			hasState = true
			if v {
				triggered = true
			}
		}
	}
	shouldHighlight := triggered
	switch info.NodeType {
	case model.NodeTypeInvisible, model.NodeTypeSilent:
		shouldHighlight = true
	case model.NodeTypeMute:
		// mute highlights only when triggered
	default:
		// other node types rely on triggered state
	}
	if !shouldHighlight && info.NodeType == model.NodeTypeRegular && !hasState {
		shouldHighlight = true
	}
	if !shouldHighlight {
		return
	}
	isMute := info.NodeType == model.NodeTypeMute
	g.clearRowHighlights(row)
	if !g.rowIsAudible(row) {
		return
	}
	g.highlightSet(key, encodeHighlight(g.frame+duration, isMute))
	if g.highlightHook != nil && (info.NodeType == model.NodeTypeRegular || isMute) {
		if g.timingTestMode && info.NodeType == model.NodeTypeRegular {
			return
		}
		if row >= len(g.lastHLIdxByRow) {
			g.lastHLIdxByRow = make([]int, len(g.drum.Rows))
			for i := range g.lastHLIdxByRow {
				g.lastHLIdxByRow[i] = -1
			}
		}
		if g.lastHLIdxByRow[row] != idx {
			g.lastHLIdxByRow[row] = idx
			g.highlightHook(row, idx)
		}
	}
	if isMute {
		g.nodeAnimSet(info.NodeID, 1)
		g.setLastTriggered(row, info.NodeID, true)
		return
	}
}
