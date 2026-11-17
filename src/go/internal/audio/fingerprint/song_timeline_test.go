package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongTimelineOf_FramesCoverDuration(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.1
	w := wave.Sine(220, 0.8, 3.0, 44100)
	tl := SongTimelineOf(w, cfg)
	if len(tl.Frames) < 20 {
		t.Fatalf("got %d frames for 3 s @0.1 hop", len(tl.Frames))
	}
	// Frames must be time-ordered.
	for i := 1; i < len(tl.Frames); i++ {
		if tl.Frames[i].TimeSec < tl.Frames[i-1].TimeSec {
			t.Fatalf("frames not time-ordered at %d", i)
		}
	}
}

func TestSongTimelineOf_NonPow2FrameFFTBandConsistency(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.1
	cfg.FrameFFTSize = 3000 // NON power-of-two → MagnitudeSpectrum rounds up to 4096
	sr := 44100
	// A 1600 Hz tone is in the "mid" band (250–2000). With the binHz bug
	// (binHz computed as sr/3000 instead of sr/4096), the peak is mis-attributed
	// to the "high" band (2000–6000). This asserts correct band scaling.
	w := wave.Sine(1600, 0.9, 2.0, sr)
	tl := SongTimelineOf(w, cfg)
	if len(tl.Frames) == 0 {
		t.Fatal("no frames")
	}
	mid := tl.Frames[len(tl.Frames)/2]
	maxIdx, maxVal := 0, 0.0
	for i, b := range mid.Bands {
		if b > maxVal {
			maxVal = b
			maxIdx = i
		}
	}
	if cfg.Bands[maxIdx].Name != "mid" {
		t.Errorf("1600 Hz tone: dominant band=%q (idx %d), want \"mid\" — per-frame binHz mis-scaled (nextPow2 mismatch)",
			cfg.Bands[maxIdx].Name, maxIdx)
	}
}

func TestSongTimelineOf_DetectsSectionBoundary(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.05
	cfg.NoveltyKernel = 8
	sr := 44100
	// First half low tone, second half high tone → a boundary near the middle.
	lo := wave.Sine(150, 0.8, 2.0, sr).Samples
	hi := wave.Sine(1500, 0.8, 2.0, sr).Samples
	w := wave.Wave{Samples: append(append([]float64{}, lo...), hi...), SampleRate: sr}
	tl := SongTimelineOf(w, cfg)
	found := false
	for _, s := range tl.Sections {
		if s.StartSec > 1.5 && s.StartSec < 2.5 {
			found = true
		}
	}
	if len(tl.Sections) < 2 || !found {
		t.Errorf("expected a section boundary near 2.0 s; sections=%+v", tl.Sections)
	}
}
