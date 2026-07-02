package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestHarmonicProfile_PrimaryIsLoudest(t *testing.T) {
	sr := 44100
	// 220 Hz strong + 440 Hz weaker.
	a := wave.Sine(220, 0.9, 0.7, sr).Samples
	b := wave.Sine(440, 0.3, 0.7, sr).Samples
	s := make([]float64, len(a))
	for i := range s {
		s[i] = a[i] + b[i]
	}
	mag, hz := wave.MagnitudeSpectrum(wave.Wave{Samples: s, SampleRate: sr}, 16384, wave.WindowHann)
	peaks := HarmonicProfile(mag, hz, 3)
	if len(peaks) < 2 {
		t.Fatalf("got %d peaks", len(peaks))
	}
	if math.Abs(peaks[0].Hz-220) > 10 {
		t.Errorf("primary=%.1f want ~220", peaks[0].Hz)
	}
	if math.Abs(peaks[1].Hz-440) > 15 {
		t.Errorf("secondary=%.1f want ~440", peaks[1].Hz)
	}
}

func TestIntonationCents_DetunedSine(t *testing.T) {
	sr := 44100
	// 440 Hz is exactly A4 → ~0 cents. 453 Hz is ~+50 cents sharp.
	m0, hz := wave.MagnitudeSpectrum(wave.Sine(440, 0.9, 0.7, sr), 16384, wave.WindowHann)
	mSharp, _ := wave.MagnitudeSpectrum(wave.Sine(453, 0.9, 0.7, sr), 16384, wave.WindowHann)
	if math.Abs(IntonationCents(m0, hz)) > 15 {
		t.Errorf("on-tune cents=%.1f want ~0", IntonationCents(m0, hz))
	}
	if IntonationCents(mSharp, hz) < 20 {
		t.Errorf("sharp cents=%.1f want clearly positive", IntonationCents(mSharp, hz))
	}
}
