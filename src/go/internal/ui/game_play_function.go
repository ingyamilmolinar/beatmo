package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// SetPlayFunc overrides the audio playback function used by this game instance.
func (g *Game) SetPlayFunc(fn func(string, float64, ...float64)) {
	// Wrap the provided function to emulate scheduling when a future timestamp
	// is provided via 'when'. This makes timing tests (real Ebiten/audio path)
	// observe the intended delay without requiring the actual audio backend.
	g.playFn = func(id string, vol float64, when ...float64) {
		if g.Paused() {
			return
		}
		// If a future time is provided, delay invocation until then.
		if len(when) > 0 {
			due := when[0] - audio.Now()
			// Use a tiny epsilon to avoid injecting measurable latency when
			// the target time is effectively now.
			const eps = 0.0001 // 100µs
			if due > eps {
				d := time.Duration(due * float64(time.Second))
				go func() {
					timer := time.NewTimer(d)
					defer timer.Stop()
					<-timer.C
					if g.Paused() {
						return
					}
					fn(id, vol, when...)
				}()
				return
			}
		}
		gain := audio.ChannelVolume(id) * audio.MainVolume()
		fn(id, vol*gain, when...)
	}
}
