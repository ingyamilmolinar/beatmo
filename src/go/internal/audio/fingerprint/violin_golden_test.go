package fingerprint

import "testing"

// TestViolinD5_Characterization verifies key timbral descriptors of a real
// violin D5 recording against numpy-verified ground truth values.
//
// Micro vs macro coupling distinction:
//   - MICRO coupling (CentroidRMSCorr): measured over the steady sustain window
//     only. The violin bow pressure causes louder=slightly-DULLER in the sustain
//     (expressive articulation within the note), so corr ≈ -0.74.
//   - MACRO coupling (MacroCentroidRMSCorr): over the WHOLE note including
//     attack and decay. The expressive crescendo swells brighter as it gets
//     louder, so corr ≈ +0.57 — the opposite sign. This is asserted against
//     fpFull.Coupling.MacroCentroidRMSCorr (the actual stored field on a
//     FromWave call over the raw whole note), not an independent recompute, so
//     the test validates the value Phase-B will consume.
func TestViolinD5_Characterization(t *testing.T) {
	w, ok := loadRefWaveForTest(t)
	if !ok {
		t.Skip("reference WAV unavailable")
	}

	// fpFull: fingerprint over the raw whole note (no AutoSegment). This
	// populates fp.Coupling.MacroCentroidRMSCorr with the true whole-note macro
	// coupling — attack + sustain + decay — the value Phase-B will consume.
	fpFull := FromWave(w, "violin-d5-full")
	if fpFull.Coupling == nil {
		t.Fatal("fpFull coupling nil")
	}
	// MACRO coupling: whole-note corr is positive (louder=brighter crescendo).
	macro := fpFull.Coupling.MacroCentroidRMSCorr
	if macro < 0.40 || macro > 0.90 {
		t.Errorf("fpFull MacroCentroidRMSCorr=%.2f, want ~+0.57 in [0.40, 0.90] (macro: louder=brighter swell)", macro)
	}

	// fp: AutoSegment'd fingerprint for all other descriptors (F0, bridge hill,
	// micro corr, vibrato) which are sustain-window metrics.
	seg := AutoSegment(w, 1.5)
	fp := FromWave(seg, "violin-d5")

	if fp.F0Hz < 583 || fp.F0Hz > 599 {
		t.Errorf("F0Hz=%.1f, want ~591 (±8)", fp.F0Hz)
	}
	if fp.SpectralEnv == nil || fp.SpectralEnv.BridgeHillHz < 2000 || fp.SpectralEnv.BridgeHillHz > 2800 {
		t.Errorf("bridge hill = %+v, want 2000-2800 Hz (canonical violin bridge hill ~2531 Hz)", fp.SpectralEnv)
	}
	if fp.Coupling == nil {
		t.Fatal("coupling nil")
	}
	// MICRO coupling: sustain-window corr is negative (louder=slightly duller in sustain).
	if fp.Coupling.CentroidRMSCorr < -0.90 || fp.Coupling.CentroidRMSCorr > -0.50 {
		t.Errorf("CentroidRMSCorr=%.2f, want ~-0.74 (micro: louder=duller in sustain)", fp.Coupling.CentroidRMSCorr)
	}
	if fp.Vibrato == nil || fp.Vibrato.RateHz < 3 || fp.Vibrato.RateHz > 7 {
		t.Errorf("vibrato rate = %+v, want ~3.65 Hz", fp.Vibrato)
	}
	// Always log the measured characterization for the record:
	t.Logf("D5 characterization: F0=%.1f bridgeHill=%.0f microCorr=%.2f macroCorr=%.2f vibRate=%.2f extent=%.1fc",
		fp.F0Hz, bridgeHillHz(fp), corr(fp), macro, vibRate(fp), vibExtent(fp))
}

// nil-safe helpers for the characterization log line.

func bridgeHillHz(fp Fingerprint) float64 {
	if fp.SpectralEnv == nil {
		return 0
	}
	return fp.SpectralEnv.BridgeHillHz
}

func corr(fp Fingerprint) float64 {
	if fp.Coupling == nil {
		return 0
	}
	return fp.Coupling.CentroidRMSCorr
}

func vibRate(fp Fingerprint) float64 {
	if fp.Vibrato == nil {
		return 0
	}
	return fp.Vibrato.RateHz
}

func vibExtent(fp Fingerprint) float64 {
	if fp.Vibrato == nil {
		return 0
	}
	return fp.Vibrato.ExtentCents
}
