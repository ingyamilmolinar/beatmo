package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (g *Game) refreshDrumRow() {
	refreshStart := time.Now()
	defer func() { g.logger.Tracef("[REFRESH] total=%v rows=%d", time.Since(refreshStart), len(g.drum.Rows)) }()

	if len(g.drum.Rows) == 0 {
		g.drumBeatInfos = nil
		return
	}
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
	// If paths changed since the last refresh, push the latest beatInfos into
	// the engine predictor before we render so VisibleAt/TriggeredAt reflect
	// newly added/deleted nodes immediately.
	if g.engine != nil && g.engine.Predictor != nil && g.pathsDirty {
		g.engine.Predictor.SetPaths(g.beatInfosByRow, g.isLoopByRow, g.loopStartByRow, g.graph.Nodes)
		g.pathsDirty = false
	}
	// Ensure horizon covers the rendered window (must run after SetPaths since it resets buffers).
	if g.engine != nil && g.engine.Predictor != nil {
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
			// Only scan the currently rendered window; reconcile additional
			// frozen history when the window reaches it.
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
			for abs := futureReleaseStart; abs <= limit; abs++ {
				if v, _, kind, ok := g.timelineCommittedWithKind(rowIdx, abs); ok {
					bi := g.beatInfoAtRow(rowIdx, abs)
					if g.Playing() && (bi.NodeType == model.NodeTypeRegular || bi.NodeType == model.NodeTypeSilent) &&
						bi.NodeType != model.NodeTypeInvisible && bi.NodeID != model.InvalidNodeID {
						continue
					}
					want := predictAt(abs, bi)
					if want != v {
						// Immutable playback/import history should never be trimmed. Keep
						// the commit and render/drain from predictor instead.
						if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport {
							continue
						}
						// Mutable seeds/gap pads: reconcile in place to the predictor
						// value so history matches playback without dropping later
						// immutable commits.
						inPast := abs < g.rowPastExclusive(rowIdx)
						switch kind {
						case timeline.CommitKindSeeded, timeline.CommitKindGapPad:
							if inPast {
								g.clampRowFreeze(rowIdx, abs-1)
								g.timelineTrimAfterMutable(rowIdx, abs-1)
								if traceRow {
									g.logger.Infof("[TIMELINE] release mutable past mask row=%d abs=%d masked=%v want=%v newFreeze=%d",
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
							// Released commits are bookkeeping and should never be rewritten.
							// If a released entry leaks into a frozen future range, shrink the
							// freeze limit so upcoming beats can follow the predictor again.
							if !inPast {
								g.clampRowFreeze(rowIdx, abs-1)
								if traceRow {
									g.logger.Infof("[TIMELINE] clamp future freeze after released row=%d abs=%d masked=%v want=%v newFreeze=%d",
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
								g.logger.Infof("[TIMELINE] releasing future freeze row=%d abs=%d masked=%v want=%v newFreeze=%d",
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
		}
		reconcileFrozen()
		reconcileElapsed := time.Since(reconcileStart)
		if reconcileElapsed > 10*time.Millisecond {
			g.logger.Infof("[REFRESH] row=%d reconcileFrozen slow elapsed=%v freezeLimit=%d futureReleaseStart=%d", rowIdx, reconcileElapsed, freezeLimit, futureReleaseStart)
		}
		// If the sequencer advanced past this window before the timeline
		// recorded commits (e.g., during short play bursts in tests), ensure
		// every subdivision strictly before the next-beat index is frozen as
		// immutable playback so later edits cannot mutate the rendered past.
		if g.Playing() {
			next := g.rowPastExclusive(rowIdx)
			if next > 0 {
				if len(g.frozenUpToByRow) != len(g.drum.Rows) {
					g.frozenUpToByRow = make([]int, len(g.drum.Rows))
					for i := range g.frozenUpToByRow {
						g.frozenUpToByRow[i] = -1
					}
				}
				upTo := g.frozenUpToByRow[rowIdx]
				target := next - 1
				if target > upTo {
					pathChangeBeat := -1
					if rowIdx < len(g.pathChangeBeatByRow) {
						pathChangeBeat = g.pathChangeBeatByRow[rowIdx]
					}
					for j := upTo + 1; j <= target; j++ {
						if pathChangeBeat >= 0 && j < pathChangeBeat {
							// Preserve pre-change history; avoid rewriting commits that predate
							// the last path mutation to keep earlier playback immutable.
							continue
						}
						if _, _, kind, ok := g.timelineCommittedWithKind(rowIdx, j); ok {
							if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport {
								continue
							}
						}
						bi := g.beatInfoAtRow(rowIdx, j)
						val := predictAt(j, bi)
						g.recordTimelineCommitKind(rowIdx, j, val, bi.NodeType, timeline.CommitKindPlayback)
					}
					g.frozenUpToByRow[rowIdx] = target
					freezeLimit = target
				}
			}
		}
		// Demote any leaked immutable commits that fall at/after the row's current
		// "next" index so upcoming edits/re-adds can reflect immediately. Preserve
		// the recorded value/type; only relax immutability for the future window.
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
			g.logger.Infof("[REFRESH] row=%d buildRowWindow slow elapsed=%v length=%d", rowIdx, buildElapsed, g.drum.Length)
		}
		r.Steps = nextSteps
		r.CellTypes = nextTypes
		if !g.rowSnapshotMode && rowIdx < len(g.rowStepsScratch) && rowIdx < len(g.rowTypesScratch) {
			// Reuse the previous published slices as scratch for the next refresh.
			g.rowStepsScratch[rowIdx] = oldSteps
			g.rowTypesScratch[rowIdx] = oldTypes
		}

		// When paused, make sure speculative timeline entries track the current
		// predictor so stale seeds from earlier highlights do not drift away
		// from the rendered DrumView and trip parity checks once logging runs.
		if !g.Playing() && freezeLimit < 0 {
			g.reconcileMutableTimeline(rowIdx, windowStart, g.drum.Length, predictAt)
		}

		// Capture timeline vs predictor mismatches so parity can flag stale
		// commits or missed predictor updates (e.g., delete/re-add flows).
		if scanTimeline && (parityFatalEnabled.Load() || g.parityWatch != parityWatchOff || (timelineTrace && rowIdx == timelineTraceRow)) {
			if mismatches := g.diffPredictorTimeline(rowIdx, windowStart, g.drum.Length, freezeLimit); len(mismatches) > 0 {
				indices := make([]int, 0, len(mismatches))
				for _, m := range mismatches {
					indices = append(indices, m.Abs)
					g.parityReport(m)
				}
				if traceRow || !timelineTrace {
					g.logger.Infof("[TIMELINE] mismatches=%d row=%d abs=%v", len(mismatches), rowIdx, indices)
				} else {
					g.logger.Tracef("[TIMELINE] mismatches=%d row=%d abs=%v", len(mismatches), rowIdx, indices)
				}
			}
		}

		// Publish timeline segments for instrumentation/debug. (DrumView caches
		// render strictly from Steps/CellTypes; timeline segments are auxiliary.)
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
			g.logger.Tracef("[TIMELINE/REFRESH] row=%d offset=%d freeze=%d pastMaskCount=%d lastImmutable=%d", rowIdx, windowStart, freezeLimit, pastCount, imm)
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
					// Playback with stationary window: count cell diffs to allow
					// the cheap cell-patch path instead of full row rebuilds.
					diff := countCellDiffs(prevSteps, prevTypes, r.Steps, r.CellTypes)
					if diff > 0 && diff <= rowCachePatchMax {
						g.drum.markRowCellsDirty(rowIdx)
					} else {
						g.drum.markRowDirty(rowIdx)
					}
				} else {
					g.drum.markRowDirty(rowIdx)
				}
			}
			g.rowRenderSig[rowIdx] = sig
		}
		rowElapsed := time.Since(rowStart)
		if rowElapsed > 20*time.Millisecond {
			g.logger.Infof("[REFRESH] row=%d total slow elapsed=%v", rowIdx, rowElapsed)
		}
	}
	// Remember the offset for the next refresh so past cell types can be
	// preserved when the window remains stationary.
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
	g.logger.Tracef("[GAME/REFRESH] refreshDrumRow offset=%d", g.drum.Offset)
	g.parityScan("refresh")
}

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

// countCellDiffs returns the number of cell positions where Steps or CellTypes
// differ between prev and next. Assumes equal-length slices.
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
