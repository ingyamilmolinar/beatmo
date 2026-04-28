package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) bpmLoop() {
	for {
		// Wait for at least one update.
		b, ok := <-g.bpmCh
		if !ok {
			return
		}
		// Drain any pending updates so only the latest BPM is applied.
		for {
			select {
			case nb, ok := <-g.bpmCh:
				if !ok {
					return
				}
				b = nb
				// keep draining
			default:
				start := time.Now()
				g.logger.Debugf("[GAME] applying BPM=%d", b)
				if bpmOwner.Load() != g {
					goto applied
				}
				g.engine.SetBPM(b)
				audio.SetBPM(b)
				g.logger.Debugf("[GAME] applied BPM=%d in %s", b, time.Since(start))
				sendLatest(g.bpmAck, b, nil)
				// If UI has moved on while we were applying BPM (e.g. audio
				// layer was blocked), immediately converge to the current
				// desired DrumView BPM without waiting for another Update().
				// This prevents flakiness where the last requested value was
				// dropped from bpmCh by coalescing.
				for {
					cur := g.drum.BPM()
					if cur == b {
						break
					}
					b = cur
					start = time.Now()
					g.logger.Debugf("[GAME] reconciling BPM=%d", b)
					if bpmOwner.Load() != g {
						break
					}
					g.engine.SetBPM(b)
					audio.SetBPM(b)
					g.logger.Debugf("[GAME] reconciled BPM=%d in %s", b, time.Since(start))
					sendLatest(g.bpmAck, b, nil)
				}
				goto applied
			}
		}
	applied:
		// loop to wait for next update
	}
}
