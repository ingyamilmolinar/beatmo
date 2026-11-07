//go:build !js

package ui

import (
	"context"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// SubmitBlocking, NOT Submit: Submit drops on backpressure and lost most of a large warm set. Blocking is safe:
// Begin runs at import time with the toast up and Play disabled.
type desktopWarmBackend struct{ pool *async.Pool }

func newDesktopWarmBackend(pool *async.Pool) warmBackend {
	return &desktopWarmBackend{pool: pool}
}

// The dispatch path's own probe, so an item already cached under the current key is never re-rendered.
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

func newWarmBackend(g *Game) warmBackend {
	if g != nil && g.voicePrewarmer != nil {
		return newDesktopWarmBackend(g.voicePrewarmer.pool)
	}
	return newDesktopWarmBackend(nil)
}
