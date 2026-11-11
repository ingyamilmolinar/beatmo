package fingerprint

import "testing"

// TestDetectTempo_OctaveCorrectsSlow: a 40-BPM click train was previously
// reported as 40 (the bar/harmonic-rhythm lock); it must now octave-correct
// above the musical floor.
func TestDetectTempo_OctaveCorrectsSlow(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.01
	w := clickTrain(1.5, 12.0, 44100) // clicks every 1.5 s = 40 BPM
	env, fhz := OnsetEnvelope(w, cfg)
	bpm, _ := DetectTempo(env, fhz, cfg)
	if bpm < tempoFoldFloorBPM {
		t.Errorf("tempo=%.1f want >= %.0f (octave-corrected from ~40)", bpm, tempoFoldFloorBPM)
	}
}

// TestHarmonicProfile_IgnoresSubAudio: a loud sub-audio bin must not be picked
// as the primary; the musical peak above harmonicMinHz wins.
func TestHarmonicProfile_IgnoresSubAudio(t *testing.T) {
	binHz := 44100.0 / 16384.0 // ~2.69 Hz/bin
	mag := make([]float64, 8192)
	mag[3] = 10.0  // ~8 Hz sub-audio rumble — loudest, but must be ignored
	mag[100] = 4.0 // ~269 Hz musical peak
	peaks := HarmonicProfile(mag, binHz, 3)
	if len(peaks) == 0 {
		t.Fatal("no peaks returned")
	}
	if peaks[0].Hz < harmonicMinHz {
		t.Errorf("primary=%.1f Hz is sub-audio; want >= %.0f", peaks[0].Hz, harmonicMinHz)
	}
}
