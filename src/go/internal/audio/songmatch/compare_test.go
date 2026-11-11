package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestCompare_IdentityAllOK(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	fp := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	r := Compare(fp, fp, cfg)
	if len(r.Axes) != 4 {
		t.Fatalf("got %d axes, want 4", len(r.Axes))
	}
	for _, a := range r.Axes {
		if a.State != "OK" {
			t.Errorf("axis %s state=%s want OK (mag=%v)", a.Axis, a.State, a.Magnitude)
		}
	}
}

func TestCompare_NoiseVsToneFlagsAxes(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	ref := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	cand := fingerprint.SongFingerprintOf(wave.Noise(5, 0.9, 2.0, 44100), cfg)
	r := Compare(ref, cand, cfg)
	flagged := false
	for _, a := range r.Axes {
		if a.State != "OK" {
			flagged = true
		}
	}
	if !flagged {
		t.Error("sine vs noise should flag at least one axis")
	}
}

func TestCompare_Deterministic(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	ref := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	cand := fingerprint.SongFingerprintOf(wave.Sine(330, 0.9, 2.0, 44100), cfg)
	a := Compare(ref, cand, cfg)
	b := Compare(ref, cand, cfg)
	if a.Distance != b.Distance || a.ImpliedTranspose != b.ImpliedTranspose || len(a.Axes) != len(b.Axes) {
		t.Fatal("Compare not deterministic")
	}
	for i := range a.Axes {
		if a.Axes[i] != b.Axes[i] {
			t.Fatalf("axis %d differs between runs", i)
		}
	}
}
