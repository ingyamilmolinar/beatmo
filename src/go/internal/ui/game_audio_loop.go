package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) audioLoop() {
	defer g.bgWG.Done()
	for {
		// Terminate on Close() via audioQuit. audioCh is never closed (this
		// loop writes back into it through the opportunistic seqScheduleTime
		// drive below), so closing it would panic that send.
		select {
		case <-g.audioQuit:
			return
		default:
		}
		var first soundReq
		select {
		case <-g.audioQuit:
			return
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

		// Gather a small batch to reduce Go→JS crossings on WASM.
		// Reuse scratch buffers across iterations so steady-state
		// playback does not allocate ~3 KB of soundReq + a BatchParam
		// slice per drain cycle. On WASM the peak working set between
		// GC cycles drives committed linear memory; this churn was a
		// contributor to long-session OOMs (see plan
		// hey-please-debug-and-cached-aurora.md Phase 4).
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
		// Persist any grow that append may have done so the next
		// iteration sees the larger capacity.
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
			// Drop sequencer-scheduled events from a previous transport generation
			// (e.g. Stop -> replay) so stale audio does not desync parity/highlights.
			if req.row >= 0 {
				curGen := g.audioGen.Load()
				if req.gen != curGen {
					g.logger.Debugf("[audio] drop stale scheduled id=%s row=%d abs=%d gen=%d curGen=%d", req.id, req.row, req.abs, req.gen, curGen)
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
			// Stage B observation: raw enqueue→dispatch latency, before the
			// lookahead subtraction that follows. The subtraction below
			// (qlat -= leadDur) is for the AudioQLatAvg metric, which folds
			// in the scheduling cushion; the 3-stage view wants the raw
			// goroutine handoff latency uncontaminated by lookahead.
			g.schedMetrics.ObserveBridge(qlat.Seconds())
			// Dispatch-time floor: never let a sequencer-scheduled req.when
			// slip into the past or the small-lead danger zone. This is the
			// last safety net before the audio buffer is scheduled; clamping
			// here is the only place where audio.Now() is the same clock the
			// audio engine reads on .start(). It exists ONLY to survive the
			// Go→JS PlayBatch crossing on WASM (MinDispatchLeadSec > 0, ~12ms:
			// the Go-side ~8ms cushion plus the ~4ms bridge per-event drift)
			// so the JS observer's minLead stays positive after the bridge
			// delay. It is disabled on desktop (MinDispatchLeadSec == 0),
			// where there is no bridge and groove "rush" can legitimately
			// schedule before the grid boundary. It applies only to
			// sequencer-scheduled events (row >= 0): fire-and-forget UI sounds
			// (row < 0, queued via queueSoundParams) are enqueued with
			// when=audio.Now() ("play now") and must dispatch immediately, not
			// be bumped into the future.
			minDispatchLead := RuntimeProf().MinDispatchLeadSec
			if req.hasWhen && audioNow > 0 {
				if minDispatchLead > 0 && req.row >= 0 {
					if floor := audioNow + minDispatchLead; req.when < floor {
						req.when = floor
					}
				}
				g.schedMetrics.Observe(audioNow, req.when)
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
				g.recordParityAudio(req.row, req.abs, req.when, req.id, req.vol, req.pitch, req.dur, req.gen)
			} else {
				g.recordParityAudio(req.row, req.abs, audio.Now(), req.id, req.vol, req.pitch, req.dur, req.gen)
			}

			b := audio.BatchParam{ID: req.id, Vol: req.vol, Pitch: req.pitch, Dur: req.dur}
			if req.hasWhen {
				b.HasWhen = true
				b.When = req.when
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
		audio.PlayBatch(batch)
		callDur := time.Since(t0)
		if n := len(batch); n > 1 {
			callDur /= time.Duration(n)
		}
		g.perf.onAudioDeq(avgQLat, callDur)
		// Opportunistic sequencer drive: PlayBatch yields to JS, which is a
		// natural goroutine scheduling point. Pulling seqScheduleTime here
		// guarantees a fire whenever the audio pipeline is moving, even if
		// the UI frame-end drive hasn't run for the last few ms. This
		// supplements the background sequencerLoop goroutine, so it is gated
		// to production: under go test that goroutine is never started and
		// scheduling is driven deterministically from Update() (game_new.go) —
		// driving it here too would race the UI's highlight advance and write
		// seqNextIdxs concurrently. closing stops the drive during teardown so
		// audioCh drains and the audioQuit select wins.
		if g.Playing() && !g.closing.Load() && !runningUnderGoTest() {
			g.seqScheduleTime()
		}
	}
}
