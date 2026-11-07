package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (s *playbackSlate) seqScheduleTime() {
	g := s.g
	if g == nil {
		return
	}
	g.logger.Tracef("[seqschedule] entering, about to acquire seqMu")
	if !g.seqMu.TryLock() {
		g.logger.Tracef("[seqschedule] seqMu held by Update(); skipping tick")
		return // Update() holds the lock; retry next 1ms tick
	}
	g.logger.Tracef("[seqschedule] acquired seqMu")
	defer func() {
		g.logger.Tracef("[seqschedule] releasing seqMu")
		g.seqMu.Unlock()
	}()
	// The lane ticks every pass, early returns included, still under seqMu; defers run LIFO, so
	// cells the UI lane reached first are observed after laneTick.
	defer g.observePendingLaneCells()
	defer s.laneTick()

	parityEnabled := !runningUnderGoTest() || g.parityWatch != parityWatchOff

	// Skip the whole tick while audioCh is near full: nothing advances, so no beat is lost.
	if g.audioChNearFull() {
		return
	}

	if g.drum == nil {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		return
	}
	// Mid row edit, row-indexed state can be out of sync with the rows; skip to avoid false panics.
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
	// The applied BPM keeps audio aligned with the engine; the UI value only until one exists.
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
		// A BPM re-anchor from the async bpmLoop can lag the input-time reset, so reset peaks here too.
		g.schedMetrics.ResetPeaks()
	}
	if bpm <= 0 {
		return
	}
	// One clock sample per tick, for both target and lateness.
	now := time.Now()
	audioNow := audio.Now()
	dtSec, ok := g.state.DtSince(now, audioNow)
	if !ok {
		return
	}
	target, _ := g.state.TargetStep(now, audioNow, bpm, div)
	// Rows can change under the running sequencer; never reset counters to zero, which would rewind.
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		old := g.seqNextIdxs
		n := len(g.drum.Rows)
		g.seqNextIdxs = make([]int, n)
		// Uninitialized counters start at zero, so idx 0 fires at once (not clamped to nextBeatIdxs,
		// which path changes may prime ahead); rows added mid-play start at target+1.
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
	// A small per-row burst catches up without audible jitter.
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
		// Never catch up by scheduling beats the UI playhead already passed: a burst of late audio.
		if g.Playing() && row < len(g.nextBeatIdxs) {
			floor := g.nextBeatIdxs[row] - 1 // current beat
			if floor < 0 {
				floor = 0
			}
			if target >= 0 && floor > target {
				// Never past the wall-clock target, or scheduling starves.
				floor = target
			}
			if g.seqNextIdxs[row] < floor {
				g.seqNextIdxs[row] = floor
			}
		}
		burst := 0
		// Each beat's ideal time is the schedule base, so scheduler lateness only shrinks the lead and
		// never reaches the audio.
		baseNow := -1.0
		if pred != nil && !ensured && g.seqNextIdxs[row] <= target {
			pred.Ensure(ensureHorizon)
			ensured = true
		}
		for g.seqNextIdxs[row] <= target {
			if row >= len(g.drum.Rows) {
				break
			}
			// 32 lets one tick fully catch up after a stall of ~750 ms at 320 BPM.
			if burst >= 32 {
				break
			}
			idx := g.seqNextIdxs[row]
			// Grace-filtered: an edit or re-anchor can stall scheduling, and the catch-up burst would latch
			// into the session-monotonic seqFireMax and fail the next check after grace.
			if g.Playing() && !g.parityInGrace() {
				beatDtSec := g.state.DtOfStep(idx, div, bpm)
				if late := dtSec - beatDtSec; late >= 0 {
					g.schedMetrics.ObserveSeqFireLate(late)
				}
			}
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
				// The lane committed this mute as a timed stop; stopping now would cut voices it scheduled ahead.
				if !g.audioLaneOn() {
					audio.Stop(inst)
				}
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
				if info.NodeType.Fires() {
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
					if g.audioLaneOn() {
						// The lane already committed the audio; the UI lane only observes and drives the visuals.
						g.observeLaneCell(row, idx)
						g.scheduleSoundUI(row, idx, info, inst, pitch, dur, g.state.AudioTimeOfStep(idx, div, bpm)+RuntimeProf().AudioOffsetSec)
					} else if runningUnderGoTest() && g.playFn != nil && g.scheduleHook == nil {
						g.seqNextIdxs[row]++
						incAtEnd = false
						if layers := g.playbackSlate.nodeLayersAt(info); layers != nil {
							for _, l := range layers {
								g.playFn(inst, vol*l.Gain)
							}
						} else {
							g.playFn(inst, vol)
						}
						// The test-only direct path skips audioLoop, so record the event and keep Audible ⟹ Enqueued.
						g.markSeqAudioEnqueued(row, idx)
						g.recordParityAudio(row, idx, audio.Now(), inst, vol, pitch, dur, g.audioGen.Load(), g.parityGen.Load())
					} else {
						// audio.Now() until an audio start is known.
						beatIdealAudio := g.state.AudioTimeOfStep(idx, div, bpm)
						if !(beatIdealAudio > 0) {
							if baseNow < 0 {
								baseNow = audio.Now()
							}
							beatIdealAudio = baseNow
						}
						g.scheduleSound(row, idx, info, inst, vol, pitch, dur, beatIdealAudio, true)
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
							// Parity exempts a highlight the full hlCh dropped.
							g.markHighlightDropped(row, idx)
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

func (g *Game) seqScheduleTime() { g.playbackSlate.seqScheduleTime() }
