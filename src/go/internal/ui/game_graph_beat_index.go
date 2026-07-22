package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (s *playbackSlate) beatInfoAtRow(row, idx int) model.BeatInfo {
	g := s.g
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

func (s *playbackSlate) wrapBeatIndexRow(row, idx int) int {
	g := s.g
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
func (s *playbackSlate) rowIsAudible(row int) bool {
	g := s.g
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
func (s *playbackSlate) playheadFloor() int {
	g := s.g
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
func (s *playbackSlate) globalPastExclusive() int {
	g := s.g
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
func (s *playbackSlate) rowPastExclusive(row int) int {
	g := s.g
	if g == nil {
		return 0
	}
	global := g.globalPastExclusive()
	if global > 0 {
		// While playing/paused the immutable-past boundary is the VISUAL
		// playhead: every subdivision the playhead has passed (abs < global) is
		// past and must render from committed history — regardless of whether
		// this row's highlight (nextBeatIdxs) or scheduler (seqNextIdxs) index
		// has caught up. Those per-row indices lag the playhead whenever the
		// row's highlight is delivered late over hlCh or dropped by the
		// stale-info guard in applySequencerHighlight (which fires for every
		// in-flight highlight the instant a circuit edit changes the row's beat
		// path). Returning the lagged index here left the cells in
		// [next, playhead] treated as mutable future, so editing the circuit
		// re-rendered the already-played past from the edited predictor — the
		// "past changes on edit" regression. This is the single source of truth
		// for the boundary: buildRowWindow (render) and diffPredictorTimeline
		// (parity) both read it, so anchoring here keeps them consistent.
		return global
	}
	// Stopped: no playback past. Fall back to per-row highlight/schedule progress
	// so seek/preview flows still see the last committed frontier.
	next := 0
	if row >= 0 && row < len(g.nextBeatIdxs) {
		next = g.nextBeatIdxs[row]
	}
	if row >= 0 && row < len(g.seqNextIdxs) {
		if seqNext := g.seqNextIdxs[row]; seqNext > next {
			next = seqNext
		}
	}
	return next
}

func (g *Game) beatInfoAtRow(row, idx int) model.BeatInfo {
	return g.playbackSlate.beatInfoAtRow(row, idx)
}

func (g *Game) wrapBeatIndexRow(row, idx int) int {
	return g.playbackSlate.wrapBeatIndexRow(row, idx)
}

func (g *Game) rowIsAudible(row int) bool {
	return g.playbackSlate.rowIsAudible(row)
}

func (g *Game) playheadFloor() int {
	return g.playbackSlate.playheadFloor()
}

func (g *Game) globalPastExclusive() int {
	return g.playbackSlate.globalPastExclusive()
}

func (g *Game) rowPastExclusive(row int) int {
	return g.playbackSlate.rowPastExclusive(row)
}
