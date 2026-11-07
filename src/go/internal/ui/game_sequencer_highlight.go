package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (s *playbackSlate) applySequencerHighlight(row, idx int, info model.BeatInfo) {
	g := s.g
	rows := len(g.drum.Rows)
	if row < 0 || row >= rows {
		g.logger.Warnf("[game] highlight row out of range: row=%d rows=%d idx=%d", row, rows, idx)
		return
	}
	// A late highlight (full hlCh) may land for the current beat but must never move the playhead back.
	if row >= 0 && row < len(g.nextBeatIdxs) {
		cur := g.nextBeatIdxs[row]
		if cur > 0 && idx+1 < cur {
			return
		}
	}
	// Queued events from older paths must not freeze wrong history after an edit.
	curInfo := g.beatInfoAtRow(row, idx)
	if curInfo.NodeID != info.NodeID || curInfo.NodeType != info.NodeType {
		return
	}
	beatDuration := int64(60.0 / float64(max1(g.state.AppliedBPM())) * ebitenTPS)
	g.highlightVisual(row, idx, info, beatDuration)
	if g.rowIsAudible(row) {
		g.nodeAnimSet(info.NodeID, 1)
		// Sync grid node highlight duration with drum view.
		beatSec := 60.0 / float64(max1(g.state.AppliedBPM()))
		now := audio.Now()
		if now > 0 {
			g.setNodeHighlightUntil(info.NodeID, now, now+beatSec)
		}
		if g.drum != nil {
			g.drum.MarkRowFired(row)
		}
	} else {
		g.nodeAnimSet(info.NodeID, 0)
	}
	if len(g.nextBeatIdxs) != rows {
		next := make([]int, rows)
		copy(next, g.nextBeatIdxs)
		g.nextBeatIdxs = next
	}
	g.nextBeatIdxs[row] = idx + 1
	if row == 0 && idx >= 0 {
		g.setPrimaryStep(idx)
	}
	// Commit to timeline history so past segments remain immutable.
	kind := timeline.CommitKindPlayback
	if !g.Playing() {
		kind = timeline.CommitKindSeeded
	}
	isOn := info.NodeType.Fires() || info.NodeType == model.NodeTypeMute
	g.recordTimelineCommitKind(row, idx, isOn, info.NodeType, kind)
}

func (g *Game) applySequencerHighlight(row, idx int, info model.BeatInfo) {
	g.playbackSlate.applySequencerHighlight(row, idx, info)
}
