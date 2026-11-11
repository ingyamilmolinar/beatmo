package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// clickTrain builds impulses every periodSec for durSec.
func clickTrain(periodSec, durSec float64, sr int) wave.Wave {
	n := int(durSec * float64(sr))
	s := make([]float64, n)
	step := int(periodSec * float64(sr))
	for i := 0; i < n; i += step {
		s[i] = 1.0
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestDetectTempo_120BPMClickTrain(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.01                // fine hop for tempo resolution
	w := clickTrain(0.5, 4.0, 44100) // 0.5 s period → 120 BPM
	env, fhz := OnsetEnvelope(w, cfg)
	bpm, _ := DetectTempo(env, fhz, cfg)
	if math.Abs(bpm-120) > 120*0.05 && math.Abs(bpm-240) > 240*0.05 && math.Abs(bpm-60) > 60*0.05 {
		t.Errorf("bpm=%.1f want ~120 (or octave 60/240)", bpm)
	}
}

func TestOnsetTimes_FindsClicks(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.01
	w := clickTrain(0.5, 2.0, 44100) // clicks at 0,0.5,1.0,1.5
	env, fhz := OnsetEnvelope(w, cfg)
	times := OnsetTimes(env, fhz, cfg)
	if len(times) < 3 {
		t.Fatalf("found %d onsets, want >=3", len(times))
	}
}
