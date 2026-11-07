//go:build !js

package main

import (
	"math"
	"testing"
)

// Every option of a family is level-matched against the SAME reference clip: per-member renders each re-derived the
// reference gain and only the first copy was played, so options sat several dB apart, the very confound level-matching
// removes (the louder source is judged fuller).

const levelEps = 1e-9

// The single-option path must give byte-identical gains to the pairwise helper, so refs compare / refs diagnose do not move.
func TestComputeGainsN_MatchesPairwiseForOneOption(t *testing.T) {
	for _, tc := range [][2]float64{{0.1, 0.1}, {0.1, 0.4}, {0.35, 0.02}, {0, 0.2}, {0.2, 0}} {
		wantRef, wantSynth := computeGains(tc[0], tc[1])
		got := computeGainsN([]float64{tc[0], tc[1]})
		if math.Abs(got[0]-wantRef) > levelEps || math.Abs(got[1]-wantSynth) > levelEps {
			t.Errorf("computeGainsN(%v) = %v, want [%v %v] from computeGains", tc, got, wantRef, wantSynth)
		}
	}
}

// After scaling, the reference and EVERY option sit at one level: the listener compares timbre and nothing else.
func TestComputeGainsN_PutsEveryClipOnOneLevel(t *testing.T) {
	levels := []float64{0.10, 0.10, 0.40, 0.05}
	gains := computeGainsN(levels)
	if len(gains) != len(levels) {
		t.Fatalf("got %d gains for %d levels", len(gains), len(levels))
	}
	target := levels[0] * gains[0]
	for i := range levels {
		got := levels[i] * gains[i]
		if math.Abs(got-target) > levelEps {
			t.Errorf("clip %d lands at %.6f but the reference is at %.6f (%.2f dB apart) -- "+
				"every clip must share one level", i, got, target, 20*math.Log10(got/target))
		}
	}
}

func TestComputeGainsN_LeavesSilentClipsAlone(t *testing.T) {
	gains := computeGainsN([]float64{0.10, 0, 0.20})
	if gains[1] != 1 {
		t.Errorf("a silent clip has no ratio to compute against; want unity gain, got %v", gains[1])
	}
	if gains[0] <= 0 || gains[2] <= 0 {
		t.Errorf("a silent sibling must not zero the other gains: %v", gains)
	}
}

// One factor for the WHOLE set, never just the offending clip, or the guard reintroduces the level difference.
func TestClampFamilyGains_PreservesRelativeLevels(t *testing.T) {
	gains := []float64{4.0, 4.0, 8.0}
	peaks := []float64{0.30, 0.30, 0.20}
	before := make([]float64, len(gains))
	for i := range gains {
		before[i] = peaks[i] * gains[i]
	}

	clamped := clampFamilyGains(gains, peaks, clipCeiling)

	var worst float64
	for i := range clamped {
		if p := peaks[i] * clamped[i]; p > worst {
			worst = p
		}
	}
	if worst > clipCeiling+levelEps {
		t.Errorf("clamped peak %.4f still exceeds the ceiling %.2f", worst, clipCeiling)
	}
	// Every clip must have moved by the SAME factor.
	ratio := clamped[0] / gains[0]
	for i := range gains {
		if got := clamped[i] / gains[i]; math.Abs(got-ratio) > levelEps {
			t.Errorf("clip %d scaled by %.6f but clip 0 scaled by %.6f -- the clamp changed "+
				"their relative loudness", i, got, ratio)
		}
	}
}

func TestClampFamilyGains_NoOpBelowCeiling(t *testing.T) {
	gains := []float64{1.0, 2.0}
	got := clampFamilyGains(gains, []float64{0.1, 0.2}, clipCeiling)
	for i := range gains {
		if got[i] != gains[i] {
			t.Errorf("gain %d changed (%v -> %v) although no peak exceeded the ceiling", i, gains[i], got[i])
		}
	}
}

// TestFamilyLevelGains_ModeDispatch covers the -match vocabulary on the family
// path, including that "none" is genuinely a no-op.
func TestFamilyLevelGains_ModeDispatch(t *testing.T) {
	rms := []float64{0.10, 0.40}
	peaks := []float64{0.20, 0.60}

	none, err := familyLevelGains(matchNone, rms, peaks)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range none {
		if g != 1 {
			t.Errorf("-match none must leave clip %d untouched, got gain %v", i, g)
		}
	}

	byRMS, err := familyLevelGains(matchRMS, rms, peaks)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(rms[0]*byRMS[0]-rms[1]*byRMS[1]) > levelEps {
		t.Error("-match rms must equalise RMS across the family")
	}

	byPeak, err := familyLevelGains(matchPeak, rms, peaks)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(peaks[0]*byPeak[0]-peaks[1]*byPeak[1]) > levelEps {
		t.Error("-match peak must equalise peak across the family")
	}

	if _, err := familyLevelGains("bogus", rms, peaks); err == nil {
		t.Error("an unknown -match mode must be rejected")
	}
}
