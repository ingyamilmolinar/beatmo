//go:build js

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// js/wasm is single-threaded, so the parallelism lives in audio.js's render workers.
type browserWarmBackend struct{}

func newBrowserWarmBackend() warmBackend { return browserWarmBackend{} }

// The dispatch path's own probe, so an item already cached under the current key is never re-rendered.
func (browserWarmBackend) Cached(inst instrument.ID, pitch float64) bool {
	return audio.VoiceCached(inst, pitch)
}

// WarmVoiceItem guarantees done() fires on every path.
func (browserWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	audio.WarmVoiceItem(inst, pitch, done)
}

func newWarmBackend(g *Game) warmBackend { return newBrowserWarmBackend() }
