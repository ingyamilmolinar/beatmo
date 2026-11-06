package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// confTone builds a steady harmonic tone — unambiguously pitched.
func confTone(sr int, f0 float64, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5*math.Sin(2*math.Pi*f0*t) + 0.25*math.Sin(4*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// confNoise builds broadband noise — no fundamental exists at all.
func confNoise(sr int, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	rng := rand.New(rand.NewSource(1))
	for i := range s {
		s[i] = rng.Float64()*2 - 1
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestDetectF0Detailed_ConfidenceSeparatesPitchedFromNoise(t *testing.T) {
	tone := DetectF0Detailed(confTone(44100, 220, 2))
	noise := DetectF0Detailed(confNoise(44100, 2))

	t.Logf("tone:  hz=%.2f confidence=%.3f frames=%d/%d", tone.Hz, tone.Confidence, tone.ConfidentFrames, tone.TotalFrames)
	t.Logf("noise: hz=%.2f confidence=%.3f frames=%d/%d", noise.Hz, noise.Confidence, noise.ConfidentFrames, noise.TotalFrames)

	if !tone.Reliable() {
		t.Errorf("a steady harmonic tone must have a reliable F0, got confidence=%.3f", tone.Confidence)
	}
	if noise.Reliable() {
		t.Errorf("broadband noise must NOT have a reliable F0, got hz=%.2f confidence=%.3f", noise.Hz, noise.Confidence)
	}
	if noise.Confidence >= tone.Confidence {
		t.Errorf("noise confidence %.3f must be below tone confidence %.3f", noise.Confidence, tone.Confidence)
	}
}

// DetectF0 must keep returning exactly what DetectF0Detailed reports, so every
// existing caller is unaffected by the split.
func TestDetectF0MatchesDetailed(t *testing.T) {
	for _, w := range []wave.Wave{confTone(44100, 220, 2), confTone(44100, 440, 1), confNoise(44100, 1)} {
		if got, want := DetectF0(w), DetectF0Detailed(w).Hz; got != want {
			t.Errorf("DetectF0=%v but DetectF0Detailed().Hz=%v", got, want)
		}
	}
}

func TestF0Confidence_IsSurfacedOnFingerprint(t *testing.T) {
	fp := FromWave(confTone(44100, 220, 2), "tone")
	if fp.F0Confidence <= 0 {
		t.Fatalf("Fingerprint.F0Confidence not populated: %v", fp.F0Confidence)
	}
	if fp.F0Confidence != DetectF0Detailed(confTone(44100, 220, 2)).Confidence {
		t.Error("Fingerprint.F0Confidence must be the detector's own value, not a re-derivation")
	}
}
