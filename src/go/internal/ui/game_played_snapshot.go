package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
	"github.com/ingyamilmolinar/beatmo/internal/ui/preview"
)

// The played snapshot is the DrumView's immutable past: for every cell the
// playhead has passed, exactly what the grid showed for it at that moment
// (timeline.Service's played tier). preview.BuildRowWindow renders a past cell
// from it and from nothing else, so no later change of live state — a node
// edit, a solo/mute, a predictor rebuild, a dropped or late highlight — can
// reach a cell that has already played.
//
// freezePlayedThroughPlayhead must therefore run BEFORE anything that changes
// what a not-yet-frozen cell would show. It is called from:
//   - setPrimaryStep, the moment the playhead advances;
//   - the top of refreshDrumRow, before a pending SetPaths;
//   - the top of updateBeatInfos, before paths are recomputed;
//   - the structural-mutation hook (solo/mute, row delete, instrument, …).
// It is idempotent and only ever touches cells not yet frozen, so extra calls
// cost one PlayedMax read per row.

// predictCellAt is the live prediction a not-yet-played cell renders.
func (g *Game) predictCellAt(row, abs int, bi model.BeatInfo) bool {
	if g.engine == nil || g.engine.Predictor == nil {
		return false
	}
	if bi.NodeType == model.NodeTypeMute {
		return g.engine.Predictor.TriggeredAt(row, abs)
	}
	return g.engine.Predictor.VisibleAt(row, abs)
}

// freezePlayedThroughPlayhead freezes every row's cells up to and including
// the current beat (the builder's past boundary is elapsedBeats+1).
func (g *Game) freezePlayedThroughPlayhead() {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	// Mid-import the document is half-built (graph cleared, rows being
	// re-created): nothing it shows is what played. Undo/redo carries the
	// played history across the re-import instead (undoRestore).
	if g.importing || (g.undoManager != nil && g.undoManager.restoring) {
		return
	}
	limit := g.globalPastExclusive() - 1
	if limit < 0 {
		return
	}
	tl := g.timelineService()
	winStart, horizon := 0, limit+1
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.Ensure(limit + 1)
		winStart = g.engine.Predictor.WindowStart()
		horizon = g.engine.Predictor.Horizon()
	}
	for row := range g.drum.Rows {
		lo := tl.PlayedMax(row) + 1
		if lo < winStart {
			// Below the predictor window nothing can be computed any more;
			// those cells render from committed history.
			lo = winStart
		}
		rowIdx := row
		beatInfoAt := func(abs int) model.BeatInfo { return g.beatInfoAtRow(rowIdx, abs) }
		wantAt := func(abs int, bi model.BeatInfo) bool { return g.predictCellAt(rowIdx, abs, bi) }
		commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
			return g.timelineCommittedWithKind(rowIdx, abs)
		}
		for abs := lo; abs <= limit && abs < horizon; abs++ {
			val, typ := preview.CrossingCell(abs, beatInfoAt, wantAt, commitAt)
			tl.FreezePlayed(row, abs, val, typ)
		}
	}
}

// playedAtFunc reads row's played snapshot for the builder.
func (g *Game) playedAtFunc(row int) preview.PlayedAtFunc {
	tl := g.timelineService()
	return func(abs int) (bool, model.NodeType, bool) { return tl.Played(row, abs) }
}

