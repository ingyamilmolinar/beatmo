package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// sustainWindow extracts the shared analysis window from w: skip the first
// SustainSkipSec (0.3 s) of attack transient and take up to SustainLenSec
// (1.3 s), yielding a 1.0 s sustained window. Falls back to the whole wave
// when fewer than 4 samples remain after slicing (e.g. short test signals).
// Used by all five analyzers (computeSpectral, computeShape, computeMFCC,
// DetectF0, computeHNR) so they all operate on the identical window.
func sustainWindow(w wave.Wave) wave.Wave {
	seg := w.Slice(SustainSkipSec*1000, SustainLenSec*1000)
	if len(seg.Samples) < 4 {
		return w
	}
	return seg
}

// olsSlope returns the ordinary-least-squares slope for paired xs and ys.
// Returns 0 when fewer than 2 points are provided or the x-variance is ~0.
// This is the single canonical OLS helper shared by envelope.go (decay slope)
// and temporal.go (centroid/partial-decay slopes).
func olsSlope(xs, ys []float64) float64 {
	n := len(xs)
	if n < 2 || n != len(ys) {
		return 0
	}
	fn := float64(n)
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}
	denom := fn*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0
	}
	return (fn*sumXY - sumX*sumY) / denom
}

// PeakNormalize normalizes the wave's peak amplitude to 0 dBFS (peak = 1.0).
// If the wave is silent (peak == 0), it is returned unchanged.
func PeakNormalize(w wave.Wave) wave.Wave {
	if w.PeakSample() == 0 {
		return w
	}
	return wave.Chain(&wave.WaveSource{W: w}, wave.NormalizeTransform(0))
}

// RMSEnvelope computes a hop-based RMS envelope from w, then downsamples to
// at most nPoints values. Each hop window is [i, i+hop) of samples; the RMS
// is sqrt(mean(x^2)) over the window. If the number of raw envelope frames is
// already <= nPoints (or nPoints <= 0), the raw envelope is returned as-is.
func RMSEnvelope(w wave.Wave, hop, nPoints int) []float64 {
	if hop < 1 {
		hop = 256
	}
	buf := w.Samples
	env := make([]float64, 0, len(buf)/hop+1)
	for i := 0; i < len(buf); i += hop {
		end := i + hop
		if end > len(buf) {
			end = len(buf)
		}
		sum := 0.0
		for _, s := range buf[i:end] {
			sum += s * s
		}
		env = append(env, math.Sqrt(sum/float64(end-i)))
	}
	// Downsample to nPoints.
	if nPoints <= 0 || len(env) <= nPoints {
		return env
	}
	out := make([]float64, nPoints)
	for i := range out {
		idx := int(float64(i) * float64(len(env)) / float64(nPoints))
		if idx >= len(env) {
			idx = len(env) - 1
		}
		out[i] = env[idx]
	}
	return out
}

// AutoSegment finds the loudest sustained window of durSec seconds in w.
// It slides a window of that length across the wave with a hop of 128 samples
// and returns the sub-wave with the highest RMS. If w is shorter than the
// requested window, the entire wave is returned.
func AutoSegment(w wave.Wave, durSec float64) wave.Wave {
	sr := w.SampleRate
	if sr == 0 {
		return w
	}
	buf := w.Samples
	winSamples := int(float64(sr) * durSec)
	if winSamples >= len(buf) {
		return w
	}
	const hopSize = 128
	bestIdx := 0
	bestRMS := 0.0
	for start := 0; start+winSamples <= len(buf); start += hopSize {
		end := start + winSamples
		sum := 0.0
		for _, s := range buf[start:end] {
			sum += s * s
		}
		rms := math.Sqrt(sum / float64(winSamples))
		if rms > bestRMS {
			bestRMS = rms
			bestIdx = start
		}
	}
	end := bestIdx + winSamples
	if end > len(buf) {
		end = len(buf)
	}
	out := make([]float64, end-bestIdx)
	copy(out, buf[bestIdx:end])
	return wave.Wave{Samples: out, SampleRate: sr, Label: w.Label}
}
