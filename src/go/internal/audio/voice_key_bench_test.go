//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Per-trigger cost of hashing the whole block, which table cells tripled.
func BenchmarkLiveVoiceKeyPitched(b *testing.B) {
	id := roster.Marimba
	if _, ok := EnsureInstrumentDoc(id); !ok {
		b.Skip("no marimba document")
	}
	for i := 0; i < b.N; i++ {
		liveVoiceKey(id, 48000, float64(i%24), true, false)
	}
}
