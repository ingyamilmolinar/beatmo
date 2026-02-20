package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (g *Game) beatInfoAtRow(row, idx int) model.BeatInfo {
	if row < 0 || row >= len(g.beatInfosByRow) {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	infos := g.beatInfosByRow[row]
	if len(infos) == 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx < 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx < len(infos) {
		return infos[idx]
	}
	if !g.isLoopByRow[row] {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	loopLen := len(infos) - g.loopStartByRow[row]
	if loopLen <= 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	idx = g.loopStartByRow[row] + (idx-g.loopStartByRow[row])%loopLen
	return infos[idx]
}

func (g *Game) wrapBeatIndexRow(row, idx int) int {
	if row < 0 || row >= len(g.beatInfosByRow) {
		return 0
	}
	infos := g.beatInfosByRow[row]
	if len(infos) == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx < len(infos) {
		return idx
	}
	if !g.isLoopByRow[row] {
		return len(infos) - 1
	}
	loopLen := len(infos) - g.loopStartByRow[row]
	if loopLen <= 0 {
		return len(infos) - 1
	}
	return g.loopStartByRow[row] + (idx-g.loopStartByRow[row])%loopLen
}

// rowIsAudible reports whether the given drum row is currently audible under
// mute/solo gating. When any row is soloed, only solo=true rows are audible.
func (g *Game) rowIsAudible(row int) bool {
	if row < 0 || row >= len(g.drum.Rows) {
		return false
	}
	anySolo := false
	for _, r := range g.drum.Rows {
		if r.Solo {
			anySolo = true
			break
		}
	}
	if g.drum.Rows[row].Muted {
		return false
	}
	if anySolo && !g.drum.Rows[row].Solo {
		return false
	}
	if !g.drum.IsInstrumentAvailable(g.drum.Rows[row].Instrument) {
		return false
	}
	return true
}

// playheadFloor returns the smallest nextBeatIdx across rows (or renderOffset
// when unknown) so parity checks can ignore cells that are already in the past.
func (g *Game) playheadFloor() int {
	if g == nil {
		return 0
	}
	if len(g.nextBeatIdxs) == 0 {
		if g.renderOffset > 0 {
			return g.renderOffset
		}
		return 0
	}
	min := g.nextBeatIdxs[0]
	for _, v := range g.nextBeatIdxs {
		if v < min {
			min = v
		}
	}
	if min < 0 {
		min = 0
	}
	return min
}

// globalPastExclusive returns the global "true past" boundary (exclusive) in
// subdivision units. It is derived from the wall-clock playhead (elapsedBeats)
// so row-specific counters cannot accidentally freeze future beats.
func (g *Game) globalPastExclusive() int {
	if g == nil {
		return 0
	}
	playingOrPaused := g.Playing()
	if g.state != nil && g.state.Paused() {
		playingOrPaused = true
	}
	if !playingOrPaused {
		return 0
	}
	next := g.elapsedBeats + 1
	if next < 0 {
		next = 0
	}
	return next
}

// rowPastExclusive returns the row's "true past" boundary (exclusive) in
// subdivision units, clamped to the global playhead to prevent stale timeline
// commits from masking the mutable future.
func (g *Game) rowPastExclusive(row int) int {
	if g == nil {
		return 0
	}
	next := 0
	if row >= 0 && row < len(g.nextBeatIdxs) {
		next = g.nextBeatIdxs[row]
	}
	if row >= 0 && row < len(g.seqNextIdxs) {
		if seqNext := g.seqNextIdxs[row]; seqNext > next {
			next = seqNext
		}
	}
	global := g.globalPastExclusive()
	if global > 0 && (next == 0 || next > global) {
		next = global
	}
	return next
}
