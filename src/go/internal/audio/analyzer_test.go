package audio

import (
	"math"
	"testing"
)

func TestAnalyzerCapturesSpectrum(t *testing.T) {
	withDefaultAudio(t)
	const (
		id     = "an-test"
		sr     = 44100
		freq   = 440.0
		window = 256
	)
	an := EnableChannelAnalyzer(id, window)
	ch := InstrumentChannel(id)
	for i := 0; i < window; i++ {
		x := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
		ch.ProcessSample(x)
	}
	snap := an.Snapshot()
	if snap.RMS <= 0 {
		t.Fatalf("expected RMS to be > 0, got %.4f", snap.RMS)
	}
	if len(snap.Spectrum) != window/2 {
		t.Fatalf("expected %d spectrum bins, got %d", window/2, len(snap.Spectrum))
	}
	// Peak bin should be non-zero for the sine.
	var max float64
	for _, v := range snap.Spectrum {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		t.Fatal("expected non-zero spectrum magnitude")
	}
}

func TestAnalyzerWindowClampsAndRounds(t *testing.T) {
	withDefaultAudio(t)
	if an := NewAnalyzer(10); len(an.window) != 64 {
		t.Fatalf("expected min window 64, got %d", len(an.window))
	}
	if an := NewAnalyzer(10000); len(an.window) != 8192 {
		t.Fatalf("expected max window 8192, got %d", len(an.window))
	}
	if an := NewAnalyzer(300); len(an.window) != 256 {
		t.Fatalf("expected nearest pow2=256, got %d", len(an.window))
	}
}

func TestAnalyzerWaveformLengthBeforeFilled(t *testing.T) {
	withDefaultAudio(t)
	an := NewAnalyzer(64)
	for i := 0; i < 10; i++ {
		an.ProcessSample(0.5)
	}
	snap := an.Snapshot()
	if len(snap.Waveform) != 10 {
		t.Fatalf("expected waveform length 10 before filled, got %d", len(snap.Waveform))
	}
}
