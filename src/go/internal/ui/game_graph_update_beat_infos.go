package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (g *Game) updateBeatInfos() {
	start := time.Now()
	defer func() { g.logger.Infof("[UPDATE_BEAT_INFOS] total=%v", time.Since(start)) }()

	g.rebuildNodeCache()
	g.logger.Debugf("[UPDATE_BEAT_INFOS] rebuildNodeCache elapsed=%v", time.Since(start))

	// Use a generously large beat length so CalculateBeatRow returns the
	// complete path even when disconnected nodes exist elsewhere in the
	// graph. We'll shrink the beat length back to the actual traversal size
	// after computing the raw path length.
	// Build unbounded path so we capture all intermediate steps without
	// relying on Graph.BeatLength. This avoids truncation now that invisible
	// steps are synthesized on-the-fly instead of stored as nodes.
	calcStart := time.Now()
	fullBeatRow, isLoop, loopStart := g.graph.CalculateBeatRowUnbounded()
	g.logger.Debugf("[UPDATE_BEAT_INFOS] CalculateBeatRowUnbounded elapsed=%v isLoop=%v loopStart=%d pathLen=%d",
		time.Since(calcStart), isLoop, loopStart, len(fullBeatRow))
	baseLen := rawBeatLen(fullBeatRow, isLoop, loopStart)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] rawBeatLen=%d", baseLen)

	g.beatInfos = fullBeatRow[:baseLen]
	g.isLoop = isLoop
	g.loopStartIndex = loopStart

	maxLen := baseLen
	g.nodeRows = map[model.NodeID]int{}
	nRows := len(g.drum.Rows)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] nRows=%d drum.Rows=%d", nRows, len(g.drum.Rows))
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
	g.logger.Debugf("[UPDATE_BEAT_INFOS] before row0 setup nRows=%d", nRows)
	if nRows > 0 {
		g.beatInfosByRow[0] = g.beatInfos
		g.isLoopByRow[0] = isLoop
		g.loopStartByRow[0] = loopStart
		if isLoop {
			g.loopLenByRow[0] = loopSegmentLen(g.beatInfos, loopStart)
		}
		origin := g.drum.Rows[0].Origin
		g.logger.Debugf("[UPDATE_BEAT_INFOS] row0 origin=%d beatInfosLen=%d", origin, len(g.beatInfos))
		for idx, b := range g.beatInfos {
			if b.NodeID != model.InvalidNodeID {
				g.nodeRows[b.NodeID] = 0
			}
			if b.NodeID == origin {
				g.originIdxsByRow[0] = append(g.originIdxsByRow[0], idx)
			}
		}
		g.logger.Debugf("[UPDATE_BEAT_INFOS] row0 setup done")
	}

	g.logger.Debugf("[UPDATE_BEAT_INFOS] before row loop, nRows=%d", nRows)
	// Compute beat paths for additional drum rows using their origin nodes.
	for i, r := range g.drum.Rows {
		g.logger.Debugf("[UPDATE_BEAT_INFOS] processing row %d origin=%d", i, r.Origin)
		if i == 0 {
			// row 0 handled above; ensure its origin tracks the start node
			if r.Origin == model.InvalidNodeID && g.start != nil {
				g.drum.Rows[0].Origin = g.start.ID
				g.drum.Rows[0].Node = g.start
			}
			continue
		}
		if r.Origin == model.InvalidNodeID {
			g.logger.Debugf("[UPDATE_BEAT_INFOS] row %d has InvalidNodeID origin, skipping", i)
			continue
		}
		g.logger.Debugf("[UPDATE_BEAT_INFOS] row %d calling CalculateBeatRowFrom origin=%d", i, r.Origin)
		rowPath, rowLoop, rowStart := g.graph.CalculateBeatRowFrom(r.Origin)
		g.logger.Debugf("[UPDATE_BEAT_INFOS] row %d CalculateBeatRowFrom done pathLen=%d", i, len(rowPath))
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

	// Reduce the graph's beat length to the actual maximum traversal size so
	// subsequent path calculations are not padded with extra loop cycles.
	g.logger.Debugf("[UPDATE_BEAT_INFOS] after row loop, maxLen=%d", maxLen)
	g.graph.SetBeatLength(maxLen)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] SetBeatLength done")

	g.resetOriginSequences()
	g.logger.Debugf("[UPDATE_BEAT_INFOS] resetOriginSequences done")
	g.muteUntilByRow = make([]int, len(g.drum.Rows))
	g.renderReady = false

	// Reset per-row trigger counters so node logic starts fresh whenever the
	// beat paths are recomputed (e.g., graph edits, origin changes).
	g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	g.lastEvalIdxByRowNode = make(map[int]map[model.NodeID]int)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] trigger counters reset")

	if maxLen < 1 {
		maxLen = 1
	}
	if !g.Playing() && maxLen > g.drum.Length {
		g.logger.Debugf("[UPDATE_BEAT_INFOS] calling drum.SetLengthClamped maxLen=%d", maxLen)
		g.drum.SetLengthClamped(maxLen)
		g.logger.Debugf("[UPDATE_BEAT_INFOS] drum.SetLengthClamped done")
	} else {
		// While playing, avoid changing DrumView.Length to prevent window
		// clamping jumps; update only the underlying graph beat length.
		g.drum.SetBeatLength(maxLen)
	}

	// Rebind active pulses to the updated beat paths while preserving
	// their absolute progression indices.
	g.logger.Debugf("[UPDATE_BEAT_INFOS] rebinding %d active pulses", len(g.activePulses))
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

	g.logger.Debugf("[UPDATE_BEAT_INFOS] active pulses done")
	g.logger.Debugf("[GAME] updateBeatInfos: drum.Length=%d, beatPath=%d", g.drum.Length, len(g.beatInfos))
	// Log per-row path sizes to aid debugging imports/demo configs.
	if len(g.beatInfosByRow) > 0 {
		sizes := make([]int, len(g.beatInfosByRow))
		for i := range g.beatInfosByRow {
			sizes[i] = len(g.beatInfosByRow[i])
		}
		g.logger.Debugf("[GAME] per-row path lens: %v", sizes)
	}
	g.logger.Debugf("[UPDATE_BEAT_INFOS] after per-row path lens")

	// Preserve current drum offset when the beat path changes. Clamp against
	// the timeline length (in subdivisions) rather than the raw beat path so
	// tracking can continue beyond the initial graph traversal.
	units := max1(g.drum.timelineUnitsPerBeat)
	maxOffset := g.drum.timelineBeats*units - g.drum.Length
	if maxOffset < 0 {
		maxOffset = 0
	}
	if g.drum.Offset > maxOffset {
		g.drum.Offset = maxOffset
	}
	g.logger.Debugf("[UPDATE_BEAT_INFOS] offset clamped")

	// Compute path signatures and detect rows whose shape changed.
	nPaths := len(g.beatInfosByRow)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] computing path signatures nPaths=%d", nPaths)
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
	g.logger.Debugf("[UPDATE_BEAT_INFOS] starting path sig loop nPaths=%d", nPaths)
	for r := 0; r < nPaths; r++ {
		prevNextVal := 0
		if r < len(g.nextBeatIdxs) {
			prevNextVal = g.nextBeatIdxs[r]
		}
		var s uint64 = 1469598103934665603
		for _, bi := range g.beatInfosByRow[r] {
			// Include NodeType so audibility/visibility-affecting edits (e.g.,
			// regular->silent) are treated as path changes for scheduling/parity.
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
		g.logger.Debugf("[UPDATE_BEAT_INFOS] path sig row %d changed=%v prevNext=%d", r, g.rowsPathChanged[r], prevNextVal)
		// If the path changed while playing, seed commits for already-traversed
		// steps in the current window so past cells remain immutable even when
		// they previously lacked explicit timeline entries (e.g., invisible gaps).
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
				if bi.NodeType == model.NodeTypeRegular || bi.NodeType == model.NodeTypeMute || bi.NodeType == model.NodeTypeSilent {
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
	g.logger.Debugf("[UPDATE_BEAT_INFOS] path sig loop done, pathsChanged=%v", pathsChanged)
	g.logger.Debugf("[UPDATE_BEAT_INFOS] starting TrimAfterPathChange loop")
	for row, changed := range g.rowsPathChanged {
		if !changed {
			continue
		}
		g.logger.Debugf("[UPDATE_BEAT_INFOS] TrimAfterPathChange row %d", row)
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
		g.logger.Debugf("[UPDATE_BEAT_INFOS] TrimAfterPathChange row %d done", row)
		maxKeep := report.MaxKeep
		// Avoid freezing future entries past the updated path frontier: clamp
		// frozenUpTo to the new maxKeep so stale commits don't mask re-added
		// nodes in the DrumView.
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
				g.logger.Infof("[TIMELINE] row=%d before-trim committed=[%d,%d] freezeBefore=%d cutoff=%d maxKeep=%d", row, beforeStart, beforeEnd, freezeBefore, cutoff, maxKeep)
			} else {
				g.logger.Infof("[TIMELINE] row=%d before-trim committed=empty freezeBefore=%d cutoff=%d maxKeep=%d", row, freezeBefore, cutoff, maxKeep)
			}
		}
		if timelineTrace && row == timelineTraceRow {
			if start, end, ok := g.timelineCommittedRange(row); ok {
				g.logger.Infof("[TIMELINE] row=%d after-trim committed=[%d,%d] freezeAfter=%d", row, start, end, freezeAfter)
			} else {
				g.logger.Infof("[TIMELINE] row=%d after-trim committed=empty freezeAfter=%d", row, freezeAfter)
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
	if pathsChanged {
		g.notifyComponentsGraphChange(GraphChange{PathsChanged: true})
	}

	// If paths changed during playback, invalidate any already-scheduled audio
	// and parity buffers. Otherwise, a live edit can leave an "old" audio event
	// queued while the predictor/drumview has already advanced to the new path,
	// producing audio_unexpected mismatches.
	if pathsChanged && g.Playing() {
		// Rewind the sequencer counters for affected rows to the next-beat boundary
		// so the next scheduling tick can re-enqueue any audio that was just
		// canceled by the generation bump / Stop calls below. Never rewind to the
		// current beat (pastExclusive-1): those cells are already immutable, and
		// re-scheduling them would violate the "edits only affect abs>=pastExclusive"
		// invariant and trip parity (audio_vs_view).
		g.logger.Debugf("[UPDATE_BEAT_INFOS] about to acquire seqMu for pathsChanged rewind")
		g.seqMu.Lock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] acquired seqMu for pathsChanged rewind")
		for row, changed := range g.rowsPathChanged {
			if !changed {
				continue
			}
			if row < 0 || row >= len(g.seqNextIdxs) {
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
		g.seqMu.Unlock()

		g.audioGen.Add(1)
		for row, changed := range g.rowsPathChanged {
			if !changed || row < 0 || row >= len(g.drum.Rows) {
				continue
			}
			audio.Stop(g.drum.Rows[row].Instrument)
		}
		g.parityMu.Lock()
		g.parityAudio = nil
		g.parityAudioMaxIdx = nil
		g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
		g.parityMu.Unlock()
		g.ClearParityMismatches()
	}

	// Update engine predictor with the new paths so scheduling/preview uses
	// the authoritative engine-owned buffers. This must run after any
	// seqMu-protected rewinds to avoid lock-order inversions with the
	// background sequencer (seqMu -> predictor).
	if g.engine != nil && g.engine.Predictor != nil {
		nodes := make(map[model.NodeID]model.Node, len(g.nodeCache))
		g.nodeCacheMu.RLock()
		for id, node := range g.nodeCache {
			nodes[id] = node
		}
		g.nodeCacheMu.RUnlock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] about to acquire seqMu for SetPaths")
		g.seqMu.Lock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] acquired seqMu for SetPaths")
		setPathsStart := time.Now()
		g.engine.Predictor.SetPaths(g.beatInfosByRow, g.isLoopByRow, g.loopStartByRow, nodes)
		g.logger.Debugf("[UPDATE_BEAT_INFOS] SetPaths elapsed=%v rows=%d nodes=%d", time.Since(setPathsStart), len(g.beatInfosByRow), len(nodes))
		g.pathsDirty = false
		// Rebase predictor contexts at the current absolute position so
		// subsequent Ensure() uses the live timeline, avoiding phase drift.
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
			g.logger.Debugf("[UPDATE_BEAT_INFOS] RebaseAt base=%d elapsed=%v", base, time.Since(rebaseStart))
		}
		g.storeSeqPathSnapshot()
		g.seqMu.Unlock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] released seqMu after SetPaths")
	} else {
		g.logger.Debugf("[UPDATE_BEAT_INFOS] about to acquire seqMu for storeSeqPathSnapshot (no predictor)")
		g.seqMu.Lock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] acquired seqMu for storeSeqPathSnapshot (no predictor)")
		g.storeSeqPathSnapshot()
		g.seqMu.Unlock()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] released seqMu for storeSeqPathSnapshot (no predictor)")
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
	g.logger.Infof("[UPDATE_BEAT_INFOS] calling Ensure horizon=%d offset=%d length=%d lookahead=%d rows=%d",
		horizon, g.drum.Offset, g.drum.Length, lookahead, len(g.beatInfosByRow))
	ensureStart := time.Now()
	g.engine.Predictor.Ensure(horizon)
	g.logger.Infof("[UPDATE_BEAT_INFOS] Ensure elapsed=%v", time.Since(ensureStart))
	// When stopped/paused, rebuild DrumView immediately so callers (tests,
	// import/export, edit flows) observe the updated window without waiting for
	// the next Update(). While playing, defer refresh to the main Update path
	// so queued highlights/syncUIToTime can advance row counters before parity
	// scans run, avoiding false highlight/audio mismatches during structural edits.
	if !g.Playing() {
		refreshStart := time.Now()
		g.refreshDrumRow()
		g.logger.Debugf("[UPDATE_BEAT_INFOS] refreshDrumRow elapsed=%v", time.Since(refreshStart))
	}
}
