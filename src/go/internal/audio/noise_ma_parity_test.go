//go:build !test && !js

package audio

import "testing"

// noise_ma transplants miniaudio's ma_noise white stream (Lehmer LCG 48271, 2^31-1); the streams must be bit-identical,
// including the int32 wraparound where state goes negative, which the transplant must reproduce, not "fix".
func TestNoiseMaParity(t *testing.T) {
	// Deep enough into the period for many negative-state wraparounds.
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

// As ma_noise_config_init: seed 0 becomes 4321.
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
