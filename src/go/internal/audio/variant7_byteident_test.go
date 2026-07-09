//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestVariant7RefactorByteIdentity is the safety net for the DSP-primitive
// refactor: the acoustic kick (variant 7) was rewritten to COMPOSE the reusable
// primitives in src/c/synth_dsp_primitives.h (sp_onepole / sp_osc /
// sp_pitch_glide / sp_reverb3) instead of open-coding the same state machines
// inline. Because each primitive is a `static inline` that reproduces the exact
// float-op order, the render must be BYTE-IDENTICAL. This pins a checksum of the
// acoustic-kick render captured from the pre-refactor code; if a future change to
// the primitives (or to variant 7) alters a single sample it fails here.
//
// If you INTENTIONALLY change the variant-7 DSP, re-capture and update the
// anchors below (render acousticKickSeed at 44100 Hz for 0.6 s and recompute).
func TestVariant7RefactorByteIdentity(t *testing.T) {
	const (
		sr          = 44100
		wantSamples = 26460
		wantSum     = -10.679367321
		wantXorBits = uint64(9841046885738300356)
	)
	buf := renderSeed(acousticKickSeed, sr, sr*6/10)
	if len(buf) != wantSamples {
		t.Fatalf("sample count = %d, want %d", len(buf), wantSamples)
	}
	var sum float64
	var bits uint64
	for i, v := range buf {
		sum += float64(v)
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	if bits != wantXorBits {
		t.Fatalf("variant-7 render bit-checksum changed: got %d want %d — the DSP-primitive refactor is NOT byte-identical (or the variant-7 DSP changed on purpose; re-capture the anchor)", bits, wantXorBits)
	}
	if math.Abs(sum-wantSum) > 1e-6 {
		t.Fatalf("variant-7 render sum = %.9f, want %.9f", sum, wantSum)
	}
}
