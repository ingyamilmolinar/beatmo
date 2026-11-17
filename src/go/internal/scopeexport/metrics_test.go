package scopeexport

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestDownsampleMinMax(t *testing.T) {
	// 8 samples, downsample to 4 bins.
	samples := []float64{0.1, 0.5, -0.2, 0.3, 0.0, -0.8, 0.7, 0.4}
	result := downsampleMinMax(samples, 4)

	if len(result) != 4 {
		t.Fatalf("expected 4 bins, got %d", len(result))
	}
	// Bin 0: samples[0:2] = [0.1, 0.5] → min=0.1, max=0.5
	if result[0][0] != 0.1 || result[0][1] != 0.5 {
		t.Errorf("bin 0: expected [0.1, 0.5], got %v", result[0])
	}
	// Bin 1: samples[2:4] = [-0.2, 0.3] → min=-0.2, max=0.3
	if result[1][0] != -0.2 || result[1][1] != 0.3 {
		t.Errorf("bin 1: expected [-0.2, 0.3], got %v", result[1])
	}
	// Bin 2: samples[4:6] = [0.0, -0.8] → min=-0.8, max=0.0
	if result[2][0] != -0.8 || result[2][1] != 0.0 {
		t.Errorf("bin 2: expected [-0.8, 0.0], got %v", result[2])
	}
	// Bin 3: samples[6:8] = [0.7, 0.4] → min=0.4, max=0.7
	if result[3][0] != 0.4 || result[3][1] != 0.7 {
		t.Errorf("bin 3: expected [0.4, 0.7], got %v", result[3])
	}
}

func TestDownsampleEmpty(t *testing.T) {
	if result := downsampleMinMax(nil, 64); result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
	if result := downsampleMinMax([]float64{1.0}, 0); result != nil {
		t.Errorf("expected nil for 0 bins, got %v", result)
	}
}

func TestTopNBins(t *testing.T) {
	obs := wave.Observation{
		Bins:     []float64{-96.0, -10.0, -20.0, -5.0, -30.0}, // DC=-96, 1=-10, 2=-20, 3=-5, 4=-30
		FreqBins: []float64{0, 43.0, 86.0, 129.0, 172.0},
	}
	result := topNBins(obs, 2)

	if len(result) != 2 {
		t.Fatalf("expected 2 bins, got %d", len(result))
	}
	// Top 1: index 3 (db=-5, hz=129.0)
	if result[0].Hz != 129.0 || result[0].DB != -5.0 {
		t.Errorf("top 1: expected {129.0, -5.0}, got %v", result[0])
	}
	// Top 2: index 1 (db=-10, hz=43.0)
	if result[1].Hz != 43.0 || result[1].DB != -10.0 {
		t.Errorf("top 2: expected {43.0, -10.0}, got %v", result[1])
	}
}

func TestComputeStageMetrics(t *testing.T) {
	// Generate a 440Hz sine wave at 44100Hz for 1024 samples.
	sr := 44100
	n := 1024
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440.0 * float64(i) / float64(sr))
	}

	peakObs := wave.NewPeakRMSObserver()
	fftObs := wave.NewFFTObserver(1024)
	zcObs := wave.NewZeroCrossingObserver()

	metrics := computeStageMetrics(samples, sr, 64, 16, peakObs, fftObs, zcObs)

	// Peak should be near 0 dB (sine wave with amplitude 1.0).
	if metrics.PeakDB < -1.0 || metrics.PeakDB > 0.1 {
		t.Errorf("expected peak near 0 dB, got %.2f", metrics.PeakDB)
	}
	// RMS of sine = 1/sqrt(2) ≈ -3.01 dB.
	if metrics.RMSDB < -4.0 || metrics.RMSDB > -2.0 {
		t.Errorf("expected RMS near -3 dB, got %.2f", metrics.RMSDB)
	}
	// No clipping.
	if metrics.ClipCount != 0 {
		t.Errorf("expected 0 clips, got %d", metrics.ClipCount)
	}
	// Waveform should have 64 bins.
	if len(metrics.Waveform64) != 64 {
		t.Errorf("expected 64 waveform bins, got %d", len(metrics.Waveform64))
	}
	// FFT top should have 16 bins.
	if len(metrics.FFTTop) != 16 {
		t.Errorf("expected 16 FFT bins, got %d", len(metrics.FFTTop))
	}
	// Top FFT bin should be near 440 Hz.
	if len(metrics.FFTTop) > 0 {
		topHz := metrics.FFTTop[0].Hz
		if topHz < 400 || topHz > 480 {
			t.Errorf("expected top FFT bin near 440 Hz, got %.1f Hz", topHz)
		}
	}
	// Should have zero crossings.
	if metrics.ZeroCrossings == 0 {
		t.Error("expected non-zero zero crossings for sine wave")
	}
}
