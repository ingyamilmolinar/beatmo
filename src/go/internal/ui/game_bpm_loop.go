package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) bpmLoop() {
	defer g.bgWG.Done()
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
				g.logger.Debugf("[game] applying BPM=%d", b)
				if bpmOwner.Load() != g {
					goto applied
				}
				g.engine.SetBPM(b)
				audio.SetBPM(b)
				g.logger.Debugf("[game] applied BPM=%d in %s", b, time.Since(start))
				sendLatest(g.bpmAck, b, nil)
				// Converge to the current UI BPM now: coalescing may have dropped the last request from bpmCh.
				for {
					cur := g.drum.BPM()
					if cur == b {
						break
					}
					b = cur
					start = time.Now()
					g.logger.Debugf("[game] reconciling BPM=%d", b)
					if bpmOwner.Load() != g {
						break
					}
					g.engine.SetBPM(b)
					audio.SetBPM(b)
					g.logger.Debugf("[game] reconciled BPM=%d in %s", b, time.Since(start))
					sendLatest(g.bpmAck, b, nil)
				}
				goto applied
			}
		}
	applied:
		// loop to wait for next update
	}
}
