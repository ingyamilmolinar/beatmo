package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) audioLoop() {
	for first := range g.audioCh {
		if g.Paused() {
			g.logger.Debugf("[AUDIO] drop id=%s vol=%.3f while paused", first.id, first.vol)
			continue
		}

		// Gather a small batch to reduce Go→JS crossings on WASM.
		maxBatch := RuntimeProf().AudioBatchMax
		reqCap := 32
		if reqCap > maxBatch {
			reqCap = maxBatch
		}
		reqs := make([]soundReq, 0, reqCap)
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
					g.logger.Debugf("[AUDIO] drop id=%s vol=%.3f while paused", req.id, req.vol)
					continue
				}
				reqs = append(reqs, req)
			default:
				drain = false
			}
		}

		batch := make([]audio.BatchParam, 0, len(reqs))
		qlatSum := time.Duration(0)
		audioNow := audio.Now()
		logged := false
		for _, req := range reqs {
			// Drop sequencer-scheduled events from a previous transport generation
			// (e.g. Stop -> replay) so stale audio does not desync parity/highlights.
			if req.row >= 0 {
				curGen := g.audioGen.Load()
				if req.gen != curGen {
					g.logger.Debugf("[AUDIO] drop stale scheduled id=%s row=%d abs=%d gen=%d curGen=%d", req.id, req.row, req.abs, req.gen, curGen)
					continue
				}
				if !g.Playing() {
					g.logger.Debugf("[AUDIO] drop scheduled id=%s row=%d abs=%d while stopped", req.id, req.row, req.abs)
					continue
				}
			}
			if !logged {
				logged = true
				g.logger.Debugf("[AUDIO] dispatch id=%s vol=%.3f pitch=%.3f dur=%.3f when=%v", req.id, req.vol, req.pitch, req.dur, req.when)
			}
			qlat := time.Since(req.enqAt)
			if req.hasWhen && audioNow > 0 {
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
	}
}
