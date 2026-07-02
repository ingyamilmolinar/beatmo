package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestRMSEnvelope_ConstantAmplitude(t *testing.T) {
	w := wave.Sine(220, 0.5, 1.0, 44100)
	env := RMSEnvelope(w, 256, 50)
	if len(env) == 0 {
		t.Fatal("empty envelope")
	}
	mid := env[len(env)/2] // sine RMS = amp/sqrt(2) ≈ 0.3536
	if math.Abs(mid-0.3536) > 0.05 {
		t.Errorf("mid RMS=%.4f want ~0.3536", mid)
	}
}

func TestAutoSegment_PicksLoudRegion(t *testing.T) {
	quiet := wave.Sine(220, 0.01, 0.5, 44100)
	loud := wave.Sine(220, 0.9, 0.5, 44100)
	cat := wave.Wave{Samples: append(append([]float64{}, quiet.Samples...), loud.Samples...), SampleRate: 44100}
	seg := AutoSegment(cat, 0.3)
	if seg.PeakSample() < 0.5 {
		t.Errorf("autosegment peak %.3f — picked quiet region", seg.PeakSample())
	}
}
