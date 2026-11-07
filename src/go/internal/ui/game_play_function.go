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
	// Emulates a future 'when', so timing tests see the delay without a real audio backend.
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {
		if g.Paused() {
			return
		}
		// If a future time is provided, delay invocation until then.
		if len(when) > 0 {
			due := when[0] - audio.Now()
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

// Falls back to immediate dispatch without a scheduler (or once closed), so fn is always called.
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
