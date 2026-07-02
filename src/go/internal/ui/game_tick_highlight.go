package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func encodeHighlight(until int64, isMute bool) int64 {
	val := until &^ highlightMuteFlag
	if isMute {
		val |= highlightMuteFlag
	}
	return val
}

func highlightUntil(val int64) int64 {
	return val &^ highlightMuteFlag
}

func isMuteHighlight(val int64) bool {
	return val&highlightMuteFlag != 0
}

func (g *Game) onTick(step int) {
	g.currentStep = step

	if step == 0 {
		for row := range g.drum.Rows {
			if g.pulseForRow(row) == nil {
				if row == 0 && g.start == nil {
					continue
				}
				if row > 0 && g.drum.Rows[row].Origin == model.InvalidNodeID {
					continue
				}
				g.spawnPulseFromRow(row, g.nextBeatIdxs[row])
			}
		}
	}
}

func (g *Game) highlightBeat(row, idx int, info model.BeatInfo, duration int64) {
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
	if !triggered && info.NodeType == model.NodeTypeRegular && !hasState {
		triggered = true
	}
	if !triggered {
		if row >= 0 && row < len(g.drum.Rows) {
			g.setLastTriggered(row, info.NodeID, false)
		}
		if info.NodeType == model.NodeTypeMute {
			g.nodeAnimSet(info.NodeID, 0)
		}
		return
	}
	// Strict policy: only one highlight per row to avoid leftover markers.
	isMute := info.NodeType == model.NodeTypeMute
	g.clearRowHighlights(row)
	if row >= 0 && row < len(g.drum.Rows) {
		g.setLastTriggered(row, info.NodeID, true)
	}
	if !g.rowIsAudible(row) {
		if isMute {
			g.nodeAnimSet(info.NodeID, 0)
		}
		return
	}
	g.highlightSet(makeBeatKey(row, idx), encodeHighlight(g.frame+duration, isMute))
	if g.highlightHook != nil && (info.NodeType == model.NodeTypeRegular || isMute) {
		if !g.timingTestMode || info.NodeType != model.NodeTypeRegular {
			// Fire hook only once per row/index across frames.
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
	}
	if isMute {
		g.setLastTriggered(row, info.NodeID, true)
		g.nodeAnimSet(info.NodeID, 1)
		return
	}
	if info.NodeType != model.NodeTypeRegular {
		return
	}
	if row >= len(g.drum.Rows) {
		return
	}
	// Respect mute/solo
	// Suppress playback when row instrument is missing
	instAvailable := true
	if row >= 0 && row < len(g.drum.Rows) {
		instAvailable = g.drum.IsInstrumentAvailable(g.drum.Rows[row].Instrument)
	}
	if row >= 0 && row < len(g.drum.Rows) && !instAvailable && g.playFn == nil && g.scheduleHook == nil {
		g.logger.Debugf("[game/audio] missing instrument for row %d", row)
		return
	}
	anySolo := false
	for _, r := range g.drum.Rows {
		if r.Solo {
			anySolo = true
			break
		}
	}
	if g.drum.Rows[row].Muted || (anySolo && !g.drum.Rows[row].Solo) {
		g.logger.Debugf("[game/audio] muted row %d", row)
		return
	}
	// Decide audible via the engine predictor (single source of truth).
	audible := (info.NodeType == model.NodeTypeRegular)
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.Ensure(idx + 1)
		audible = g.engine.Predictor.AudibleAt(row, idx)
	}
	inst := g.drum.Rows[row].Instrument
	if audible {
		// Trigger node animation for audible events respecting node logic
		g.nodeAnimSet(info.NodeID, 1)
		if row >= len(g.lastFiredNodeByRow) {
			g.lastFiredNodeByRow = make([]model.NodeID, len(g.drum.Rows))
		}
		g.lastFiredNodeByRow[row] = info.NodeID
		g.setLastTriggered(row, info.NodeID, true)
		g.logger.Tracef("[game/highlight] row=%d idx=%d inst=%s", row, idx, inst)
		// Avoid double-triggering audio while the sequencer is running during playback.
		if !g.Playing() {
			vol, pitch, dur := g.evalNodeParamsOnly(row, idx, info)
			g.scheduleSound(row, idx, info, inst, vol, pitch, dur, math.NaN(), false)
		}
		g.logger.Tracef("[game/highlight] played inst=%s node=%d beat=%d row=%d", inst, info.NodeID, idx, row)
	} else {
		g.setLastTriggered(row, info.NodeID, false)
		// Explicitly clear any lingering animation on skipped triggers to
		// satisfy tests that assert no visual pulse on a gated event.
		g.nodeAnimSet(info.NodeID, 0)
		g.clearNodeHighlight(info.NodeID)
	}
}
