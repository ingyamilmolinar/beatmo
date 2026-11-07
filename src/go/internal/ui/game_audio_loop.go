package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) audioLoop() {
	defer g.bgWG.Done()
	for {
		// audioCh is never closed: this loop sends into it through the seqScheduleTime drive below.
		select {
		case <-g.audioQuit:
			return
		default:
		}
		var first soundReq
		select {
		case <-g.audioQuit:
			return
		case <-g.laneWake:
			// Audio-lane ops (schedule / cancel / stop) recorded by the lane.
			g.flushLaneOps()
			continue
		case req, ok := <-g.audioCh:
			if !ok {
				return
			}
			first = req
		}
		if g.Paused() {
			g.logger.Debugf("[audio] drop id=%s vol=%.3f while paused", first.id, first.vol)
			continue
		}

		// Batched to cut Go→JS crossings; scratch buffers are reused because on WASM per-drain
		// allocations grow committed linear memory.
		maxBatch := RuntimeProf().AudioBatchMax
		reqCap := 32
		if reqCap > maxBatch {
			reqCap = maxBatch
		}
		if cap(g.audioLoopReqs) < reqCap {
			g.audioLoopReqs = make([]soundReq, 0, reqCap)
		} else {
			g.audioLoopReqs = g.audioLoopReqs[:0]
		}
		reqs := g.audioLoopReqs
		reqs = append(reqs, first)

		drain := true
		for drain && len(reqs) < maxBatch {
			select {
			case req, ok := <-g.audioCh:
				if !ok {
					drain = false
					continue
				}
				if g.Paused() {
					g.logger.Debugf("[audio] drop id=%s vol=%.3f while paused", req.id, req.vol)
					continue
				}
				reqs = append(reqs, req)
			default:
				drain = false
			}
		}
		g.audioLoopReqs = reqs

		if cap(g.audioLoopBatch) < len(reqs) {
			g.audioLoopBatch = make([]audio.BatchParam, 0, len(reqs))
		} else {
			g.audioLoopBatch = g.audioLoopBatch[:0]
		}
		batch := g.audioLoopBatch
		qlatSum := time.Duration(0)
		audioNow := audio.Now()
		logged := false
		for _, req := range reqs {
			// Drop events from a previous transport generation so stale audio cannot desync parity.
			if req.row >= 0 {
				curGen := g.audioGen.Load()
				if req.gen != curGen {
					g.logger.Debugf("[audio] drop stale scheduled id=%s row=%d abs=%d gen=%d curGen=%d", req.id, req.row, req.abs, req.gen, curGen)
					continue
				}
				// Only the edited row's stale beats are dropped (and rescheduled); other rows keep theirs, so a
				// placement elsewhere leaves no gap.
				if curRowGen := g.rowStructGenLoad(req.row); req.rowGen != curRowGen {
					g.logger.Debugf("[audio] drop stale-row scheduled id=%s row=%d abs=%d rowGen=%d curRowGen=%d", req.id, req.row, req.abs, req.rowGen, curRowGen)
					continue
				}
				if !g.Playing() {
					g.logger.Debugf("[audio] drop scheduled id=%s row=%d abs=%d while stopped", req.id, req.row, req.abs)
					continue
				}
			}
			if !logged {
				logged = true
				g.logger.Debugf("[audio] dispatch id=%s vol=%.3f pitch=%.3f dur=%.3f when=%v", req.id, req.vol, req.pitch, req.dur, req.when)
			}
			qlat := time.Since(req.enqAt)
			// Raw handoff latency, before the lookahead subtraction. Grace-filtered here too: an edit stall
			// would otherwise latch into the session-monotonic bridgeMax and fail the next check.
			if !g.parityInGrace() {
				g.schedMetrics.ObserveBridge(qlat.Seconds())
			}
			// The last safety net before scheduling, on the clock .start() reads. WASM only (desktop has no
			// bridge, and groove rush may schedule early), and only for sequencer events; UI sounds play now.
			minDispatchLead := RuntimeProf().MinDispatchLeadSec
			// Observed BEFORE the floor clamp: after it, a late note read as early and the lateness gates
			// could never fire.
			requested := req.when
			if req.hasWhen && audioNow > 0 {
				if minDispatchLead > 0 && req.row >= 0 {
					if floor := audioNow + minDispatchLead; req.when < floor {
						req.when = floor
					}
				}
				// Only Observe is grace-filtered (an edit stall would latch maxLag); lead/lag below always runs.
				if !g.parityInGrace() {
					g.schedMetrics.Observe(audioNow, requested)
				}
				if lead := req.when - audioNow; lead > 0 {
					leadDur := time.Duration(lead * float64(time.Second))
					if qlat > leadDur {
						qlat -= leadDur
					} else {
						qlat = 0
					}
				}
			}
			qlatSum += qlat

			// Record parity only for audio that actually passes the final dispatch boundary.
			if req.hasWhen {
				g.recordParityAudio(req.row, req.abs, req.when, req.id, req.vol, req.pitch, req.dur, req.gen, req.parityGen)
			} else {
				g.recordParityAudio(req.row, req.abs, audio.Now(), req.id, req.vol, req.pitch, req.dur, req.gen, req.parityGen)
			}

			b := audio.BatchParam{ID: req.id, Vol: req.vol, Pitch: req.pitch, Dur: req.dur, Row: req.row, Abs: req.abs, Grid: req.grid}
			if req.hasWhen {
				b.HasWhen = true
				b.When = req.when
				b.Ideal = requested
			}
			batch = append(batch, b)
		}
		if len(batch) == 0 {
			continue
		}

		if g.playFn != nil {
			// Test override path: dispatch individually to preserve expected behavior.
			for _, r := range batch {
				if r.HasWhen {
					g.playFn(r.ID, r.Vol, r.When)
				} else {
					g.playFn(r.ID, r.Vol)
				}
			}
			continue
		}

		// Default path: batch-dispatch to audio engine (WASM uses JS batch).
		avgQLat := time.Duration(int64(qlatSum) / int64(len(batch)))
		t0 := time.Now()
		g.flushLaneOps() // keep lane ops ordered ahead of this batch
		audio.PlayBatch(batch)
		g.drainEmissionReports()
		callDur := time.Since(t0)
		if n := len(batch); n > 1 {
			callDur /= time.Duration(n)
		}
		g.perf.onAudioDeq(avgQLat, callDur)
		// PlayBatch yields to JS, so drive the sequencer here too while audio moves. Production only:
		// under go test scheduling is deterministic from Update, and this would race it.
		if g.Playing() && !g.closing.Load() && !runningUnderGoTest() {
			g.seqScheduleTime()
		}
	}
}
