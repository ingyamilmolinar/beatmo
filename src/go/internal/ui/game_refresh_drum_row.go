package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (s *playbackSlate) refreshDrumRow() {
	g := s.g
	refreshStart := time.Now()
	defer func() { g.logger.Tracef("[refresh] total=%v rows=%d", time.Since(refreshStart), len(g.drum.Rows)) }()

	if len(g.drum.Rows) == 0 {
		g.drumBeatInfos = nil
		return
	}
	// Before a pending SetPaths can change what they show.
	g.freezePlayedThroughPlayhead()
	// Mark render state as rebuilding so parity checks ignore in-flight buffers.
	g.renderReady = false
	// Always use predictions (single source of truth). Ensure horizon covers window.
	if cap(g.drumBeatInfos) < g.drum.Length {
		g.drumBeatInfos = make([]model.BeatInfo, g.drum.Length)
	} else {
		g.drumBeatInfos = g.drumBeatInfos[:g.drum.Length]
	}
	for i := range g.drumBeatInfos {
		g.drumBeatInfos[i] = g.beatInfoAtRow(0, g.drum.Offset+i)
	}
	horizon := g.drum.Offset + g.drum.Length
	// Before rendering, so VisibleAt/TriggeredAt see new or deleted nodes at once.
	if g.engine != nil && g.engine.Predictor != nil && g.pathsDirty {
		g.engine.Predictor.SetPaths(g.beatInfosByRow, g.isLoopByRow, g.loopStartByRow, g.graph.Nodes)
		g.pathsDirty = false
		// A path change invalidates every row's reconciliation watermark.
		g.invalidateReconciledAll()
	}
	// Ensure horizon covers the rendered window (must run after SetPaths since it resets buffers).
	if g.engine != nil && g.engine.Predictor != nil {
		// Before Ensure, so its slide clamp honours the visible region; otherwise after pause/resume the
		// window may have advanced past drum.Offset and only extends forward (grey grid).
		g.engine.Predictor.SetVisibleMinAbs(g.drum.Offset)
		g.engine.Predictor.Ensure(horizon)
	}
	prevStepsOffset := g.lastStepsOffset
	// Ensure signature buffer matches row count.
	if len(g.rowRenderSig) != len(g.drum.Rows) {
		g.rowRenderSig = make([]uint64, len(g.drum.Rows))
	}
	if !g.rowSnapshotMode {
		if len(g.rowStepsScratch) != len(g.drum.Rows) {
			g.rowStepsScratch = make([][]bool, len(g.drum.Rows))
		}
		if len(g.rowTypesScratch) != len(g.drum.Rows) {
			g.rowTypesScratch = make([][]model.NodeType, len(g.drum.Rows))
		}
	}
	scanTimeline := true
	if g.Playing() {
		every, _ := g.parityScanSettings()
		scanTimeline = g.parityScanDue(every)
	}
	for rowIdx, r := range g.drum.Rows {
		rowStart := time.Now()
		oldSteps := r.Steps
		oldTypes := r.CellTypes
		prevSteps := oldSteps
		prevTypes := oldTypes
		if g.rowSnapshotMode {
			prevSteps = append([]bool(nil), oldSteps...)
			prevTypes = append([]model.NodeType(nil), oldTypes...)
		}
		freezeLimit := -1
		if rowIdx < len(g.frozenUpToByRow) {
			freezeLimit = g.frozenUpToByRow[rowIdx]
		}
		fast := g.perfMode.FastPathEnabled()
		windowStart := g.drum.Offset
		traceRow := timelineTrace && rowIdx == timelineTraceRow
		predictAt := func(abs int, bi model.BeatInfo) bool {
			if g.engine == nil || g.engine.Predictor == nil {
				return false
			}
			if bi.NodeType == model.NodeTypeMute {
				return g.engine.Predictor.TriggeredAt(rowIdx, abs)
			}
			return g.engine.Predictor.VisibleAt(rowIdx, abs)
		}
		futureReleaseStart := windowStart
		if g.Playing() {
			nextAbs := g.rowPastExclusive(rowIdx)
			if nextAbs > futureReleaseStart {
				futureReleaseStart = nextAbs
			}
		}
		syncFreezeLimit := func() {
			if rowIdx < len(g.frozenUpToByRow) {
				freezeLimit = g.frozenUpToByRow[rowIdx]
			}
		}
		reconcileStart := time.Now()
		reconcileFrozen := func() {
			if freezeLimit < 0 {
				return
			}
			limit := freezeLimit
			windowEnd := windowStart + g.drum.Length - 1
			if g.drum.Length <= 0 {
				windowEnd = windowStart - 1
			}
			if limit > windowEnd {
				limit = windowEnd
			}
			if limit < futureReleaseStart {
				return
			}
			// Resume past the last reconciled abs, so each refresh scans only the newly frozen cells.
			start := futureReleaseStart
			if rowIdx < len(g.reconciledUpToByRow) {
				if r := g.reconciledUpToByRow[rowIdx]; r >= start {
					start = r + 1
				}
			}
			if start > limit {
				return
			}
			for abs := start; abs <= limit; abs++ {
				if v, _, kind, ok := g.timelineCommittedWithKind(rowIdx, abs); ok {
					bi := g.beatInfoAtRow(rowIdx, abs)
					if g.Playing() && (bi.NodeType.Fires() || bi.NodeType == model.NodeTypeSilent) &&
						bi.NodeType != model.NodeTypeInvisible && bi.NodeID != model.InvalidNodeID {
						continue
					}
					want := predictAt(abs, bi)
					if want != v {
						// Immutable history is never trimmed.
						if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport {
							continue
						}
						// Mutable seeds and gap pads reconcile in place, without dropping later immutable commits.
						inPast := abs < g.rowPastExclusive(rowIdx)
						switch kind {
						case timeline.CommitKindSeeded, timeline.CommitKindGapPad:
							if inPast {
								g.clampRowFreeze(rowIdx, abs-1)
								g.timelineTrimAfterMutable(rowIdx, abs-1)
								if traceRow {
									g.logger.Debugf("[timeline] release mutable past mask row=%d abs=%d masked=%v want=%v newFreeze=%d",
										rowIdx, abs, v, want, g.frozenUpToByRow[rowIdx])
								}
								limit = g.frozenUpToByRow[rowIdx]
								if limit < abs {
									break
								}
							} else {
								// Future window: avoid trimming; just align the mask entry.
								g.timelineReplaceCommit(rowIdx, abs, want, bi.NodeType, timeline.CommitKindReleased)
							}
						case timeline.CommitKindReleased:
							// Released commits are never rewritten; one leaking into the frozen future shrinks the freeze limit.
							if !inPast {
								g.clampRowFreeze(rowIdx, abs-1)
								if traceRow {
									g.logger.Debugf("[timeline] clamp future freeze after released row=%d abs=%d masked=%v want=%v newFreeze=%d",
										rowIdx, abs, v, want, g.frozenUpToByRow[rowIdx])
								}
								limit = g.frozenUpToByRow[rowIdx]
								if limit < abs {
									break
								}
							}
						default:
							g.clampRowFreeze(rowIdx, abs-1)
							g.timelineTrimAfter(rowIdx, abs-1)
							if traceRow {
								g.logger.Debugf("[timeline] releasing future freeze row=%d abs=%d masked=%v want=%v newFreeze=%d",
									rowIdx, abs, v, want, g.frozenUpToByRow[rowIdx])
							}
							limit = g.frozenUpToByRow[rowIdx]
							if limit < abs {
								break
							}
						}
					}
				}
			}
			syncFreezeLimit()
			// Every abs in [start, limit] was inspected, so the next refresh starts at limit+1.
			if rowIdx >= len(g.reconciledUpToByRow) {
				oldLen := len(g.reconciledUpToByRow)
				g.reconciledUpToByRow = append(g.reconciledUpToByRow, make([]int, rowIdx+1-oldLen)...)
				for i := oldLen; i <= rowIdx; i++ {
					g.reconciledUpToByRow[i] = -1
				}
			}
			if limit > g.reconciledUpToByRow[rowIdx] {
				g.reconciledUpToByRow[rowIdx] = limit
			}
		}
		reconcileFrozen()
		reconcileElapsed := time.Since(reconcileStart)
		if reconcileElapsed > 10*time.Millisecond {
			g.logger.Warnf("[refresh] row=%d reconcileFrozen slow elapsed=%v freezeLimit=%d futureReleaseStart=%d", rowIdx, reconcileElapsed, freezeLimit, futureReleaseStart)
		}
		// If the sequencer ran past this window before the timeline committed, freeze everything before
		// the next-beat index so later edits cannot rewrite the past.
		if g.Playing() {
			next := g.rowPastExclusive(rowIdx)
			if next > 0 {
				if len(g.frozenUpToByRow) != len(g.drum.Rows) {
					g.frozenUpToByRow = make([]int, len(g.drum.Rows))
					for i := range g.frozenUpToByRow {
						g.frozenUpToByRow[i] = -1
					}
				}
				if len(g.reconciledUpToByRow) != len(g.drum.Rows) {
					g.reconciledUpToByRow = make([]int, len(g.drum.Rows))
					for i := range g.reconciledUpToByRow {
						g.reconciledUpToByRow[i] = -1
					}
				}
				upTo := g.frozenUpToByRow[rowIdx]
				target := next - 1
				if target > upTo {
					pathChangeBeat := -1
					if rowIdx < len(g.pathChangeBeatByRow) {
						pathChangeBeat = g.pathChangeBeatByRow[rowIdx]
					}
					// Bounded to the predictor window: below it prediction is meaningless, above it absent. Unbounded,
					// a resume re-froze the whole session in one frame and stalled for seconds.
					lo := upTo + 1
					predWinEnd := target + 1
					if g.engine != nil && g.engine.Predictor != nil {
						if ws := g.engine.Predictor.WindowStart(); ws > lo {
							lo = ws
						}
						predWinEnd = g.engine.Predictor.Horizon()
					}
					for j := lo; j <= target && j < predWinEnd; j++ {
						g.freezeBackfillIters++
						if pathChangeBeat >= 0 && j < pathChangeBeat {
							// Commits before the last path change stay immutable.
							continue
						}
						if _, _, kind, ok := g.timelineCommittedWithKind(rowIdx, j); ok {
							if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport {
								continue
							}
						}
						bi := g.beatInfoAtRow(rowIdx, j)
						val := predictAt(j, bi)
						// Released, NOT Playback: applySequencerHighlight is the only source of immutable playback commits,
						// so a wrong speculation here can still be corrected by the next reconcile.
						g.recordTimelineCommitKind(rowIdx, j, val, bi.NodeType, timeline.CommitKindReleased)
					}
					g.frozenUpToByRow[rowIdx] = target
					freezeLimit = target
				}
			}
		}
		// Relax immutability only for the future window, so re-adds show at once; value and type stay.
		nextIdx := g.rowPastExclusive(rowIdx)
		if nextIdx > 0 {
			end := windowStart + g.drum.Length
			if end < windowStart {
				end = windowStart
			}
			for abs := nextIdx; abs < end; abs++ {
				if abs < windowStart {
					continue
				}
				if v, typ, kind, ok := g.timelineCommittedWithKind(rowIdx, abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
					g.timelineReplaceCommit(rowIdx, abs, v, typ, timeline.CommitKindReleased)
				}
			}
		}
		reuseSteps := []bool(nil)
		reuseTypes := []model.NodeType(nil)
		if !g.rowSnapshotMode && rowIdx < len(g.rowStepsScratch) && rowIdx < len(g.rowTypesScratch) {
			reuseSteps = g.rowStepsScratch[rowIdx]
			reuseTypes = g.rowTypesScratch[rowIdx]
		}
		buildStart := time.Now()
		nextSteps, nextTypes := g.buildRowWindow(rowIdx, rowWindowConfig{
			freezeLimit:     freezeLimit,
			prevSteps:       prevSteps,
			prevStepsOffset: prevStepsOffset,
			prevTypes:       prevTypes,
			prevTypesOffset: g.lastCellTypesOffset,
			fastPath:        fast,
			predictAt:       predictAt,
			reuseSteps:      reuseSteps,
			reuseTypes:      reuseTypes,
		})
		buildElapsed := time.Since(buildStart)
		if buildElapsed > 10*time.Millisecond {
			g.logger.Warnf("[refresh] row=%d buildRowWindow slow elapsed=%v length=%d", rowIdx, buildElapsed, g.drum.Length)
		}
		r.Steps = nextSteps
		r.CellTypes = nextTypes
		if !g.rowSnapshotMode && rowIdx < len(g.rowStepsScratch) && rowIdx < len(g.rowTypesScratch) {
			// Reuse the previous published slices as scratch for the next refresh.
			g.rowStepsScratch[rowIdx] = oldSteps
			g.rowTypesScratch[rowIdx] = oldTypes
		}

		// Paused: keep speculative entries on the predictor so stale seeds cannot trip parity.
		if !g.Playing() && freezeLimit < 0 {
			g.reconcileMutableTimeline(rowIdx, windowStart, g.drum.Length, predictAt)
		}

		if scanTimeline && (parityFatalEnabled.Load() || g.parityWatch != parityWatchOff || (timelineTrace && rowIdx == timelineTraceRow)) {
			if mismatches := g.diffPredictorTimeline(rowIdx, windowStart, g.drum.Length, freezeLimit); len(mismatches) > 0 {
				indices := make([]int, 0, len(mismatches))
				for _, m := range mismatches {
					indices = append(indices, m.Abs)
					g.parityReport(m)
				}
				if traceRow || !timelineTrace {
					g.logger.Debugf("[timeline] mismatches=%d row=%d abs=%v", len(mismatches), rowIdx, indices)
				} else {
					g.logger.Tracef("[timeline] mismatches=%d row=%d abs=%v", len(mismatches), rowIdx, indices)
				}
			}
		}

		// Auxiliary: the caches render from Steps/CellTypes only.
		windowLen := g.drum.Length
		capacity := windowLen
		g.timelineService().UpdateRowSegments(rowIdx, windowStart, windowLen, capacity, r.Steps, func(view timeline.SegmentsView) {
			g.drum.setTimelineSegments(rowIdx, view.Offset, view.Past, view.PastTypes, view.Present, view.Future)
		})
		if timelineTrace && rowIdx == timelineTraceRow {
			// Small debug trace to catch stale past masking re-added nodes.
			pastCount := 0
			for _, v := range g.timelineService().Snapshot(rowIdx).PastMask {
				if v {
					pastCount++
				}
			}
			imm := g.lastImmutableCommit(rowIdx)
			g.logger.Tracef("[timeline/refresh] row=%d offset=%d freeze=%d pastMaskCount=%d lastImmutable=%d", rowIdx, windowStart, freezeLimit, pastCount, imm)
		}

		// Cache invalidation: mark row dirty only when render-relevant inputs change.
		if rowIdx < len(g.rowRenderSig) {
			sig := rowRenderSignature(r.Steps, r.CellTypes)
			if g.rowRenderSig[rowIdx] != sig {
				softDirty := false
				if prevStepsOffset != g.drum.Offset && prevStepsOffset == g.lastCellTypesOffset {
					if rowShiftCompatible(prevSteps, prevTypes, r.Steps, r.CellTypes, g.drum.Offset-prevStepsOffset) {
						softDirty = true
					}
				}
				if softDirty {
					g.drum.markRowShiftDirty(rowIdx)
				} else if g.Playing() && len(r.Steps) == len(prevSteps) {
					// A few changed cells take the cheap patch path instead of a full rebuild.
					diff := countCellDiffs(prevSteps, prevTypes, r.Steps, r.CellTypes)
					if diff > 0 {
						g.drum.markRowCellsDirty(rowIdx)
					}
				} else {
					g.drum.markRowDirty(rowIdx)
				}
			}
			g.rowRenderSig[rowIdx] = sig
		}
		rowElapsed := time.Since(rowStart)
		if rowElapsed > 20*time.Millisecond {
			g.logger.Warnf("[refresh] row=%d total slow elapsed=%v", rowIdx, rowElapsed)
		}
	}
	// So past cell types survive while the window stays put.
	g.lastStepsOffset = g.drum.Offset
	g.lastCellTypesOffset = g.drum.Offset
	g.renderOffset = g.drum.Offset
	g.renderLength = g.drum.Length
	g.renderFrame = g.frame
	// Clear length change flag after rows are rebuilt
	if g.drum != nil && g.drum.lengthChanging {
		g.drum.lengthChanging = false
	}
	g.renderReady = true
	g.logger.Tracef("[game/refresh] refreshDrumRow offset=%d", g.drum.Offset)
	g.parityScan("refresh")
	g.parityTimingCheck()
}

