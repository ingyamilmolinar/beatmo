package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestDistance_IdentityZero(t *testing.T) {
	fp := FromWave(wave.Sine(220, 0.9, 1.5, 44100), "a")
	s := Distance(fp, fp)
	if s.Total > 1e-6 {
		t.Errorf("identity Total=%.6f want 0", s.Total)
	}
}

func TestDistance_CloserIsSmaller(t *testing.T) {
	ref := FromWave(wave.Sine(220, 0.9, 1.5, 44100), "ref")
	near := FromWave(wave.Sine(225, 0.9, 1.5, 44100), "near")
	far := FromWave(wave.Noise(5, 0.9, 1.5, 44100), "far")
	if !(Distance(ref, near).Total < Distance(ref, far).Total) {
		t.Errorf("near=%.3f far=%.3f — expected near < far", Distance(ref, near).Total, Distance(ref, far).Total)
	}
}

func TestDistance_Symmetric(t *testing.T) {
	a := FromWave(wave.Sine(220, 0.9, 1.0, 44100), "a")
	b := FromWave(wave.Sine(300, 0.9, 1.0, 44100), "b")
	if math.Abs(Distance(a, b).Total-Distance(b, a).Total) > 1e-9 {
		t.Error("Distance not symmetric")
	}
}

func TestDistance_IdentityStillZero(t *testing.T) {
	w := synthVibratoTone(48000, 2.0, 440, []float64{1, .6, .3}, 6, 25)
	fp := FromWave(w, "a")
	if d := Distance(fp, fp).Total; d > 1e-9 {
		t.Fatalf("identity distance=%.6g, want 0", d)
	}
}

func TestDistance_VibratoTermDiscriminates(t *testing.T) {
	vib := FromWave(synthVibratoTone(48000, 2, 440, []float64{1, .6, .3}, 6, 30), "vib")
	flat := FromWave(synthTone(48000, 2, 440, []float64{1, .6, .3}), "flat")
	if Distance(vib, flat).Vibrato <= 0 {
		t.Fatal("vibrato term did not discriminate vibrato vs flat")
	}
}

func TestDistance_Symmetry(t *testing.T) {
	a := FromWave(synthVibratoTone(48000, 2, 440, []float64{1, .6, .3}, 6, 30), "a")
	b := FromWave(synthTone(48000, 2, 392, []float64{1, .4, .2}), "b")
	if math.Abs(Distance(a, b).Total-Distance(b, a).Total) > 1e-9 {
		t.Fatal("distance not symmetric")
	}
}
