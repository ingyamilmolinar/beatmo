package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestTempoMagnitude_OctaveEquivalent(t *testing.T) {
	if m := TempoMagnitude(120, 120); m > 1e-9 {
		t.Errorf("identical tempo mag=%v want 0", m)
	}
	if m := TempoMagnitude(120, 240); m > 0.05 {
		t.Errorf("octave tempo mag=%v want ~0 (folded)", m)
	}
	if m := TempoMagnitude(120, 130); !(m > 0) {
		t.Errorf("near tempo mag=%v want >0", m)
	}
}

func TestMixMagnitude_IdentityAndDiff(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	if m := MixMagnitude(a, a); m > 1e-9 {
		t.Errorf("identity mix mag=%v want 0", m)
	}
	b := []float64{5, 4, 3, 2, 1}
	if MixMagnitude(a, b) <= 0 {
		t.Error("different band balance should give >0")
	}
}

func TestSongDistance_IdentityZero(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	fp := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	s := SongDistance(fp, fp, cfg)
	if s.Total > 1e-6 {
		t.Errorf("identity Total=%v want 0", s.Total)
	}
}

func TestKeyMagnitude_SilentIdentityIsZero(t *testing.T) {
	// Two all-zero-chroma (silent) fingerprints must be a perfect key match, not 0.5.
	var silent fingerprint.SongFingerprint // zero value: Chroma all zero
	mag, shift := KeyMagnitude(silent, silent)
	if mag != 0 || shift != 0 {
		t.Errorf("KeyMagnitude(silent,silent)=(%v,%d) want (0,0)", mag, shift)
	}
	// And SongDistance identity holds for the zero fingerprint.
	cfg := fingerprint.DefaultAnalysisConfig()
	if s := SongDistance(silent, silent, cfg); s.Total != 0 {
		t.Errorf("SongDistance(silent,silent).Total=%v want 0", s.Total)
	}
}
