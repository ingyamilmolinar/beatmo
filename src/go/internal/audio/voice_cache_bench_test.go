//go:build !test && !js

package audio

import (
	"testing"
)

// Phase-1 baseline benchmark for the pitch-shift cache-miss path. Today every
// _p() drum renderer that does pitch shifting (drums.c:1718, 1823, 1887,
// 2018, 2062) malloc()s a per-call scratch buffer the size of the render
// window. Voice cache hides this on hits, but a miss stalls the audio
// thread on desktop.
//
// Why measure Go-side allocs/op when the leak is C-side? Two reasons:
//   1. After Phase 6 the scratch buffer is owned by post_config / a static
//      scratch, so the C malloc disappears. Go-side allocs/op should remain
//      at the same low number — this bench is the regression guard that the
//      Go wrapper itself stays zero-alloc.
//   2. ns/op measured here is dominated by the C malloc/free (and the
//      resample loop). Post-Phase-6 ns/op should drop noticeably; the
//      delta is the savings the audio thread sees on a cache miss.
//
// Recorded with:
//   go test -tags '' -modfile=go.mod -run=^$ -bench=BenchmarkPitchShift -benchmem ./internal/audio/
// on an i7-12700 the pre-Phase-6 baseline is ~30-50us/op with 0 Go allocs
// (all allocation is C-side via malloc, invisible to Go's runtime).

// renderCowbellPitch renders the cowbell recipe through the modular binding with
// an explicit pitch override. The cowbell POST stage wires pitch (PostOrder=0), so
// a non-zero pitch routes through the SAME shared apply_post_params pitch-scratch
// path this bench guards — render_cowbell_p was deleted in the Phase-6 modular
// migration, so the bench drives the post-resample via the unified engine.
func renderCowbellPitch(buf []float32, sampleRate, samples int, pitch float64) {
	merged := MergeRecipeDefaults("drum-cowbell", RecipeParams{"pitch": pitch})
	spec := cymbalVariantSpecs["drum-cowbell"]
	renderModularP(buf, sampleRate, samples,
		cymbalRecipeToModular("drum-cowbell", merged, spec.variant, cymbalWiredFields("drum-cowbell")))
}

func BenchmarkPitchShiftCacheMissBaseline(b *testing.B) {
	const sampleRate = 44100
	const samples = sampleRate / 4 // 0.25 s — a representative render window
	buf := make([]float32, samples)
	// pitch=5 forces the pitch-scratch malloc path in the shared apply_post_params
	// (the cowbell POST stage resamples). render_cowbell_p was deleted in the
	// Phase-6 modular migration; the modular cowbell binding exercises the same
	// pitch_scratch path this bench guards.

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

// BenchmarkPitchShiftIdentityPath measures the same call with pitch=0 so the
// malloc path is skipped (the if-guard at drums.c:1820 short-circuits). The
// gap between the two benchmarks isolates the malloc + resample cost.
// Post-Phase-6 these two should converge.
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
