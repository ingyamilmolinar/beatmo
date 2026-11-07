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

// Only needs to outlast the audio event's stay in the parity scan window (under a second).
const hlDroppedExemptFrames = 600

// A highlight dropped because hlCh was full is an expected cosmetic loss under load, so parity
// exempts it. Sequencer goroutine; lock order seqMu → highlightMu.
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

// Caller must NOT hold highlightMu.
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

// Bucketed so Draw iterates a row's own entries, not every highlight per row.
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

// Anything further below Offset cannot be displayed: highlights are transient, and scroll-back
// history belongs to the timeline archive.
const highlightAbsRetentionSlack = 64

func (g *Game) clearExpiredHighlights() {
	g.highlightMu.Lock()
	defer g.highlightMu.Unlock()
	// Pruned even with no live highlights, so sustained drops cannot grow the map without bound.
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

// Separate from Update so the syncHighlights JS export can run it alone.
func (g *Game) drainAndDecayHighlights() {
	// Applied on the UI thread, so highlight state has no races.
	for {
		select {
		case ev := <-g.hlCh:
			g.applySequencerHighlight(ev.row, ev.idx, ev.info)
			// Only for a HEARD row: a glow for a muted or soloed-out row would claim a sound that never played.
			if g.drum != nil && ev.row >= 0 && ev.row < len(g.drum.Rows) && g.drum.Rows[ev.row] != nil &&
				rowAudible(g.drum.Rows[ev.row], anySoloActive(g.drum.Rows)) {
				g.drum.onInstrumentTriggered(g.drum.Rows[ev.row].Instrument)
			}
		default:
			goto hlDone
		}
	}
hlDone:
	// Always: on WASM's fast path nothing else clears node highlights.
	g.clearExpiredHighlights()

	// One audio.Now() per pass: on WASM each is a JS bridge call.
	g.nodeAnimMu.Lock()
	now, haveNow := 0.0, false
	for id, v := range g.nodeAnim {
		if start, end, ok := g.nodeHighlightUntil(id); ok {
			if !haveNow {
				now, haveNow = audio.Now(), true
			}
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

// Never queues audio. Every caller knows the beat is firing, so the call itself is the trigger;
// consulting the predictor dropped highlights once its sliding window passed idx.
func (g *Game) highlightVisual(row, idx int, info model.BeatInfo, duration int64) {
	g.highlightVisualImpl(row, idx, info, duration, true /*trusted: caller is firing the beat*/)
}

// For callers without first-hand knowledge that the beat fires; keeps the fallback cascade off
// the scheduler path.
//
//nolint:unused // reserved for future indirect callers
func (g *Game) highlightVisualUntrusted(row, idx int, info model.BeatInfo, duration int64) {
	g.highlightVisualImpl(row, idx, info, duration, false)
}

func (g *Game) highlightVisualImpl(row, idx int, info model.BeatInfo, duration int64, trusted bool) {
	key := makeBeatKey(row, idx)
	shouldHighlight := false
	if trusted {
		// Trust the scheduler: the beat has already been decided to fire.
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
		if !triggered && info.NodeType.Fires() {
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
		if !shouldHighlight && info.NodeType.Fires() && !hasState {
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
	if info.NodeType.Fires() && row >= 0 && row < len(g.drum.Rows) {
		g.setLastTriggered(row, info.NodeID, true)
	}
	if g.highlightHook != nil && (info.NodeType.Fires() || isMute) {
		if g.timingTestMode && info.NodeType.Fires() {
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
