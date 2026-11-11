//go:build !js

package ui

import (
	"context"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// desktopWarmBackend renders gated warm items on the audio.prewarm pool.
//
// It uses SubmitBlocking, NOT Submit. Submit is non-blocking by contract and
// DROPS on backpressure — that is how the pre-gate prewarmer lost most of a
// 273-item working set into a 64-deep queue. A gated warm set must complete,
// so the feeder blocks instead. Blocking is safe here: Warm is called from
// warmupController.Begin on the UI goroutine at import time, when no frame
// deadline is being missed (the toast is already up and Play is already
// disabled), and the queue drains at worker speed.
type desktopWarmBackend struct{ pool *async.Pool }

func newDesktopWarmBackend(pool *async.Pool) warmBackend {
	return &desktopWarmBackend{pool: pool}
}

// Cached delegates to the same probe the dispatch path reads, so an item
// already rendered under the current bpm/params/pitch key is never re-rendered.
func (b *desktopWarmBackend) Cached(inst instrument.ID, pitch float64) bool {
	return audio.VoiceCached(inst, pitch)
}

func (b *desktopWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	if b.pool == nil {
		done() // no pool: count it complete so the gate cannot deadlock
		return
	}
	err := b.pool.SubmitBlocking(context.Background(), func(context.Context) {
		audio.PrewarmVoice(inst, pitch)
		done()
	})
	if err != nil {
		done() // closed pool: never leave the gate waiting on a job that cannot run
	}
}

// newWarmBackend selects the platform warm backend. Desktop renders on the
// audio.prewarm pool; browser renders on audio.js's render workers.
func newWarmBackend(g *Game) warmBackend {
	if g != nil && g.voicePrewarmer != nil {
		return newDesktopWarmBackend(g.voicePrewarmer.pool)
	}
	return newDesktopWarmBackend(nil)
}
