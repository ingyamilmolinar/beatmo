//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Variant 7 composes the static-inline DSP primitives with the exact float-op order of the open-coded version, so a
// single changed sample fails here. Re-capture only for an intentional variant-7 DSP change.
func TestVariant7RefactorByteIdentity(t *testing.T) {
	const (
		sr          = 44100
		wantSamples = 26460
		wantSum     = -10.679367535
		wantXorBits = uint64(9819419212577791131)
	)
	// The test pins the C voice, not the instrument's tuning, so the two values the kick's later retune moved are frozen
	// at the anchors' values.
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
