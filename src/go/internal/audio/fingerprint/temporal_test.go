package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestTemporal_VibratoDetected(t *testing.T) {
	sr := 44100
	dur := 1.5
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		depth := math.Pow(2, 20.0/1200) - 1 // ~20 cents
		f := 440 * (1 + depth*math.Sin(2*math.Pi*5*tsec))
		s[i] = 0.8 * math.Sin(2*math.Pi*f*tsec)
	}
	tf := computeTemporal(wave.Wave{Samples: s, SampleRate: sr}, 440)
	if !tf.VibratoDetected || math.Abs(tf.VibratoRateHz-5) > 1.0 {
		t.Errorf("vibrato=%v rate=%.2f want detected ~5Hz", tf.VibratoDetected, tf.VibratoRateHz)
	}
}

func TestTemporal_StaticToneLowFlux(t *testing.T) {
	tf := computeTemporal(wave.Sine(440, 0.8, 1.5, 44100), 440)
	if tf.HarmonicFluxMean > 0.05 {
		t.Errorf("static sine flux=%.4f want ~0", tf.HarmonicFluxMean)
	}
}
