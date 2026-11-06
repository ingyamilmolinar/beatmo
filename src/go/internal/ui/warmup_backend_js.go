//go:build js

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// browserWarmBackend renders gated warm items through the JS render worker
// pool. Unlike desktop there is no Go-side pool: js/wasm is single-threaded, so
// parallelism lives entirely in audio.js's render workers.
type browserWarmBackend struct{}

func newBrowserWarmBackend() warmBackend { return browserWarmBackend{} }

// Cached delegates to the same probe the dispatch path reads, so an item
// already rendered under the current bpm/params/pitch key is never re-rendered.
func (browserWarmBackend) Cached(inst instrument.ID, pitch float64) bool {
	return audio.VoiceCached(inst, pitch)
}

// Warm renders one item and reports completion via WarmVoiceItem, which
// guarantees done() fires on every path (see voice_prewarm_js.go).
func (browserWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	audio.WarmVoiceItem(inst, pitch, done)
}

// newWarmBackend selects the platform warm backend. Desktop renders on the
// audio.prewarm pool; browser renders on audio.js's render workers.
func newWarmBackend(g *Game) warmBackend { return newBrowserWarmBackend() }
