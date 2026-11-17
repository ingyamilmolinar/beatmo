package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func TestAutoGainForPeak(t *testing.T) {
	cases := []struct {
		name string
		peak float64
		want float64
	}{
		{"silence", 0.0, 1.0},
		{"near-silence", 0.0005, 1.0},
		{"loud-full-scale", 1.0, 1.0},
		{"half", 0.45, 2.0},
		{"quiet", 0.09, 10.0},
		{"tiny-clamped", 0.001, 16.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := autoGainForPeak(c.peak)
			if math.Abs(got-c.want) > 1e-6 {
				t.Fatalf("autoGainForPeak(%v) = %v, want %v", c.peak, got, c.want)
			}
		})
	}
}

func TestAdvanceWavePeak(t *testing.T) {
	if got := advanceWavePeak(0.1, 0.8); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("attack: got %v, want 0.8", got)
	}
	got := advanceWavePeak(0.8, 0.0)
	if got >= 0.8 || got <= 0.0 {
		t.Fatalf("release: got %v, want between 0 and 0.8 (exclusive)", got)
	}
	if math.Abs(got-0.8*waveAutoGainRelease) > 1e-9 {
		t.Fatalf("release: got %v, want %v", got, 0.8*waveAutoGainRelease)
	}
}

func TestWaveEdgeAmplitude(t *testing.T) {
	if got := waveEdgeAmplitude(4.0); math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("waveEdgeAmplitude(4) = %v, want 0.25", got)
	}
	if got := waveEdgeAmplitude(1.0); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("waveEdgeAmplitude(1) = %v, want 1.0", got)
	}
}

// "AG" (not "AUTO") — the auto-gain concept carries ONE label across every
// tab (Chain's AG pill, Wave's pill + badge). 2026-07-04 label unification.
func TestWaveGainBadgeText(t *testing.T) {
	if got := waveGainBadgeText(4.2, true); got != "AG x4.2" {
		t.Fatalf("auto badge = %q, want %q", got, "AG x4.2")
	}
	if got := waveGainBadgeText(2.0, false); got != "x2.0" {
		t.Fatalf("manual badge = %q, want %q", got, "x2.0")
	}
}

func TestWaveWindowPeak(t *testing.T) {
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.0, -0.3, 0.5, -0.2}}
	if got := waveWindowPeak(ch, nil); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("waveWindowPeak = %v, want 0.5", got)
	}
}

func TestDrawAnalyzerWaveformGainAcceptsParams(t *testing.T) {
	img := newTrackedImage("waveTest", 200, 80)
	defer releaseImage(img)
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: make([]float64, 256)}
	for i := range ch.Waveform {
		ch.Waveform[i] = 0.1 // quiet
	}
	drawAnalyzerWaveform(img, img.Bounds(), ch, nil, nil, 8.0, true, false)
}
