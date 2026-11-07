package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (s *playbackSlate) updateBeatInfos() {
	g := s.g
	start := time.Now()
	defer func() { g.logger.Debugf("[update_beat_infos] total=%v", time.Since(start)) }()

	// Before the paths are rebuilt: those cells have played.
	g.freezePlayedThroughPlayhead()

	g.rebuildNodeCache()
	g.logger.Debugf("[update_beat_infos] rebuildNodeCache elapsed=%v", time.Since(start))

	// An unbounded path, so disconnected nodes or synthesized invisible steps never truncate it.
	calcStart := time.Now()
	fullBeatRow, isLoop, loopStart := g.graph.CalculateBeatRowUnbounded()
	g.logger.Debugf("[update_beat_infos] CalculateBeatRowUnbounded elapsed=%v isLoop=%v loopStart=%d pathLen=%d",
		time.Since(calcStart), isLoop, loopStart, len(fullBeatRow))
	baseLen := rawBeatLen(fullBeatRow, isLoop, loopStart)
	g.logger.Debugf("[update_beat_infos] rawBeatLen=%d", baseLen)

	g.beatInfos = fullBeatRow[:baseLen]
	g.isLoop = isLoop
	g.loopStartIndex = loopStart

	maxLen := baseLen
	g.nodeRows = map[model.NodeID]int{}
	nRows := len(g.drum.Rows)
	g.logger.Debugf("[update_beat_infos] nRows=%d drum.Rows=%d", nRows, len(g.drum.Rows))
	prevNextByRow := g.nextBeatIdxs
	g.beatInfosByRow = make([][]model.BeatInfo, nRows)
	g.isLoopByRow = make([]bool, nRows)
	g.loopStartByRow = make([]int, nRows)
	g.loopLenByRow = make([]int, nRows)
	g.originIdxsByRow = make([][]int, nRows)
	g.nextOriginIdxByRow = make([]int, nRows)
	g.nextBeatIdxs = make([]int, nRows)
	copy(g.nextBeatIdxs, prevNextByRow)
	// Reset last-highlighted indices to force a hook on first arrival per row.
	g.lastHLIdxByRow = make([]int, nRows)
	for i := range g.lastHLIdxByRow {
		g.lastHLIdxByRow[i] = -1
	}
	g.logger.Debugf("[update_beat_infos] before row0 setup nRows=%d", nRows)
	if nRows > 0 {
		g.beatInfosByRow[0] = g.beatInfos
		g.isLoopByRow[0] = isLoop
		g.loopStartByRow[0] = loopStart
		if isLoop {
			g.loopLenByRow[0] = loopSegmentLen(g.beatInfos, loopStart)
		}
		origin := g.drum.Rows[0].Origin
		g.logger.Debugf("[update_beat_infos] row0 origin=%d beatInfosLen=%d", origin, len(g.beatInfos))
		for idx, b := range g.beatInfos {
			if b.NodeID != model.InvalidNodeID {
				g.nodeRows[b.NodeID] = 0
			}
			if b.NodeID == origin {
				g.originIdxsByRow[0] = append(g.originIdxsByRow[0], idx)
			}
		}
		g.logger.Debugf("[update_beat_infos] row0 setup done")
	}

	g.logger.Debugf("[update_beat_infos] before row loop, nRows=%d", nRows)
	// Compute beat paths for additional drum rows using their origin nodes.
	for i, r := range g.drum.Rows {
		g.logger.Debugf("[update_beat_infos] processing row %d origin=%d", i, r.Origin)
		if i == 0 {
			// row 0 handled above; ensure its origin tracks the start node
			if r.Origin == model.InvalidNodeID && g.start != nil {
				g.drum.Rows[0].Origin = g.start.ID
				g.drum.Rows[0].Node = g.start
			}
			continue
		}
		if r.Origin == model.InvalidNodeID {
			g.logger.Debugf("[update_beat_infos] row %d has InvalidNodeID origin, skipping", i)
			continue
		}
		g.logger.Debugf("[update_beat_infos] row %d calling CalculateBeatRowFrom origin=%d", i, r.Origin)
		rowPath, rowLoop, rowStart := g.graph.CalculateBeatRowFrom(r.Origin)
		g.logger.Debugf("[update_beat_infos] row %d CalculateBeatRowFrom done pathLen=%d", i, len(rowPath))
		rowLen := rawBeatLen(rowPath, rowLoop, rowStart)
		g.beatInfosByRow[i] = rowPath[:rowLen]
		g.isLoopByRow[i] = rowLoop
		g.loopStartByRow[i] = rowStart
		if rowLoop {
			g.loopLenByRow[i] = loopSegmentLen(g.beatInfosByRow[i], rowStart)
		}
		origin := r.Origin
		for idx, b := range rowPath[:rowLen] {
			if b.NodeID != model.InvalidNodeID {
				if _, exists := g.nodeRows[b.NodeID]; !exists {
					g.nodeRows[b.NodeID] = i
				}
			}
			if b.NodeID == origin {
				g.originIdxsByRow[i] = append(g.originIdxsByRow[i], idx)
			}
		}
		if rowLen > maxLen {
			maxLen = rowLen
		}
	}

	// Shrink to the real traversal so later paths are not padded with loop cycles.
	g.logger.Debugf("[update_beat_infos] after row loop, maxLen=%d", maxLen)
	g.graph.SetBeatLength(maxLen)
	g.logger.Debugf("[update_beat_infos] SetBeatLength done")

	g.resetOriginSequences()
	g.logger.Debugf("[update_beat_infos] resetOriginSequences done")
	g.muteUntilByRow = make([]int, len(g.drum.Rows))
	g.renderReady = false

	// Node logic starts fresh whenever the paths are recomputed.
	g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.lastEvalIdxByRowNode = make(map[int]map[model.NodeID]int)
	g.logger.Debugf("[update_beat_infos] trigger counters reset")

	if maxLen < 1 {
		maxLen = 1
	}
	if !g.Playing() && maxLen > g.drum.Length {
		// Always store the full circuit length so the timeline is scrollable.
		g.drum.SetBeatLength(maxLen)
		// Mobile caps the visible window at its default unless the user changed it.
		visLen := maxLen
		if !g.drum.userAdjustedLength {
			if db := Profile().DefaultTimelineBeats; db > 0 {
				cap := db * g.drum.timelineUnitsPerBeat
				if visLen > cap {
					visLen = cap
				}
			}
		}
		g.logger.Debugf("[update_beat_infos] calling drum.SetLengthClamped visLen=%d (maxLen=%d)", visLen, maxLen)
		g.drum.SetLengthClamped(visLen)
		// Restore graph beat length that SetLengthClamped may have shrunk.
		g.graph.SetBeatLength(maxLen)
		g.logger.Debugf("[update_beat_infos] drum.SetLengthClamped done")
	} else {
		// While playing only the beat length changes, so the window does not jump.
		g.drum.SetBeatLength(maxLen)
	}

	// Keep each pulse's absolute progression.
	g.logger.Debugf("[update_beat_infos] rebinding %d active pulses", len(g.activePulses))
	for _, p := range g.activePulses {
		if p.row >= len(g.beatInfosByRow) {
			continue
		}
		path := g.beatInfosByRow[p.row]
		if len(path) == 0 {
			continue
		}
		p.path = path
		absLast := g.nextBeatIdxs[p.row] - 1
		wrappedLast := g.wrapBeatIndexRow(p.row, absLast)
		wrappedNext := g.wrapBeatIndexRow(p.row, g.nextBeatIdxs[p.row])
		p.lastIdx = absLast
		p.fromBeatInfo = path[wrappedLast]
		p.toBeatInfo = path[wrappedNext]
		p.pathIdx = wrappedNext
		p.from = g.nodeByID(p.fromBeatInfo.NodeID)
		p.to = g.nodeByID(p.toBeatInfo.NodeID)
		if p.from != nil {
			p.x1, p.y1 = p.from.X, p.from.Y
		}
		if p.to != nil {
			p.x2, p.y2 = p.to.X, p.to.Y
		}
	}

	g.logger.Debugf("[update_beat_infos] active pulses done")
	g.logger.Debugf("[game] updateBeatInfos: drum.Length=%d, beatPath=%d", g.drum.Length, len(g.beatInfos))
	// Log per-row path sizes to aid debugging imports/demo configs.
	if len(g.beatInfosByRow) > 0 {
		sizes := make([]int, len(g.beatInfosByRow))
		for i := range g.beatInfosByRow {
			sizes[i] = len(g.beatInfosByRow[i])
		}
		g.logger.Debugf("[game] per-row path lens: %v", sizes)
	}
	g.logger.Debugf("[update_beat_infos] after per-row path lens")

	// Clamped against the timeline length, so tracking can continue past the first traversal.
	units := max1(g.drum.timelineUnitsPerBeat)
	maxOffset := g.drum.timelineBeats*units - g.drum.Length
	if maxOffset < 0 {
		maxOffset = 0
	}
	if g.drum.Offset > maxOffset {
		g.drum.Offset = maxOffset
	}
	g.logger.Debugf("[update_beat_infos] offset clamped")

	// Compute path signatures and detect rows whose shape changed.
	nPaths := len(g.beatInfosByRow)
	g.logger.Debugf("[update_beat_infos] computing path signatures nPaths=%d", nPaths)
	if len(g.pathSigByRow) != nPaths {
		old := g.pathSigByRow
		g.pathSigByRow = make([]uint64, nPaths)
		copy(g.pathSigByRow, old)
	}
	if len(g.rowsPathChanged) != nPaths {
		g.rowsPathChanged = make([]bool, nPaths)
	}
	if len(g.pathChangeBeatByRow) != nPaths {
		g.pathChangeBeatByRow = make([]int, nPaths)
		for i := range g.pathChangeBeatByRow {
			g.pathChangeBeatByRow[i] = -1
		}
	}
	pathsChanged := false
	g.logger.Debugf("[update_beat_infos] starting path sig loop nPaths=%d", nPaths)
	for r := 0; r < nPaths; r++ {
		prevNextVal := 0
		if r < len(g.nextBeatIdxs) {
			prevNextVal = g.nextBeatIdxs[r]
		}
		var s uint64 = 1469598103934665603
		for _, bi := range g.beatInfosByRow[r] {
			// NodeType is included, so regular→silent counts as a path change.
			v := uint64(uint32(bi.I)<<16|uint32(uint16(bi.J))) ^ uint64(bi.NodeID) ^ (uint64(bi.NodeType) << 56)
			s = (s ^ v) * 1099511628211
		}
		g.rowsPathChanged[r] = (s != g.pathSigByRow[r])
		g.pathSigByRow[r] = s
		if g.rowsPathChanged[r] {
			pathsChanged = true
			g.resetLogicStateForRow(r)
			if g.drum != nil {
				g.drum.markRowDirty(r)
			}
			if r < len(g.pathChangeBeatByRow) {
				g.pathChangeBeatByRow[r] = g.elapsedBeats
			}
		}
		g.logger.Debugf("[update_beat_infos] path sig row %d changed=%v prevNext=%d", r, g.rowsPathChanged[r], prevNextVal)
		// Seed commits for traversed steps so past cells stay immutable even where they had none.
		if g.rowsPathChanged[r] && g.Playing() && g.drum != nil && r < len(g.drum.Rows) {
			next := 0
			if r < len(g.nextBeatIdxs) {
				next = g.nextBeatIdxs[r]
			}
			pastEnd := next - 1
			if pastEnd >= 0 {
				offset := g.drum.Offset
				windowEnd := offset + len(g.drum.Rows[r].Steps) - 1
				if pastEnd > windowEnd {
					pastEnd = windowEnd
				}
				if pastEnd >= offset {
					if len(g.frozenUpToByRow) != len(g.drum.Rows) {
						g.frozenUpToByRow = make([]int, len(g.drum.Rows))
						for i := range g.frozenUpToByRow {
							g.frozenUpToByRow[i] = -1
						}
					}
					if os.Getenv("DEBUG_HISTORY_SEED") == "1" {
						freezeBefore := -1
						if r < len(g.frozenUpToByRow) {
							freezeBefore = g.frozenUpToByRow[r]
						}
						fmt.Printf("[SEED] row=%d next=%d offset=%d pastEnd=%d stepsLen=%d freezeBefore=%d\n",
							r, next, offset, pastEnd, len(g.drum.Rows[r].Steps), freezeBefore)
					}
					windowLen := g.timelineWindowLen()
					capacity := windowLen
					if freeze := g.frozenUpToByRow[r]; freeze >= offset {
						if span := freeze - offset + 1; span > capacity {
							capacity = span
						}
					}
					g.timelineService().SeedFromWindow(r, offset, pastEnd, g.drum.Rows[r].Steps, g.drum.Rows[r].CellTypes, timeline.CommitKindSeeded, windowLen, capacity)
				}
			}
		}
		if g.rowsPathChanged[r] && r < len(g.nextBeatIdxs) {
			target := g.drum.Offset + 1
			maxIdx := g.drum.Offset + g.drum.Length
			for i := g.drum.Offset + 1; i < maxIdx; i++ {
				bi := g.beatInfoAtRow(r, i)
				if bi.NodeType.Fires() || bi.NodeType == model.NodeTypeMute || bi.NodeType == model.NodeTypeSilent {
					target = i
					break
				}
			}
			if target <= g.drum.Offset {
				target = g.drum.Offset + 1
			}
			freezeLimit := -1
			if r < len(g.frozenUpToByRow) {
				freezeLimit = g.frozenUpToByRow[r]
			}
			if freezeLimit >= 0 && target <= freezeLimit {
				target = freezeLimit + 1
			}
			next := target
			if next <= g.drum.Offset {
				next = g.drum.Offset + 1
			}
			if freezeLimit >= 0 && next <= freezeLimit {
				next = freezeLimit + 1
			}
			if g.rowsPathChanged[r] {
				if r < len(g.nextIdxSticky) && g.nextIdxSticky[r] {
					if r < len(prevNextByRow) {
						next = prevNextByRow[r]
					}
				} else if r < len(g.frozenUpToByRow) && prevNextVal > 0 {
					past := prevNextVal - 1
					if g.frozenUpToByRow[r] > past {
						g.frozenUpToByRow[r] = past
					}
				}
			} else if r < len(prevNextByRow) && prevNextByRow[r] > next {
				next = prevNextByRow[r]
			}
			if r < len(g.frozenUpToByRow) {
				if freezeNext := g.frozenUpToByRow[r] + 1; freezeNext > next {
					next = freezeNext
				}
			}
			g.nextBeatIdxs[r] = next
		}
	}
	g.logger.Debugf("[update_beat_infos] path sig loop done, pathsChanged=%v", pathsChanged)
	g.logger.Debugf("[update_beat_infos] starting TrimAfterPathChange loop")
	for row, changed := range g.rowsPathChanged {
		if !changed {
			continue
		}
		g.logger.Debugf("[update_beat_infos] TrimAfterPathChange row %d", row)
		cutoff := g.drum.Offset
		if row < len(g.nextBeatIdxs) {
			cutoff = g.nextBeatIdxs[row]
		}
		freezeBefore := -1
		if row < len(g.frozenUpToByRow) {
			freezeBefore = g.frozenUpToByRow[row]
		}
		var beforeStart, beforeEnd int
		beforeOK := false
		if row == 0 {
			beforeStart, beforeEnd, beforeOK = g.timelineCommittedRange(row)
		}
		report := g.timelineService().TrimAfterPathChange(row, cutoff, freezeBefore)
		g.logger.Debugf("[update_beat_infos] TrimAfterPathChange row %d done", row)
		maxKeep := report.MaxKeep
		// Stale commits past the new frontier would mask re-added nodes.
		if row < len(g.frozenUpToByRow) && g.frozenUpToByRow[row] > maxKeep {
			g.frozenUpToByRow[row] = maxKeep
		}
		freezeAfter := -1
		if row < len(g.frozenUpToByRow) {
			freezeAfter = g.frozenUpToByRow[row]
		}
		// Trace timeline trim for debugging live-edit staleness.
		if timelineTrace && row == timelineTraceRow {
			if beforeOK {
				g.logger.Debugf("[timeline] row=%d before-trim committed=[%d,%d] freezeBefore=%d cutoff=%d maxKeep=%d", row, beforeStart, beforeEnd, freezeBefore, cutoff, maxKeep)
			} else {
				g.logger.Debugf("[timeline] row=%d before-trim committed=empty freezeBefore=%d cutoff=%d maxKeep=%d", row, freezeBefore, cutoff, maxKeep)
			}
		}
		if timelineTrace && row == timelineTraceRow {
			if start, end, ok := g.timelineCommittedRange(row); ok {
				g.logger.Debugf("[timeline] row=%d after-trim committed=[%d,%d] freezeAfter=%d", row, start, end, freezeAfter)
			} else {
				g.logger.Debugf("[timeline] row=%d after-trim committed=empty freezeAfter=%d", row, freezeAfter)
			}
		}
	}
	if os.Getenv("DEBUG_HISTORY_SEED") == "1" && g.timeline != nil && len(g.drum.Rows) > 0 {
		if start, end, ok := g.timelineCommittedRange(0); ok {
			fmt.Printf("[SEED] final range row=0 [%d,%d] freeze=%v\n", start, end, g.frozenUpToByRow)
		} else {
			fmt.Printf("[SEED] final range row=0 empty freeze=%v\n", g.frozenUpToByRow)
		}
	}
	if len(g.lastHLIdxByRow) > 0 {
		for row, changed := range g.rowsPathChanged {
			if changed && row < len(g.lastHLIdxByRow) {
				g.lastHLIdxByRow[row] = -1
			}
		}
	}
	g.nextIdxSticky = nil

	// Invalidate and republish under ONE seqMu section: with separate locks the sequencer could
	// schedule against the old paths after the wipe, contradicting the new predictor.

	// Outside seqMu (under nodeCacheMu) so the hold below stays short.
	var nodes map[model.NodeID]model.Node
	if g.engine != nil && g.engine.Predictor != nil {
		nodes = make(map[model.NodeID]model.Node, len(g.nodeCache))
		g.nodeCacheMu.RLock()
		for id, node := range g.nodeCache {
			nodes[id] = node
		}
		g.nodeCacheMu.RUnlock()
	}

	clearedParity := false
	g.logger.Debugf("[update_beat_infos] about to acquire seqMu for invalidate+SetPaths")
	g.seqMu.Lock()
	g.logger.Debugf("[update_beat_infos] acquired seqMu for invalidate+SetPaths")
	if pathsChanged && g.Playing() {
		// Never rewind to the current beat: those cells are immutable, and re-scheduling them breaks
		// "edits only affect abs >= pastExclusive".
		for row, changed := range g.rowsPathChanged {
			if !changed || row < 0 || row >= len(g.seqNextIdxs) {
				continue
			}
			next := 0
			if row < len(g.nextBeatIdxs) {
				next = g.nextBeatIdxs[row]
			}
			if next < 0 {
				next = 0
			}
			if g.seqNextIdxs[row] > next {
				g.seqNextIdxs[row] = next
			}
		}
		// Per-row generations, never the global audioGen: unchanged rows are not rescheduled, so
		// dropping their beats leaves an audible gap. The parity generation advances too.
		for row, changed := range g.rowsPathChanged {
			if changed {
				g.bumpRowStructGen(row)
			}
		}
		g.bumpParityGen("paths-changed", structuralMutationOptions{
			SkipPathsDirty:     true,
			SkipPathChangeMark: true,
			SkipBufferClear:    true,
		})
		// No audio.Stop here: an edit may only change that node's future triggers, and every voice
		// already ringing keeps ringing. Stale scheduled beats are dropped above.
		g.parityMu.Lock()
		g.parityAudio = nil
		g.parityAudioMaxIdx = nil
		g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
		g.parityMu.Unlock()
		clearedParity = true
	}

	// Lock order: seqMu, then the predictor.
	if g.engine != nil && g.engine.Predictor != nil {
		setPathsStart := time.Now()
		g.engine.Predictor.SetPaths(g.beatInfosByRow, g.isLoopByRow, g.loopStartByRow, nodes)
		g.logger.Debugf("[update_beat_infos] SetPaths elapsed=%v rows=%d nodes=%d", time.Since(setPathsStart), len(g.beatInfosByRow), len(nodes))
		g.pathsDirty = false
		// Rebase at the current position so Ensure() follows the live timeline without phase drift.
		if g.Playing() {
			base := g.elapsedBeats
			if len(g.nextBeatIdxs) > 0 {
				min := g.nextBeatIdxs[0]
				for i := 1; i < len(g.nextBeatIdxs); i++ {
					if g.nextBeatIdxs[i] < min {
						min = g.nextBeatIdxs[i]
					}
				}
				if min > 0 {
					base = min
				}
			}
			if base < 0 {
				base = 0
			}
			rebaseStart := time.Now()
			g.engine.Predictor.RebaseAt(base)
			g.logger.Debugf("[update_beat_infos] RebaseAt base=%d elapsed=%v", base, time.Since(rebaseStart))
		}
	}
	g.storeSeqPathSnapshot()
	g.seqMu.Unlock()
	g.logger.Debugf("[update_beat_infos] released seqMu after invalidate+SetPaths")
	if clearedParity {
		g.ClearParityMismatches()
	}

	// Set dirty flag so next Update refreshes DrumView immediately on edit.
	g.pathsDirty = true

	// Compute ahead for current window plus a small lookahead to keep UI snappy.
	div2 := 32
	if g.grid != nil {
		div2 = g.grid.MaxDiv()
	}
	lookahead := div2 * 32
	horizon := g.drum.Offset + g.drum.Length + lookahead
	if horizon < g.drum.Length {
		horizon = g.drum.Length
	}
	g.logger.Debugf("[update_beat_infos] calling Ensure horizon=%d offset=%d length=%d lookahead=%d rows=%d",
		horizon, g.drum.Offset, g.drum.Length, lookahead, len(g.beatInfosByRow))
	ensureStart := time.Now()
	g.engine.Predictor.Ensure(horizon)
	g.logger.Debugf("[update_beat_infos] Ensure elapsed=%v", time.Since(ensureStart))
	// Stopped: rebuild now so callers see the window. Playing: defer to Update so highlights advance
	// the row counters before parity scans.
	if !g.Playing() {
		refreshStart := time.Now()
		g.refreshDrumRow()
		g.logger.Debugf("[update_beat_infos] refreshDrumRow elapsed=%v", time.Since(refreshStart))
	}
	// Warm upcoming voices off audioLoop, so PlayBatch never cold-renders inline.
	if g.voicePrewarmer != nil {
		g.voicePrewarmer.warmFromBeatInfos()
	}
}

func (g *Game) updateBeatInfos() {
	g.playbackSlate.updateBeatInfos()
}
