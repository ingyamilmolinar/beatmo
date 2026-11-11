package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// seqScheduleTime schedules audio for all rows based on wall-clock time,
// independent of UI rendering, at subdivision resolution.
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
	// The audio lane ticks on every pass — including the early returns below —
	// still under seqMu (deferred after the unlock, so it runs first). Cells the
	// UI lane reached before the lane committed them are observed AFTER that
	// tick (defers run LIFO: laneTick, then observePendingLaneCells, then unlock).
	defer g.observePendingLaneCells()
	defer s.laneTick()

	parityEnabled := !runningUnderGoTest() || g.parityWatch.Load() != parityWatchOff

	// Skip entire tick when the audio channel is near-full. The audioLoop
	// goroutine hasn't drained pending events yet; computing more would
	// waste CPU since sendLatest will drop them. The 4ms ticker retries
	// shortly, and no seqNextIdxs are advanced, so no beats are lost.
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
		// A live BPM change re-anchors seqNextIdxs from nextBeatIdxs, which
		// can produce the same catch-up-burst shape as a structural edit
		// (C1 fix). The normal UI BPM path (DrumView.SetBPM ->
		// onStructuralMutation("bpm-change") -> bumpParityGen) already
		// resets peaks at input time; this is a second line of defense for
		// the async bpmLoop applied-BPM re-anchor detected here, which can
		// lag behind that first grace window and fire independently of it.
		g.schedMetrics.ResetPeaks()
	}
	if bpm <= 0 {
		return
	}
	// Compute target absolute subdivision index since playStart.
	// Single clock snapshot for this tick — used for both target and lateness.
	now := time.Now()
	audioNow := audio.Now()
	dtSec, ok := g.state.DtSince(now, audioNow)
	if !ok {
		return
	}
	target, _ := g.state.TargetStep(now, audioNow, bpm, div)
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
	// Schedule up to a small burst per row to catch up to target
	// without introducing audible jitter at high BPM/short segments.
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
		// Per-beat ideal audio time replaces the prior per-burst baseNow
		// (audio.Now() snapped once per row). Using the beat's ideal moment
		// as the schedule base means scheduler lateness (Stage A) does
		// NOT propagate into audio lateness — it just reduces Stage C
		// lead. Without this, a 24ms scheduler stall pushed audio to
		// ideal+64ms instead of ideal+40ms.
		baseNow := -1.0
		if pred != nil && !ensured && g.seqNextIdxs[row] <= target {
			pred.Ensure(ensureHorizon)
			ensured = true
		}
		for g.seqNextIdxs[row] <= target {
			if row >= len(g.drum.Rows) {
				break
			}
			// Burst limit. Originally 8; raised to 32 so a single tick can
			// fully catch up after a goroutine starvation stall of up to
			// ~750ms at 320 BPM. The lower limit forced multi-tick catchup
			// walks that left Stage A P99 stuck at 100-200ms even with the
			// per-frame sync drive in Update().
			if burst >= 32 {
				break
			}
			idx := g.seqNextIdxs[row]
			// Stage A — observe how late the scheduler is firing this
			// beat relative to its ideal wall-clock moment. The ideal
			// moment expressed in dtSec terms is
			//   beatDtSec = (idx/div - baseBeats) * 60 / bpm
			// dtSec is the current elapsed-since-playStart; subtract.
			// Grace-filter the observation itself, not just the downstream
			// check: a live circuit/rule edit or BPM re-anchor can stall
			// scheduling and produce a catch-up burst here whose lateness has
			// nothing to do with steady-state audio health. Without this guard
			// the burst latches into schedMetrics' session-monotonic
			// seqFireMax and can panic the NEXT parityTimingCheck after grace
			// expires (design §3.4: "universal suppression across all
			// stages" — this observation sat outside that spine before the
			// fix).
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
				// Lane mode: the lane committed this mute as a timed StopLaneAt
				// (sparing notes after it); an immediate Stop here would cut
				// voices the lane already scheduled ahead.
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
						// Audio was committed ahead by the audio lane; the UI lane
						// only observes it (parity) and drives the visuals.
						g.observeLaneCell(row, idx)
						g.scheduleSoundUI(row, idx, info, inst, pitch, dur, g.state.AudioTimeOfStep(idx, div, bpm)+RuntimeProf().AudioOffsetSec)
					} else if runningUnderGoTest() && g.playFn != nil && g.scheduleHook == nil {
						g.seqNextIdxs[row]++
						incAtEnd = false
						if layers := g.nodeLayersAt(info); layers != nil {
							for _, l := range layers {
								g.playFn(inst, vol*l.Gain)
							}
						} else {
							g.playFn(inst, vol)
						}
						// The test-only direct-play path bypasses audioLoop; still record
						// a parity audio event so parityScan's audio_missing check reflects
						// what was dispatched, and mark the decision enqueued to keep the
						// Audible⟹Enqueued invariant honest on this path too.
						g.markSeqAudioEnqueued(row, idx)
						g.recordParityAudio(row, idx, audio.Now(), inst, vol, pitch, dur, g.audioGen.Load(), g.parityGen.Load())
					} else {
						// Ideal audio time for this beat: anchored to
						// audioStart + beat's position on the timeline (via the
						// clock authority). Falls back to audio.Now() if no
						// audioStart is available (very early playback, edge case).
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
							// hlCh full: this beat's highlight is dropped and the
							// UI will never paint it. Record the drop so parity
							// exempts it from highlight_vs_audio (cosmetic loss
							// under load, not an audio/UI desync).
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
