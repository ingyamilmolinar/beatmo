package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (s *playbackSlate) timelineWindowLen() int {
	g := s.g
	if g == nil || g.drum == nil {
		return 1
	}
	if g.drum.Length > 0 {
		return g.drum.Length
	}
	return 1
}

func (s *playbackSlate) timelineService() *timeline.Service {
	g := s.g
	if g.timeline == nil {
		g.timeline = timeline.NewService()
	}
	return g.timeline
}

func (s *playbackSlate) recordTimelineCommit(row, abs int, val bool, typ model.NodeType) {
	g := s.g
	g.recordTimelineCommitKind(row, abs, val, typ, timeline.CommitKindPlayback)
}

func (s *playbackSlate) recordTimelineCommitKind(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) {
	g := s.g
	windowLen := g.timelineWindowLen()
	capacity := windowLen
	g.timelineService().RecordCommitKind(row, abs, val, typ, kind, windowLen, capacity)
}

func (s *playbackSlate) timelineTrimAfter(row, maxAbs int) {
	g := s.g
	g.timelineService().TrimAfter(row, maxAbs)
}

func (s *playbackSlate) timelineTrimAfterMutable(row, maxAbs int) {
	g := s.g
	g.timelineService().TrimAfterMutable(row, maxAbs)
}

// timelineRewindTo archives post-abs immutable commits for row and cuts the
// mutable/ring tier at abs-1 (Service.RewindTo), for a backward seek.
func (s *playbackSlate) timelineRewindTo(row, abs int) {
	if s.g == nil {
		return
	}
	s.g.timelineService().RewindTo(row, abs)
}

func (s *playbackSlate) timelineClearRow(row int) {
	g := s.g
	g.timelineService().ClearRow(row)
}

func (s *playbackSlate) timelineCommitted(row, abs int) (bool, model.NodeType, bool) {
	g := s.g
	return g.timelineService().Committed(row, abs)
}

func (s *playbackSlate) timelineCommittedWithKind(row, abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
	g := s.g
	return g.timelineService().CommittedKind(row, abs)
}

func (s *playbackSlate) timelineReplaceCommit(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) bool {
	g := s.g
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

func (s *playbackSlate) TimelineSegments(row int) TimelineSegments {
	g := s.g
	return g.timelineService().Snapshot(row)
}

func (s *playbackSlate) rowWindow(row int) RowWindow {
	g := s.g
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

func (s *playbackSlate) timelineCommittedRange(row int) (int, int, bool) {
	g := s.g
	return g.timelineService().CommittedRange(row)
}

func (g *Game) timelineWindowLen() int { return g.playbackSlate.timelineWindowLen() }

func (g *Game) timelineService() *timeline.Service { return g.playbackSlate.timelineService() }

func (g *Game) recordTimelineCommit(row, abs int, val bool, typ model.NodeType) {
	g.playbackSlate.recordTimelineCommit(row, abs, val, typ)
}

func (g *Game) recordTimelineCommitKind(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) {
	g.playbackSlate.recordTimelineCommitKind(row, abs, val, typ, kind)
}

func (g *Game) timelineTrimAfter(row, maxAbs int) { g.playbackSlate.timelineTrimAfter(row, maxAbs) }

func (g *Game) timelineTrimAfterMutable(row, maxAbs int) {
	g.playbackSlate.timelineTrimAfterMutable(row, maxAbs)
}

// timelineRewindTo is the Game-side chokepoint for rewinding a row's timeline
// on a backward seek: it archives post-abs immutable commits and cuts the
// mutable tier via Service.RewindTo, then lowers the freeze watermark so the
// rewound region renders from the predictor (live re-simulation), not the
// archived past.
func (g *Game) timelineRewindTo(row, abs int) {
	g.playbackSlate.timelineRewindTo(row, abs)
	g.clampRowFreeze(row, abs-1)
}

func (g *Game) timelineClearRow(row int) { g.playbackSlate.timelineClearRow(row) }

func (g *Game) timelineCommitted(row, abs int) (bool, model.NodeType, bool) {
	return g.playbackSlate.timelineCommitted(row, abs)
}

func (g *Game) timelineCommittedWithKind(row, abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
	return g.playbackSlate.timelineCommittedWithKind(row, abs)
}

func (g *Game) timelineReplaceCommit(row, abs int, val bool, typ model.NodeType, kind timeline.CommitKind) bool {
	return g.playbackSlate.timelineReplaceCommit(row, abs, val, typ, kind)
}

func (g *Game) TimelineSegments(row int) TimelineSegments {
	return g.playbackSlate.TimelineSegments(row)
}

func (g *Game) rowWindow(row int) RowWindow { return g.playbackSlate.rowWindow(row) }

func (g *Game) timelineCommittedRange(row int) (int, int, bool) {
	return g.playbackSlate.timelineCommittedRange(row)
}
