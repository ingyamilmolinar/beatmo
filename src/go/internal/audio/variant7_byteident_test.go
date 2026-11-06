//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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
// anchors below (render the pinned document at 44100 Hz for 0.6 s and recompute).
func TestVariant7RefactorByteIdentity(t *testing.T) {
	const (
		sr          = 44100
		wantSamples = 26460
		wantSum     = -10.679367535
		wantXorBits = uint64(9819419212577791131)
	)
	// Re-captured 2026-10-02 (config-only refactor Phase A: the param block is
	// double, so the seed's non-float32-exact values — h2 0.35, click 0.9,
	// reverb 0.85 … — now reach the voice exactly; with params rounded back to
	// float32 (BEATMO_MODULAR_PARAMS_F32=1) the old anchor still reproduces).
	// The anchors were captured with the acoustic seed as it stood before the
	// 2026-10-02 fullness retune (attack 1.0, sat 0.25); the test pins the C
	// voice, not the instrument's tuning, so it renders the kick-acoustic
	// document (the baked seed, bit-identically) with those two values frozen.
	buf := renderFieldsForTest(t, roster.KickAcoustic, sr, sr*6/10, map[string]float64{"gen1_kick_attack": 1.0, "gen1_kick_sat": 0.25})
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
		t.Fatalf("variant-7 render bit-checksum changed: got %d (sum %.9f) want %d — the DSP-primitive refactor is NOT byte-identical (or the variant-7 DSP changed on purpose; re-capture the anchor)", bits, sum, wantXorBits)
	}
	if math.Abs(sum-wantSum) > 1e-6 {
		t.Fatalf("variant-7 render sum = %.9f, want %.9f", sum, wantSum)
	}
}
