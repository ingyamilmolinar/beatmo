package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestHarmonicTrajectory_DetectsMidNotePeak is the primary TDD test: H2 of a
// synthSwellH2 wave peaks at the midpoint of the window, so PeakTimeSec[1]
// must land in the middle-ish region (not at the very start or end).
func TestHarmonicTrajectory_DetectsMidNotePeak(t *testing.T) {
	sr := 48000
	w := synthSwellH2(sr, 2.0, 440)
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	if traj.PeakTimeSec[1] < 0.2 || traj.PeakTimeSec[1] > 1.1 {
		t.Fatalf("H2 peak time=%.2fs, want mid-window (0.2..1.1)", traj.PeakTimeSec[1])
	}
	if traj.N < 2 {
		t.Fatalf("N=%d, want >=2", traj.N)
	}
}

// TestHarmonicTrajectory_NTracked verifies N == min(16, nyquist/f0).
func TestHarmonicTrajectory_NTracked(t *testing.T) {
	sr := 48000
	f0 := 440.0
	// nyquist/f0 = 24000/440 ≈ 54, so N should be capped at 16.
	w := synthTone(sr, 1.0, f0, []float64{1.0, 0.5, 0.25})
	traj := computeHarmonicTrajectory(sustainWindow(w), f0, DefaultAnalysisConfig())
	if traj.N != 16 {
		t.Fatalf("N=%d, want 16 (capped)", traj.N)
	}
}

// TestHarmonicTrajectory_SteadyToneH1SustainLevel verifies that for a steady
// pure tone, SustainLevel[0] is near 1.0 (normalized to itself).
func TestHarmonicTrajectory_SteadyToneH1SustainLevel(t *testing.T) {
	sr := 48000
	w := synthTone(sr, 2.0, 440, []float64{1.0})
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	// SustainLevel[0] = median(middle-third) / H1Peak; for a steady tone that is ~1.0
	if traj.SustainLevel[0] < 0.8 || traj.SustainLevel[0] > 1.2 {
		t.Fatalf("H1 SustainLevel=%.3f, want ~1.0 for steady tone", traj.SustainLevel[0])
	}
}

// TestHarmonicTrajectory_SwellDecayRateNegative verifies that for synthSwellH2,
// H2's DecayRate is negative (amplitude falls post-peak).
func TestHarmonicTrajectory_SwellDecayRateNegative(t *testing.T) {
	sr := 48000
	w := synthSwellH2(sr, 2.0, 440)
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	if traj.DecayRate[1] >= 0 {
		t.Fatalf("H2 DecayRate=%.3f, want negative (post-peak swell decays)", traj.DecayRate[1])
	}
}

// TestHarmonicTrajectory_EvenOddLength verifies EvenOddOverTime has one entry
// per STFT frame and that values are finite and non-negative.
func TestHarmonicTrajectory_EvenOddLength(t *testing.T) {
	sr := 48000
	w := synthTone(sr, 1.0, 440, []float64{1.0, 0.5, 0.25, 0.125})
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	if len(traj.EvenOddOverTime) == 0 {
		t.Fatal("EvenOddOverTime is empty, want at least one frame")
	}
	for i, v := range traj.EvenOddOverTime {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("EvenOddOverTime[%d]=%v, want finite non-negative", i, v)
		}
	}
}

// TestHarmonicTrajectory_SilentInput verifies that a silent wave returns a
// zero HarmonicTrajectory without panicking and without NaN/Inf values.
func TestHarmonicTrajectory_SilentInput(t *testing.T) {
	sr := 48000
	silent := make([]float64, sr*2)
	w := wave.Wave{Samples: silent, SampleRate: sr}
	traj := computeHarmonicTrajectory(w, 440, DefaultAnalysisConfig())
	// Must not NaN/Inf any fixed-size array field.
	for k := 0; k < 16; k++ {
		if math.IsNaN(traj.PeakTimeSec[k]) || math.IsInf(traj.PeakTimeSec[k], 0) {
			t.Fatalf("PeakTimeSec[%d]=%v, want finite", k, traj.PeakTimeSec[k])
		}
		if math.IsNaN(traj.AttackRate[k]) || math.IsInf(traj.AttackRate[k], 0) {
			t.Fatalf("AttackRate[%d]=%v, want finite", k, traj.AttackRate[k])
		}
	}
}

// TestHarmonicTrajectory_SwellAttackRatePositive verifies that H2 of the swell
// tone has a positive AttackRate (amplitude rises in dB before its peak).
func TestHarmonicTrajectory_SwellAttackRatePositive(t *testing.T) {
	sr := 48000
	w := synthSwellH2(sr, 2.0, 440)
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	if traj.AttackRate[1] <= 0 {
		t.Fatalf("H2 AttackRate=%.3f dB/s, want positive (swell rises)", traj.AttackRate[1])
	}
}
