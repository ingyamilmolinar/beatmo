package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Skip 0.3 s, take to 1.3 s; the whole wave when under 4 samples remain. Every analyser shares it so all see the
// identical window.
func sustainWindow(w wave.Wave) wave.Wave {
	seg := w.Slice(SustainSkipSec*1000, SustainLenSec*1000)
	if len(seg.Samples) < 4 {
		return w
	}
	return seg
}

// 0 for fewer than 2 points or ~0 x-variance.
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

// A silent wave is returned unchanged.
func PeakNormalize(w wave.Wave) wave.Wave {
	if w.PeakSample() == 0 {
		return w
	}
	return wave.Chain(&wave.WaveSource{W: w}, wave.NormalizeTransform(0))
}

// Raw when it already has <= nPoints frames or nPoints <= 0.
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

// The whole wave when it is shorter than the window.
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
