package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// seqScheduleTime schedules audio for all rows based on wall-clock time,
// independent of UI rendering, at subdivision resolution.
func (g *Game) seqScheduleTime() {
	if g == nil {
		return
	}
	g.logger.Tracef("[SEQSCHEDULE] entering, about to acquire seqMu")
	if !g.seqMu.TryLock() {
		g.logger.Tracef("[SEQSCHEDULE] seqMu held by Update(); skipping tick")
		return // Update() holds the lock; retry next 1ms tick
	}
	g.logger.Tracef("[SEQSCHEDULE] acquired seqMu")
	defer func() {
		g.logger.Tracef("[SEQSCHEDULE] releasing seqMu")
		g.seqMu.Unlock()
	}()

	parityEnabled := !runningUnderGoTest() || g.parityWatch != parityWatchOff
	if g.drum == nil {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		return
	}
	// Structural row edits (add/delete) can temporarily leave row-indexed state
	// (paths, counters, gates) out of sync while the UI thread reconciles the
	// change. Skip scheduling/parity in that window to avoid false panics from
	// mismatched row mappings.
	rows := len(g.drum.Rows)
	if rows == 0 {
		return
	}
	snap := g.seqPathSnapshot()
	if snap == nil {
		return
	}
	if len(snap.beatInfosByRow) != rows || len(snap.isLoopByRow) != rows || len(snap.loopStartByRow) != rows || len(g.nextBeatIdxs) != rows {
		return
	}
	// Drive scheduling from the applied BPM to keep audio tightly aligned to
	// the engine/audio layer. Fall back to UI BPM only if no applied value is
	// available yet (e.g., just started).
	bpm := g.state.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if g.Playing() && bpm != g.prevBPM && g.prevBPM > 0 {
		audioNow := audio.Now()
		g.state.AnchorForBPMChange(g.prevBPM, time.Now(), audioNow)
		if len(g.seqNextIdxs) != len(g.drum.Rows) {
			g.seqNextIdxs = make([]int, len(g.drum.Rows))
		}
		for row := range g.drum.Rows {
			next := 0
			if row < len(g.nextBeatIdxs) {
				next = g.nextBeatIdxs[row]
			}
			g.seqNextIdxs[row] = next
		}
		g.prevBPM = bpm
	}
	if bpm <= 0 {
		return
	}
	// Compute target absolute subdivision index since playStart.
	var dtSec float64
	audioStart := g.state.AudioStart()
	if n := audio.Now(); n > 0 && audioStart > 0 {
		dtSec = n - audioStart
	} else if playStart := g.state.PlayStart(); !playStart.IsZero() {
		dtSec = time.Since(playStart).Seconds()
	} else {
		return
	}
	baseBeats := g.state.BeatBase()
	target := int(math.Floor((baseBeats + dtSec*float64(bpm)/60.0) * float64(div)))
	// Ensure counters align to row count. Row add/delete operations can change
	// g.drum.Rows length while the background sequencer is running; never reset
	// counters back to zero in that case or we'll "rewind" scheduling and trip
	// parity/highlight logic.
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		old := g.seqNextIdxs
		n := len(g.drum.Rows)
		g.seqNextIdxs = make([]int, n)
		// When counters are uninitialized (common on a fresh start and in tests),
		// begin scheduling from zero so idx=0 can fire immediately. Do not clamp
		// to nextBeatIdxs in this case: updateBeatInfos may prime nextBeatIdxs to
		// offset+1 on path changes even before playback advances.
		//
		// When rows are added/removed mid-playback, keep the default baseline at
		// target+1 to avoid "rewinding" and re-scheduling past beats.
		if len(old) == 0 {
			for row := 0; row < n; row++ {
				g.seqNextIdxs[row] = 0
			}
		} else {
			baseline := target + 1
			if baseline < 0 {
				baseline = 0
			}
			for row := 0; row < n; row++ {
				next := baseline
				if row < len(g.nextBeatIdxs) && g.nextBeatIdxs[row] > next {
					next = g.nextBeatIdxs[row]
				}
				if row < len(old) && old[row] > next {
					next = old[row]
				}
				g.seqNextIdxs[row] = next
			}
		}
	}
	g.ensureGateSlices()
	pred := g.engine.Predictor
	ensureHorizon := target + 1
	if ensureHorizon < 0 {
		ensureHorizon = 0
	}
	ensured := false
	// Schedule up to a small burst per row to catch up to target without
	// introducing audible jitter at high BPM/short segments.
	for row := range g.drum.Rows {
		if row >= len(g.drum.Rows) {
			break
		}
		if row >= len(g.seqNextIdxs) {
			g.ensureGateSlices()
			if row >= len(g.seqNextIdxs) {
				break
			}
		}
		// If the UI playhead has advanced past the sequencer counters (e.g.,
		// syncUIToTime re-anchored while the sequencer fell behind), do not
		// attempt to "catch up" by scheduling already-past beats. Doing so
		// produces a burst of late audio that parity intentionally ignores
		// (beats older than the current playhead).
		if g.Playing() && row < len(g.nextBeatIdxs) {
			floor := g.nextBeatIdxs[row] - 1 // current beat
			if floor < 0 {
				floor = 0
			}
			if target >= 0 && floor > target {
				// nextBeatIdxs may be primed by offset/path bookkeeping; never
				// clamp past the wall-clock target or we'll starve scheduling.
				floor = target
			}
			if g.seqNextIdxs[row] < floor {
				g.seqNextIdxs[row] = floor
			}
		}
		burst := 0
		baseNow := -1.0
		if pred != nil && !ensured && g.seqNextIdxs[row] <= target {
			pred.Ensure(ensureHorizon)
			ensured = true
		}
		for g.seqNextIdxs[row] <= target {
			if row >= len(g.drum.Rows) {
				break
			}
			if burst >= 8 {
				break
			}
			idx := g.seqNextIdxs[row]
			info := seqBeatInfoAtRow(snap, row, idx)
			inst := g.drum.Rows[row].Instrument
			missing := (row >= 0 && row < len(g.drum.Rows) && !g.drum.IsInstrumentAvailable(inst) && g.playFn == nil && g.scheduleHook == nil)
			expected := g.parityExpected(row, idx, info)
			if info.NodeType == model.NodeTypeMute {
				trigger := expected
				g.recordSeqDecision(row, idx, trigger, info.NodeType, missing)
				if !trigger {
					g.setLastTriggered(row, info.NodeID, false)
					if g.scheduleHook != nil {
						g.scheduleHook(row, idx)
					}
					g.seqNextIdxs[row] = idx + 1
					burst++
					if parityEnabled {
						g.parityCheck(row, idx, info, trigger, "time", missing)
					}
					continue
				}
				if row < len(g.muteUntilByRow) && idx+1 > g.muteUntilByRow[row] {
					g.muteUntilByRow[row] = idx + 1
				}
				audio.Stop(inst)
				if g.scheduleHook != nil {
					g.scheduleHook(row, idx)
				}
				if row >= len(g.lastFiredNodeByRow) {
					g.lastFiredNodeByRow = make([]model.NodeID, len(g.drum.Rows))
				}
				g.lastFiredNodeByRow[row] = info.NodeID
				g.setLastTriggered(row, info.NodeID, true)
				if g.rowIsAudible(row) {
					g.nodeAnimSet(info.NodeID, 1)
				} else {
					g.nodeAnimSet(info.NodeID, 0)
				}
				if g.timingTestMode && g.highlightHook != nil {
					g.highlightHook(row, idx)
				} else {
					select {
					case g.hlCh <- struct {
						row, idx int
						info     model.BeatInfo
					}{row: row, idx: idx, info: info}:
					default:
					}
				}
				g.seqNextIdxs[row] = idx + 1
				burst++
				if parityEnabled {
					g.parityCheck(row, idx, info, trigger, "time", missing)
				}
				continue
			}
			audible := expected
			var logicDecision model.NodeDecision
			logicApplied := false
			gateActive := row < len(g.muteUntilByRow) && idx < g.muteUntilByRow[row]
			if !gateActive {
				if node, ok := g.nodeSnapshot(info.NodeID); ok && node.Params.Logic != nil {
					if g.nodeTriggerCountsByRow == nil {
						g.nodeTriggerCountsByRow = make(map[int]map[model.NodeID]int)
					}
					counts := g.nodeTriggerCountsByRow[row]
					if counts == nil {
						counts = make(map[model.NodeID]int)
						g.nodeTriggerCountsByRow[row] = counts
					}
					if g.seqShouldTriggerNode(row, idx, info, node, counts, snap) {
						count := g.incrementLogicTriggerCount(row, info.NodeID)
						logicDecision = node.Params.Logic(model.NodeContext{
							NodeID:        info.NodeID,
							Row:           row,
							AbsoluteIndex: idx,
							TriggerCount:  count,
						})
						logicApplied = true
					}
				}
			}
			if logicApplied && logicDecision.Enabled != nil && !*logicDecision.Enabled {
				audible = false
			}
			g.recordSeqDecision(row, idx, audible, info.NodeType, missing)
			incAtEnd := true
			if !missing && audible {
				anySolo := false
				for _, r := range g.drum.Rows {
					if r.Solo {
						anySolo = true
						break
					}
				}
				if g.drum.Rows[row].Muted || (anySolo && !g.drum.Rows[row].Solo) {
					g.recordSeqDecision(row, idx, false, info.NodeType, missing)
					g.seqNextIdxs[row] = idx + 1
					burst++
					if parityEnabled {
						g.parityCheck(row, idx, info, false, "time", missing)
					}
					continue
				}
				if info.NodeType == model.NodeTypeRegular {
					if row < len(g.muteUntilByRow) && idx < g.muteUntilByRow[row] {
						g.recordSeqDecision(row, idx, false, info.NodeType, missing)
						g.setLastTriggered(row, info.NodeID, false)
						if g.scheduleHook != nil {
							g.scheduleHook(row, idx)
						}
						g.seqNextIdxs[row] = idx + 1
						burst++
						if parityEnabled {
							g.parityCheck(row, idx, info, false, "time", missing)
						}
						continue
					}
					vol, pitch, dur := g.evalNodeParamsOnly(row, idx, info)
					if logicApplied && audible {
						if logicDecision.Enabled == nil || *logicDecision.Enabled {
							vol, pitch, dur = applyNodeDecision(vol, pitch, dur, logicDecision)
						}
					}
					if runningUnderGoTest() && g.playFn != nil && g.scheduleHook == nil {
						g.seqNextIdxs[row]++
						incAtEnd = false
						g.playFn(inst, vol)
						// The test-only direct-play path bypasses audioLoop; still record
						// a parity audio event so parityScan's audio_missing check reflects
						// what was dispatched.
						g.recordParityAudio(row, idx, audio.Now(), inst, vol, pitch, dur, g.audioGen.Load())
					} else {
						if baseNow < 0 {
							baseNow = audio.Now()
						}
						g.scheduleSound(row, idx, info, inst, vol, pitch, dur, baseNow, true)
					}
					if g.scheduleHook != nil {
						g.scheduleHook(row, idx)
					}
					if row >= len(g.lastFiredNodeByRow) {
						g.lastFiredNodeByRow = make([]model.NodeID, len(g.drum.Rows))
					}
					g.lastFiredNodeByRow[row] = info.NodeID
					if g.timingTestMode && g.highlightHook != nil {
						g.highlightHook(row, idx)
					} else {
						select {
						case g.hlCh <- struct {
							row, idx int
							info     model.BeatInfo
						}{row: row, idx: idx, info: info}:
						default:
						}
					}
				} else {
					g.setLastTriggered(row, info.NodeID, false)
				}
			} else {
				g.setLastTriggered(row, info.NodeID, false)
			}
			if incAtEnd {
				g.seqNextIdxs[row] = idx + 1
			}
			burst++
			if parityEnabled {
				g.parityCheck(row, idx, info, audible, "time", missing)
			}
		}
	}
}
