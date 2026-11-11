//go:build !test && !js

package audio

import "testing"

// noise_ma is the in-repo transplant of miniaudio's ma_noise white stream
// (Lehmer LCG, MA_LCG_A=48271, MA_LCG_M=2^31-1, MA_LCG_C=0; seed 0 is
// replaced by MA_DEFAULT_LCG_SEED=4321 in ma_noise_config_init —
// miniaudio.h:66997). The legacy drum renderers consume ma_noise with seed
// 0; their modular-preset replacements consume noise_ma. This test is the
// proof the two streams are bit-identical, including the int32-overflow
// wraparound inside the LCG step (state can go negative — the transplant
// must reproduce that, not "fix" it).
func TestNoiseMaParity(t *testing.T) {
	// 262144 draws — far longer than any voice render, and deep enough into
	// the LCG period to exercise many negative-state wraparounds where a
	// wrong sign/modulus reproduction would surface.
	const n = 1 << 18
	want := make([]float32, n)
	got := make([]float32, n)
	oracleMaNoiseWhiteFill(want, 0) // real ma_noise, seed 0
	noiseMaWhiteFill(got, 0)        // transplant,    seed 0

	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("stream diverged at draw %d: ma_noise=%v noise_ma=%v", i, want[i], got[i])
		}
	}
}

// Seed handling must match ma_noise_config_init: explicit non-zero seeds
// pass through; 0 becomes 4321.
func TestNoiseMaSeedZeroIsDefault(t *testing.T) {
	a := make([]float32, 1024)
	b := make([]float32, 1024)
	noiseMaWhiteFill(a, 0)
	noiseMaWhiteFill(b, 4321)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed 0 and seed 4321 diverged at %d", i)
		}
	}
	c := make([]float32, 1024)
	noiseMaWhiteFill(c, 7)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("seed 7 produced the same stream as seed 4321 — seeding is inert")
	}
}
