package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Local to these tests, so it cannot drift with the older analyzers' fixtures.
func bundleTone(sr int, hz, secs float64) wave.Wave {
	n := int(float64(sr) * secs)
	s := make([]float64, n)
	for i := range s {
		s[i] = 0.5 * math.Sin(2*math.Pi*hz*float64(i)/float64(sr))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func bundleSilence(sr int, secs float64) wave.Wave {
	return wave.Wave{Samples: make([]float64, int(float64(sr)*secs)), SampleRate: sr}
}

func TestBundleMemoizesFingerprint(t *testing.T) {
	b := NewBundle(bundleTone(44100, 220, 2), "t")
	a1 := b.Fingerprint()
	a2 := b.Fingerprint()
	if a1.F0Hz != a2.F0Hz {
		t.Fatal("Fingerprint() is not stable across calls")
	}
	if b.fingerprintCalls != 1 {
		t.Fatalf("Fingerprint computed %d times, want 1", b.fingerprintCalls)
	}
}

func TestBundleMemoizesSegments(t *testing.T) {
	b := NewBundle(bundleTone(44100, 220, 2), "t")
	_ = b.Segments()
	_ = b.Segments()
	if b.segmentCalls != 1 {
		t.Fatalf("Segments computed %d times, want 1", b.segmentCalls)
	}
}

func TestBundleSTFTMemoizedPerShape(t *testing.T) {
	b := NewBundle(bundleTone(44100, 220, 2), "t")
	_ = b.STFT(2048, 512)
	_ = b.STFT(2048, 512)
	_ = b.STFT(1024, 256)
	if b.stftCalls != 2 {
		t.Fatalf("STFT computed %d times, want 2 (one per distinct shape)", b.stftCalls)
	}
}

func TestBundleSTFTShape(t *testing.T) {
	sr := 44100
	b := NewBundle(bundleTone(sr, 440, 1), "t")
	s := b.STFT(2048, 512)
	if len(s.Frames) == 0 {
		t.Fatal("no STFT frames produced")
	}
	if got, want := len(s.Frames[0]), 2048/2+1; got != want {
		t.Errorf("bins per frame: got %d want %d", got, want)
	}
	if want := float64(sr) / 2048; math.Abs(s.BinHz-want) > 1e-9 {
		t.Errorf("BinHz: got %v want %v", s.BinHz, want)
	}
	if want := 512.0 / float64(sr); math.Abs(s.HopSec-want) > 1e-12 {
		t.Errorf("HopSec: got %v want %v", s.HopSec, want)
	}
	// A 440 Hz tone must peak in the bin nearest 440 Hz.
	peak, peakBin := 0.0, 0
	for i, m := range s.Frames[len(s.Frames)/2] {
		if m > peak {
			peak, peakBin = m, i
		}
	}
	if hz := float64(peakBin) * s.BinHz; math.Abs(hz-440) > s.BinHz {
		t.Errorf("peak at %v Hz, want ~440", hz)
	}
}

func TestBundleSTFTOnShortWaveIsEmptyNotPanic(t *testing.T) {
	b := NewBundle(wave.Wave{Samples: []float64{0.1, 0.2}, SampleRate: 44100}, "short")
	s := b.STFT(2048, 512)
	if len(s.Frames) != 0 {
		t.Fatalf("a wave shorter than one frame must yield no frames, got %d", len(s.Frames))
	}
}

func TestBundleSilenceSegmentsAreSilent(t *testing.T) {
	b := NewBundle(bundleSilence(44100, 1), "silent")
	if !b.Segments().Silent {
		t.Fatal("silence must be reported as Silent")
	}
}
