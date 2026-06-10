//go:build test

package scope

import (
	"math"
	"testing"
)

// TestService_LatestPeakReflectsLastPush — Phase 3 chain per-stage
// meters: every stage that received samples (even if not currently
// tapped) must expose its latest peak/RMS via LatestPeak so the UI can
// paint a glanceable signal-flow display.
func TestService_LatestPeakReflectsLastPush(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})

	// Push a known wave (peak 0.5, RMS = 0.5 / sqrt(2) for a sine).
	buf := make([]float64, 1024)
	for i := range buf {
		buf[i] = 0.5 * math.Sin(2*math.Pi*float64(i)/64)
	}
	svc.PushSamples(StageInsertFX, "kick", buf)

	peakDB, rmsDB := svc.LatestPeak(StageInsertFX)
	wantPeakDB := 20 * math.Log10(0.5) // -6.02 dB
	wantRMSDB := 20 * math.Log10(0.5/math.Sqrt2)
	if math.Abs(peakDB-wantPeakDB) > 0.5 {
		t.Errorf("peakDB: got %v, want ~%v", peakDB, wantPeakDB)
	}
	if math.Abs(rmsDB-wantRMSDB) > 0.5 {
		t.Errorf("rmsDB: got %v, want ~%v", rmsDB, wantRMSDB)
	}
}

// TestService_LatestPeakSilentStageReturnsFloor — a stage that never
// received any samples should return -math.Inf (or our floor) for
// peakDB so the UI renders it as silent.
func TestService_LatestPeakSilentStageReturnsFloor(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})

	peakDB, rmsDB := svc.LatestPeak(StageMaster)
	if !math.IsInf(peakDB, -1) {
		t.Errorf("silent stage peakDB: got %v, want -Inf", peakDB)
	}
	if !math.IsInf(rmsDB, -1) {
		t.Errorf("silent stage rmsDB: got %v, want -Inf", rmsDB)
	}
}

// TestService_LatestPeakIndependentAcrossStages — pushing into one
// stage must not bleed into another stage's reading.
func TestService_LatestPeakIndependentAcrossStages(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})

	loud := make([]float64, 256)
	for i := range loud {
		loud[i] = 0.9
	}
	svc.PushSamples(StageEQ, "kick", loud)

	if peakDB, _ := svc.LatestPeak(StageEQ); peakDB < -2 {
		t.Errorf("StageEQ peakDB after loud push: got %v, want close to 0", peakDB)
	}
	if peakDB, _ := svc.LatestPeak(StageSynth); !math.IsInf(peakDB, -1) {
		t.Errorf("StageSynth peakDB should be -Inf (no push), got %v", peakDB)
	}
}
