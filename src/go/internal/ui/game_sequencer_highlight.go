package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (g *Game) applySequencerHighlight(row, idx int, info model.BeatInfo) {
	rows := len(g.drum.Rows)
	if row < 0 || row >= rows {
		g.logger.Warnf("[game] highlight row out of range: row=%d rows=%d idx=%d", row, rows, idx)
		return
	}
	// Guard against late/out-of-order highlights (e.g., a full hlCh) rewinding
	// counters after syncUIToTime has already advanced the playhead. Highlights
	// are allowed for the current beat (idx == nextBeatIdxs[row]-1) but ignored
	// if they would move the playhead backwards.
	if row >= 0 && row < len(g.nextBeatIdxs) {
		cur := g.nextBeatIdxs[row]
		if cur > 0 && idx+1 < cur {
			return
		}
	}
	// Drop stale highlights that no longer match the current beat info.
	// This prevents queued events from earlier paths from freezing incorrect
	// playback history after delete/re-add edits.
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
	isOn := info.NodeType == model.NodeTypeRegular || info.NodeType == model.NodeTypeMute
	g.recordTimelineCommitKind(row, idx, isOn, info.NodeType, kind)
}