func (g *Game) refreshDrumRow() { g.playbackSlate.refreshDrumRow() }

type rowWindowConfig struct {
	freezeLimit     int
	prevSteps       []bool
	prevStepsOffset int
	prevTypes       []model.NodeType
	prevTypesOffset int
	fastPath        bool
	predictAt       func(int, model.BeatInfo) bool
	reuseSteps      []bool
	reuseTypes      []model.NodeType
}

// Equal-length slices.
func countCellDiffs(prevSteps []bool, prevTypes []model.NodeType, nextSteps []bool, nextTypes []model.NodeType) int {
	n := len(nextSteps)
	diff := 0
	for j := 0; j < n; j++ {
		if nextSteps[j] != prevSteps[j] {
			diff++
			continue
		}
		if j < len(nextTypes) && j < len(prevTypes) && nextTypes[j] != prevTypes[j] {
			diff++
		}
	}
	return diff
}

func rowShiftCompatible(prevSteps []bool, prevTypes []model.NodeType, nextSteps []bool, nextTypes []model.NodeType, delta int) bool {
	if delta == 0 {
		return false
	}
	if len(prevSteps) != len(nextSteps) || len(prevTypes) != len(nextTypes) {
		return false
	}
	n := len(nextSteps)
	for i := 0; i < n; i++ {
		j := i + delta
		if j < 0 || j >= n {
			continue
		}
		if nextSteps[i] != prevSteps[j] {
			return false
		}
		if nextTypes[i] != prevTypes[j] {
			return false
		}
	}
	return true
}
