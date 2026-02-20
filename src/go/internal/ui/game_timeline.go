package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (g *Game) timelineWindowLen() int {
	if g == nil || g.drum == nil {
		return 1
	}
	if g.drum.Length > 0 {
		return g.drum.Length
	}
	return 1
}

func (g *Game) timelineService() *timeline.Service {
	if g.timeline == nil {
		g.timeline = timeline.NewService()
	}
	return g.timeline
}

func (g *Game) recordTimelineCommit(row, abs int, val bool, typ model.NodeType) {
	g.recordTimelineCommitKind(row, abs, val, typ, timeline.CommitKindPlayback)
}

func (g *Game) recordTimelineCommitKind(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) {
	windowLen := g.timelineWindowLen()
	capacity := windowLen
	g.timelineService().RecordCommitKind(row, abs, val, typ, kind, windowLen, capacity)
}

func (g *Game) timelineTrimAfter(row, maxAbs int) {
	g.timelineService().TrimAfter(row, maxAbs)
}

func (g *Game) timelineTrimAfterMutable(row, maxAbs int) {
	g.timelineService().TrimAfterMutable(row, maxAbs)
}

func (g *Game) timelineClearRow(row int) {
	g.timelineService().ClearRow(row)
}

func (g *Game) timelineCommitted(row, abs int) (bool, model.NodeType, bool) {
	return g.timelineService().Committed(row, abs)
}

func (g *Game) timelineCommittedWithKind(row, abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
	return g.timelineService().CommittedKind(row, abs)
}

func (g *Game) timelineReplaceCommit(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) bool {
	// Keep immutable past playback/import commits stable; allow demotion only
	// for future/leaked entries so live edits can refresh upcoming beats.
	if v, t, k, ok := g.timelineCommittedWithKind(row, abs); ok && (k == timeline.CommitKindPlayback || k == timeline.CommitKindImport) {
		globalNext := g.globalPastExclusive()
		// When the sequencer is enabled, its next index is the authoritative
		// monotonic boundary for the true past. Prevent replacements for any
		// playback/import commit strictly before that boundary, even if the UI's
		// freeze limit was temporarily clamped during a live edit.
		if row >= 0 && row < len(g.seqNextIdxs) {
			seqNext := g.seqNextIdxs[row]
			if globalNext > 0 && seqNext > globalNext {
				seqNext = globalNext
			}
			if seqNext > 0 && abs < seqNext {
				_ = v
				_ = t
				return false
			}
		}
		// A commit is "truly past" for a specific row when its absolute index is
		// strictly less than that row's next-beat index. Some rows can drift from
		// the global playhead during live edits or when the UI is catching up, so
		// do not rely on g.elapsedBeats here.
		next := globalNext
		if next <= 0 {
			next = g.elapsedBeats + 1
		}
		if row >= 0 && row < len(g.nextBeatIdxs) && g.nextBeatIdxs[row] > 0 {
			next = g.nextBeatIdxs[row]
		}
		if globalNext > 0 && (next == 0 || next > globalNext) {
			next = globalNext
		}
		if abs < next {
			_ = v
			_ = t
			return false
		}
	}
	return g.timelineService().ReplaceCommit(row, abs, val, typ, kind)
}

func (g *Game) TimelineSegments(row int) TimelineSegments {
	return g.timelineService().Snapshot(row)
}

func (g *Game) rowWindow(row int) RowWindow {
	if g == nil || g.drum == nil || row < 0 || row >= len(g.drum.Rows) {
		return RowWindow{}
	}
	offset := g.drum.Offset
	steps := g.drum.Rows[row].Steps
	types := g.drum.Rows[row].CellTypes
	win := RowWindow{Offset: offset}
	if len(steps) > 0 {
		win.Steps = make([]bool, len(steps))
		copy(win.Steps, steps)
	}
	if len(types) > 0 {
		win.Types = make([]model.NodeType, len(types))
		copy(win.Types, types)
	}
	return win
}

func (g *Game) timelineCommittedRange(row int) (int, int, bool) {
	return g.timelineService().CommittedRange(row)
}
