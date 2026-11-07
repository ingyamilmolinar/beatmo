//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A pitch-shifting miss renders through the shared post pitch-scratch path; this keeps the Go wrapper zero-alloc and
// measures what the audio thread pays on a miss.

// A FINISH-stage pitch routes through the shared pitch-scratch path.
func renderCowbellPitch(buf []float32, sampleRate, samples int, pitch float64) {
	if out, err := RenderInstrumentWithFields(roster.Cowbell, sampleRate, samples, map[string]float64{"post_pitch": pitch}); err == nil {
		copy(buf, out)
	}
}

func BenchmarkPitchShiftCacheMissBaseline(b *testing.B) {
	const sampleRate = 44100
	const samples = sampleRate / 4 // 0.25 s — a representative render window
	buf := make([]float32, samples)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Re-zero so each iteration starts from the same input state.
		for j := range buf {
			buf[j] = 0
		}
		renderCowbellPitch(buf, sampleRate, samples, 5)
	}
}

// pitch 0 skips the scratch path, so the gap isolates its cost.
func BenchmarkPitchShiftIdentityPath(b *testing.B) {
	const sampleRate = 44100
	const samples = sampleRate / 4
	buf := make([]float32, samples)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range buf {
			buf[j] = 0
		}
		renderCowbellPitch(buf, sampleRate, samples, 0) // pitch=0 skips the malloc path
	}
}
