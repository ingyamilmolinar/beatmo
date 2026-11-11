package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSpectralFlatness_SineLowNoiseHigh(t *testing.T) {
	var s, n Fingerprint
	computeShape(wave.Sine(440, 0.9, 1.0, 44100), &s)
	computeShape(wave.Noise(11, 0.9, 1.0, 44100), &n)
	if s.SpectralFlatness > 0.2 {
		t.Errorf("sine flatness=%.3f want low", s.SpectralFlatness)
	}
	if n.SpectralFlatness < 0.4 {
		t.Errorf("noise flatness=%.3f want high", n.SpectralFlatness)
	}
}
