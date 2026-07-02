package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

// hlDroppedExemptFrames is how long (in frames, ~ a few seconds at 60 Hz) a
// dropped-highlight exemption is honored before pruning. It need only outlast
// the audio event's residence in the parity scan window (lookahead + grace),
// which is well under a second; this is generous slack.
const hlDroppedExemptFrames = 600

// markHighlightDropped records that the sequencer dropped (row, idx)'s highlight
// because hlCh was full. highlight_vs_audio parity then exempts that beat: the
// UI legitimately never paints it, so "audio fired but no highlight" is an
// expected cosmetic loss under load, not a desync. Called from the sequencer
// goroutine (holds seqMu); takes highlightMu (seqMu -> highlightMu order, same
// as applySequencerHighlight).
func (g *Game) markHighlightDropped(row, idx int) {
	if g == nil {
		return
	}
	key := makeBeatKey(row, idx)
	g.highlightMu.Lock()
	if g.hlDropped == nil {
		g.hlDropped = make(map[int]int64)
	}
	g.hlDropped[key] = g.frame + hlDroppedExemptFrames
	g.highlightMu.Unlock()
}

// highlightWasDropped reports whether (row, abs)'s highlight was dropped and the
// exemption is still live. Caller must NOT hold highlightMu.
func (g *Game) highlightWasDropped(row, abs int) bool {
	if g == nil {
		return false
	}
	key := makeBeatKey(row, abs)
	g.highlightMu.RLock()
	until, ok := g.hlDropped[key]
	g.highlightMu.RUnlock()
	return ok && until > g.frame
}

// highlightEntry holds a single per-row highlight (column index + encoded value).
type highlightEntry struct {
	idx int
	val int64 // expiration frame with mute flag in high bit
}

// highlightSnapshotByRow returns highlights bucketed by row for O(row_entries)
// iteration in Draw instead of O(total_highlights) per visible row.
func (g *Game) highlightSnapshotByRow(maxRows int) [][]highlightEntry {
	g.highlightMu.RLock()
	if len(g.highlightedBeats) == 0 {
		g.highlightMu.RUnlock()
		return nil
	}
	out := make([][]highlightEntry, maxRows)
	for key, val := range g.highlightedBeats {
		row, idx := splitBeatKey(key)
		if row >= 0 && row < maxRows {
			out[row] = append(out[row], highlightEntry{idx: idx, val: val})
		}
	}
	g.highlightMu.RUnlock()
	return out
}

//nolint:unused // called from js_exports_timeline_predictor.go (WASM build tag)
func (g *Game) hasAnyRowHighlight(row int) bool {
	g.highlightMu.RLock()
	defer g.highlightMu.RUnlock()
	for key := range g.highlightedBeats {
		if r, _ := splitBeatKey(key); r == row {
			return true
		}
	}
	return false
}

//nolint:unused // called from js_exports_timeline_predictor.go (WASM build tag)
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

//nolint:unused // called from js_exports_harness.go (WASM build tag)
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

// highlightAbsRetentionSlack is the lookback below drum.Offset that
// clearExpiredHighlights still keeps. Anything strictly below
// (drum.Offset - highlightAbsRetentionSlack) cannot be displayed and is
// unconditionally evictable; the highlight animation is a transient visual
// effect, so historical scroll-back does not replay it (timeline cold archive
// owns playback history). 64 ≈ 8 visible windows of 8 — generous slack.
const highlightAbsRetentionSlack = 64

func (g *Game) clearExpiredHighlights() {
	g.highlightMu.Lock()
	defer g.highlightMu.Unlock()
	// Prune expired dropped-highlight exemptions (independent of highlightedBeats;
	// runs even when no highlights are live so the map can't grow unbounded under
	// sustained drop load).
	for key, until := range g.hlDropped {
		if g.frame > until {
			delete(g.hlDropped, key)
		}
	}
	if len(g.highlightedBeats) == 0 {
		return
	}
	var minAbs int
	var haveBound bool
	if g.drum != nil && g.drum.Length > 0 {
		minAbs = g.drum.Offset - highlightAbsRetentionSlack
		haveBound = true
	}
	for key, val := range g.highlightedBeats {
		until := highlightUntil(val)
		row, idx := splitBeatKey(key)
		if g.frame > until {
			delete(g.highlightedBeats, key)
			g.logger.Debugf("[game] Cleared expired highlight for beat %d row %d. highlightedBeats: %v", idx, row, g.highlightedBeats)
			continue
		}
		if haveBound && idx < minAbs {
			delete(g.highlightedBeats, key)
		}
	}
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
	g.hlDropped = map[int]int64{}
	g.highlightMu.Unlock()
}