// onDrumRowRemoved moves every index-keyed per-row store down with the rows
// when DrumView.DeleteRow splices row i out. Rows are addressed by index, so a
// store left unshifted hands row i+1 the deleted row's history, freeze
// watermark and scheduler cursor — the row below a deleted row then rendered
// (and re-seeded) its past from its neighbour's.
func (g *Game) onDrumRowRemoved(i int, origin model.NodeID) {
	if g == nil {
		return
	}
	if h := g.timelineService().RemoveRow(i); h != nil && origin != model.InvalidNodeID {
		if g.playedGraveyard == nil {
			g.playedGraveyard = make(map[model.NodeID]*timeline.PlayedHistory)
		}
		g.playedGraveyard[origin] = h
	}
	s := &g.playbackSlate
	s.beatInfosByRow = removeRowAt(s.beatInfosByRow, i)
	s.isLoopByRow = removeRowAt(s.isLoopByRow, i)
	s.loopStartByRow = removeRowAt(s.loopStartByRow, i)
	s.loopLenByRow = removeRowAt(s.loopLenByRow, i)
	s.originIdxsByRow = removeRowAt(s.originIdxsByRow, i)
	s.nextOriginIdxByRow = removeRowAt(s.nextOriginIdxByRow, i)
	s.nextBeatIdxs = removeRowAt(s.nextBeatIdxs, i)
	s.seqNextIdxs = removeRowAt(s.seqNextIdxs, i)
	s.lastHLIdxByRow = removeRowAt(s.lastHLIdxByRow, i)
	s.frozenUpToByRow = removeRowAt(s.frozenUpToByRow, i)
	s.reconciledUpToByRow = removeRowAt(s.reconciledUpToByRow, i)
	s.rowsPathChanged = removeRowAt(s.rowsPathChanged, i)
	s.pathSigByRow = removeRowAt(s.pathSigByRow, i)
	s.pathChangeBeatByRow = removeRowAt(s.pathChangeBeatByRow, i)
	s.rowRenderSig = removeRowAt(s.rowRenderSig, i)
	s.rowStepsScratch = removeRowAt(s.rowStepsScratch, i)
	s.rowTypesScratch = removeRowAt(s.rowTypesScratch, i)
	s.lastFiredNodeByRow = removeRowAt(s.lastFiredNodeByRow, i)
	s.muteUntilByRow = removeRowAt(s.muteUntilByRow, i)
	s.loopCountByRow = removeRowAt(s.loopCountByRow, i)
	s.nodeTriggerCountsByRow = removeRowKey(s.nodeTriggerCountsByRow, i)
	s.nodeLogicTriggerCountsByRow = removeRowKey(s.nodeLogicTriggerCountsByRow, i)
	s.lastTriggeredByRow = removeRowKey(s.lastTriggeredByRow, i)
}

func removeRowAt[T any](s []T, i int) []T {
	if i < 0 || i >= len(s) {
		return s
	}
	return append(s[:i], s[i+1:]...)
}

func removeRowKey[V any](m map[int]V, i int) map[int]V {
	if len(m) == 0 {
		return m
	}
	out := make(map[int]V, len(m))
	for r, v := range m {
		switch {
		case r < i:
			out[r] = v
		case r > i:
			out[r-1] = v
		}
	}
	return out
}

// detachPlayedByOrigin moves every row's played history (and the graveyard of
// deleted rows) out of the timeline, keyed by the row's origin node. Paired
// with reattachPlayedByOrigin around a document restore: undo/redo re-imports
// the document, which rebuilds rows and drops the timeline, but what already
// played is history, not document, and must come back untouched.
func (g *Game) detachPlayedByOrigin() map[model.NodeID]*timeline.PlayedHistory {
	out := make(map[model.NodeID]*timeline.PlayedHistory, len(g.drum.Rows)+len(g.playedGraveyard))
	for origin, h := range g.playedGraveyard {
		out[origin] = h
	}
	tl := g.timelineService()
	for i, r := range g.drum.Rows {
		if r == nil || r.Origin == model.InvalidNodeID {
			continue
		}
		if h := tl.TakePlayed(i); h != nil {
			out[r.Origin] = h
		}
	}
	return out
}

// reattachPlayedByOrigin reinstalls detached histories on the rows that carry
// the same origin node; histories whose row the restore removed go back to the
// graveyard for a later redo.
func (g *Game) reattachPlayedByOrigin(held map[model.NodeID]*timeline.PlayedHistory) {
	tl := g.timelineService()
	for i, r := range g.drum.Rows {
		if r == nil {
			continue
		}
		if h, ok := held[r.Origin]; ok {
			tl.PutPlayed(i, h)
			delete(held, r.Origin)
		}
	}
	if len(held) > 0 {
		g.playedGraveyard = held
	} else {
		g.playedGraveyard = nil
	}
}
