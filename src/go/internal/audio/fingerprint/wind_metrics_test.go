package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// mkNote builds preSec of low-level breath noise (breathAmp of peak) followed
// by a tone with a linear attackSec ramp to full level, sustained to durSec.
func mkNote(sr int, preSec, breathAmp, attackSec, durSec, f0 float64) wave.Wave {
	n := int((preSec + durSec) * float64(sr))
	s := make([]float64, n)
	rng := uint32(12345)
	noise := func() float64 {
		rng = rng*1664525 + 1013904223
		return (float64(rng)/float64(math.MaxUint32))*2 - 1
	}
	pre := int(preSec * float64(sr))
	for i := 0; i < pre; i++ {
		s[i] = breathAmp * noise()
	}
	for i := pre; i < n; i++ {
		t := float64(i-pre) / float64(sr)
		envA := 1.0
		if attackSec > 0 && t < attackSec {
			envA = t / attackSec
		}
		s[i] = 0.8 * envA * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestOnsetSegment_SkipsBreathPreroll(t *testing.T) {
	// 1.4 s of 2%-level breath (like the mtg bari-sax recording) then the tone.
	// A 1%-of-peak trigger would fire inside the breath; the 10% trigger with
	// 1% backtrack must land within ~50 ms of the true tone start.
	sr := 48000
	w := mkNote(sr, 1.4, 0.02, 0.15, 1.5, 123)
	_, onset := OnsetSegment(w, 1.0)
	if math.Abs(onset-1.4) > 0.05 {
		t.Errorf("onset=%.3fs want ~1.4 (must skip the breath preroll)", onset)
	}
}

func TestComputeWindAttack_AttackTime(t *testing.T) {
	sr := 48000
	w := mkNote(sr, 0.5, 0.02, 0.20, 1.5, 123)
	wa := ComputeWindAttack(w)
	// Linear ramp: 10→90% of amplitude = 0.8×attackSec = 0.16 s.
	if math.Abs(wa.AttackTimeSec-0.16) > 0.04 {
		t.Errorf("AttackTimeSec=%.3f want ~0.16", wa.AttackTimeSec)
	}
	if wa.TemporalCentroid <= 0 {
		t.Errorf("TemporalCentroid=%.3f want > 0", wa.TemporalCentroid)
	}
}

func TestComputeWindAttack_CentroidBloom(t *testing.T) {
	// A tone whose 2nd..4th harmonics fade in AFTER the fundamental (wind
	// "bloom") must show a rising centroid trajectory and a positive slope.
	sr := 48000
	n := int(1.0 * float64(sr))
	s := make([]float64, n)
	f0 := 200.0
	for i := range s {
		t := float64(i) / float64(sr)
		hi := math.Min(t/0.3, 1) // upper harmonics ramp over 300 ms
		s[i] = 0.5*math.Sin(2*math.Pi*f0*t) +
			hi*(0.4*math.Sin(2*math.Pi*2*f0*t)+
				0.3*math.Sin(2*math.Pi*3*f0*t)+
				0.25*math.Sin(2*math.Pi*4*f0*t))
	}
	wa := ComputeWindAttack(wave.Wave{Samples: s, SampleRate: sr})
	if wa.CentroidSlopeHzS <= 0 {
		t.Errorf("CentroidSlopeHzS=%.1f want > 0 (upper harmonics arrive later)", wa.CentroidSlopeHzS)
	}
	if wa.AttackCentroids[3] <= wa.AttackCentroids[0] {
		t.Errorf("centroid at 300ms (%.0f) must exceed centroid at 25ms (%.0f)",
			wa.AttackCentroids[3], wa.AttackCentroids[0])
	}
}

func TestSpectralIrregularity_SmoothVsJagged(t *testing.T) {
	var smooth, jagged [16]float64
	for i := 0; i < 16; i++ {
		smooth[i] = 1.0 / float64(i+1) // saw-like 1/n envelope
	}
	// Sax-like alternating envelope (measured mtg bari): strong even/odd swings.
	meas := []float64{1.0, 2.03, 1.83, 1.03, 0.73, 1.36, 0.66, 0.39,
		0.59, 0.78, 0.38, 0.24, 0.33, 0.49, 0.61, 0.59}
	copy(jagged[:], meas)
	s := SpectralIrregularity(smooth)
	j := SpectralIrregularity(jagged)
	if j <= s*2 {
		t.Errorf("jagged=%.2f dB should clearly exceed smooth=%.2f dB", j, s)
	}
	if s > 1.0 {
		t.Errorf("smooth 1/n envelope irregularity=%.2f dB want < 1 dB", s)
	}
}

func TestFormantBandEnergies_PlacesEnergy(t *testing.T) {
	sr := 48000
	// Two tones: 600 Hz (band 450-900) and 3000 Hz (band 2000-8000), equal
	// amplitude → equal energy split between bands 1 and 3, none in 0 and 2.
	n := int(1.0 * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5*math.Sin(2*math.Pi*600*t) + 0.5*math.Sin(2*math.Pi*3000*t)
	}
	be := FormantBandEnergies(wave.Wave{Samples: s, SampleRate: sr}, WindFormantBandsHz)
	if len(be) != 4 {
		t.Fatalf("len=%d want 4", len(be))
	}
	if math.Abs(be[1]-0.5) > 0.05 || math.Abs(be[3]-0.5) > 0.05 {
		t.Errorf("bands=%v want ~[0, 0.5, 0, 0.5]", be)
	}
	if be[0] > 0.02 || be[2] > 0.02 {
		t.Errorf("bands=%v: leakage into empty bands", be)
	}
	sum := be[0] + be[1] + be[2] + be[3]
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("band fractions sum=%.6f want 1", sum)
	}
}
