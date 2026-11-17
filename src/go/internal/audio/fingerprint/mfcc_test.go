package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestMFCC_DiffersSineVsNoise(t *testing.T) {
	s := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	n := computeMFCC(wave.Noise(3, 0.9, 0.5, 44100), 13)
	d := 0.0
	for i := 1; i < 13; i++ { // skip c0 (energy)
		d += math.Abs(s[i] - n[i])
	}
	if d < 1.0 {
		t.Errorf("MFCC sine vs noise distance=%.3f too small", d)
	}
}

func TestMFCC_Stable(t *testing.T) {
	a := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	b := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	if a != b {
		t.Error("MFCC not deterministic for identical input")
	}
}
