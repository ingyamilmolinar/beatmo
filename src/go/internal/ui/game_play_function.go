package ui

import (
	"context"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// SetPlayFunc overrides the audio playback function used by this game instance.
func (g *Game) SetPlayFunc(fn func(instrument.ID, float64, ...float64)) {
	// Wrap the provided function to emulate scheduling when a future timestamp
	// is provided via 'when'. This makes timing tests (real Ebiten/audio path)
	// observe the intended delay without requiring the actual audio backend.
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {
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
				g.scheduleDelayedPlay(due, fn, id, vol, when)
				return
			}
		}
		gain := audio.ChannelVolume(channel.Of(id)) * audio.MainVolume()
		fn(id, vol*gain, when...)
	}
}

// scheduleDelayedPlay submits fn to fire after due seconds via the
// bounded audioScheduler. Falls back to immediate dispatch if no
// scheduler is wired (Game constructed outside New()) or if the
// scheduler is closed during shutdown — preserves the test contract
// that fn is eventually called.
func (g *Game) scheduleDelayedPlay(due float64, fn func(instrument.ID, float64, ...float64), id instrument.ID, vol float64, when []float64) {
	deliver := func() {
		if g.Paused() {
			return
		}
		fn(id, vol, when...)
	}
	if g.audioScheduler == nil {
		deliver()
		return
	}
	fireAt := time.Now().Add(time.Duration(due * float64(time.Second)))
	if _, err := g.audioScheduler.Schedule(fireAt, func(_ context.Context) { deliver() }); err != nil {
		deliver()
	}
}
