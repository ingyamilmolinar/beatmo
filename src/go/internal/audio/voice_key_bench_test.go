//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// BenchmarkLiveVoiceKeyPitched: the per-trigger cost of a melodic voice's
// cache key (BlockHashWith over the whole block) — the dispatch hot path the
// table cells tripled (439 → 1321 doubles).
func BenchmarkLiveVoiceKeyPitched(b *testing.B) {
	id := roster.Marimba
	if _, ok := EnsureInstrumentDoc(id); !ok {
		b.Skip("no marimba document")
	}
	for i := 0; i < b.N; i++ {
		liveVoiceKey(id, 48000, float64(i%24), true, false)
	}
}