// drainAndDecayHighlights drains pending highlight events from the sequencer,
// clears expired highlights, and decays per-node trigger animations. This is
// extracted from Update() so it can also be called from the syncHighlights JS
// export, allowing tests to process highlight state without a full Update().
func (g *Game) drainAndDecayHighlights() {
	// Drain any pending highlight events dispatched by the sequencer loop and
	// apply them on the UI thread to avoid data races with highlight state.
	for {
		select {
		case ev := <-g.hlCh:
			g.applySequencerHighlight(ev.row, ev.idx, ev.info)
		default:
			goto hlDone
		}
	}
hlDone:
	// Highlight cleanup always runs - essential for WASM where fastPath is enabled.
	// Without this, node highlights stay on forever in the browser.
	g.clearExpiredHighlights()

	// Decay per-node trigger animations
	g.nodeAnimMu.Lock()
	for id, v := range g.nodeAnim {
		if start, end, ok := g.nodeHighlightUntil(id); ok {
			now := audio.Now()
			if now >= end {
				g.clearNodeHighlight(id)
				delete(g.nodeAnim, id)
				continue
			}
			if now >= start {
				// Inside active highlight window
				g.nodeAnim[id] = 1
			} else {
				// Before start - don't show highlight yet
				g.nodeAnim[id] = 0
			}
			continue
		}
		nv, alive := DecayStep(v, genAnimHighlightDecay)
		if !alive {
			delete(g.nodeAnim, id)
		} else {
			g.nodeAnim[id] = nv
		}
	}
	g.nodeAnimMu.Unlock()
}

// highlightVisual mirrors highlightBeat but never queues audio. It only
// updates the highlight map and optional test hook.
//
// All in-tree callers (sequencer/audio schedule/sync-UI/pause-replay)
// know the beat is firing at call time, so the call itself is the
// authoritative trigger signal. The internal triggered/predictor
// cascade exists only as defense for indirect callers that aren't
// scheduler-driven — see highlightVisualUntrusted. Long-session
// failure mode pre-fix: once the predictor's 4096-cap sliding window
// passed `idx`, nodeTriggeredState returned (false, stateKnown=true)
// via predictor fallback, which bypassed the regular-node rescue
// branch and silently dropped the highlight write.
func (g *Game) highlightVisual(row, idx int, info model.BeatInfo, duration int64) {
	g.highlightVisualImpl(row, idx, info, duration, true /*trusted: caller is firing the beat*/)
}

// highlightVisualUntrusted is for callers that don't have first-hand
// knowledge that the beat is firing (e.g. exploratory probes from
// tests). Reserved for future use; no in-tree callers today. The
// fallback cascade lives here, isolated from the hot scheduler path.
//
//nolint:unused // reserved for future indirect callers
func (g *Game) highlightVisualUntrusted(row, idx int, info model.BeatInfo, duration int64) {
	g.highlightVisualImpl(row, idx, info, duration, false)
}

func (g *Game) highlightVisualImpl(row, idx int, info model.BeatInfo, duration int64, trusted bool) {
	key := makeBeatKey(row, idx)
	shouldHighlight := false
	if trusted {
		// Scheduler/audio/sync path: trust the caller. Regular and mute
		// beats have already been decided to fire; invisible/silent
		// always show their highlight; the only state we still consult
		// is mute-isMute (encoded in the highlight value) below.
		shouldHighlight = true
	} else {
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
		if !triggered && info.NodeType == model.NodeTypeRegular {
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.Ensure(idx + 1)
				if g.engine.Predictor.VisibleAt(row, idx) {
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
		shouldHighlight = triggered
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
	if info.NodeType == model.NodeTypeRegular && row >= 0 && row < len(g.drum.Rows) {
		g.setLastTriggered(row, info.NodeID, true)
	}
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
