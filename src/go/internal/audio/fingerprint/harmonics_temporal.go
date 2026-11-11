package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// HarmonicTrajectory captures per-harmonic amplitude evolution over the note
// window. Harmonics are indexed at k-1 (H1 → index 0, H2 → index 1, …).
// Arrays are fixed at [16]; unused slots (index >= N) remain zero.
type HarmonicTrajectory struct {
	N               int         // number of tracked harmonics (min(16, nyquist/f0))
	AttackRate      [16]float64 // OLS dB/sec for each harmonic's pre-peak amplitude ramp
	PeakTimeSec     [16]float64 // time of each harmonic's amplitude peak (sec from window start)
	SustainLevel    [16]float64 // median amplitude of middle third, normalized to H1 peak
	DecayRate       [16]float64 // OLS dB/sec post-peak (negative = decaying)
	EvenOddOverTime []float64   // per-frame sum(even harmonics)/sum(odd harmonics)
}

// computeHarmonicTrajectory builds a HarmonicTrajectory from the given wave
// (which should already be the sustain window), tracking up to 16 harmonics of
// f0 via a 2048-sample / 512-hop STFT using wave.MagnitudeSpectrum.
//
// Algorithm:
//  1. STFT the window (2048/512, Hann).
//  2. N = min(16, int(sr/2/f0)).
//  3. For each harmonic k (1..N), read the magnitude at the nearest bin to k*f0
//     in every frame → per-harmonic amplitude tracks.
//  4. PeakTimeSec[k-1]  = argmax(track) * hopSec.
//  5. AttackRate[k-1]   = OLS dB/sec over pre-peak frames.
//  6. DecayRate[k-1]    = OLS dB/sec over post-peak frames (should be ≤ 0).
//  7. SustainLevel[k-1] = median(middle-third of track) / H1Peak.
//  8. EvenOddOverTime   = sum(even-harmonic mags) / sum(odd-harmonic mags) per frame.
func computeHarmonicTrajectory(w wave.Wave, f0 float64, _ AnalysisConfig) HarmonicTrajectory {
	const windowSize = 2048
	const hopSize = 512

	sr := w.SampleRate
	if sr == 0 {
		sr = 44100
	}
	half := windowSize / 2 // 1024 bins (discard Nyquist)

	// Guard: no usable f0 or too-short signal.
	if f0 <= 0 || len(w.Samples) < windowSize {
		return HarmonicTrajectory{}
	}

	// Determine N = min(16, floor(nyquist / f0)).
	nyquist := float64(sr) / 2.0
	n := int(nyquist / f0)
	if n > 16 {
		n = 16
	}
	if n < 1 {
		return HarmonicTrajectory{}
	}

	binHz := float64(sr) / float64(windowSize)
	hopSec := float64(hopSize) / float64(sr)

	// Build STFT frames.
	var frames [][]float64
	samples := w.Samples
	for start := 0; start+windowSize <= len(samples); start += hopSize {
		seg := samples[start : start+windowSize]
		frameWave := wave.Wave{Samples: seg, SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(frameWave, windowSize, wave.WindowHann)
		frame := make([]float64, half)
		copy(frame, mag[:half])
		frames = append(frames, frame)
	}

	numFrames := len(frames)
	if numFrames == 0 {
		return HarmonicTrajectory{N: n}
	}

	// For each harmonic k (1..N), extract the amplitude track.
	// harmonicBin[k] = nearest bin index to k*f0.
	harmonicBin := make([]int, n+1) // index 0 unused
	for k := 1; k <= n; k++ {
		bin := int(float64(k)*f0/binHz + 0.5)
		if bin >= half {
			bin = half - 1
		}
		harmonicBin[k] = bin
	}

	// tracks[k-1] = amplitude per frame for harmonic k.
	tracks := make([][]float64, n)
	for k := 1; k <= n; k++ {
		bin := harmonicBin[k]
		tr := make([]float64, numFrames)
		for fi, mag := range frames {
			tr[fi] = mag[bin]
		}
		tracks[k-1] = tr
	}

	// Compute H1 peak for SustainLevel normalization.
	h1Peak := 0.0
	for _, v := range tracks[0] {
		if v > h1Peak {
			h1Peak = v
		}
	}
	if h1Peak < 1e-12 {
		// Silent or near-silent: return zero struct.
		return HarmonicTrajectory{N: n}
	}

	// Build frame-time axis (seconds).
	xs := make([]float64, numFrames)
	for fi := range xs {
		xs[fi] = float64(fi) * hopSec
	}

	var traj HarmonicTrajectory
	traj.N = n

	for k := 0; k < n; k++ {
		tr := tracks[k]

		// --- PeakTimeSec: frame with maximum amplitude.
		peakFi := 0
		peakAmp := tr[0]
		for fi, v := range tr {
			if v > peakAmp {
				peakAmp = v
				peakFi = fi
			}
		}
		traj.PeakTimeSec[k] = float64(peakFi) * hopSec

		// --- AttackRate: OLS dB/sec over frames [0..peakFi].
		if peakFi >= 1 {
			xAttack := xs[:peakFi+1]
			yAttack := toDBTrack(tr[:peakFi+1])
			traj.AttackRate[k] = olsSlope(xAttack, yAttack)
		}

		// --- DecayRate: OLS dB/sec over frames [peakFi..end].
		if peakFi < numFrames-1 {
			xDecay := xs[peakFi:]
			yDecay := toDBTrack(tr[peakFi:])
			traj.DecayRate[k] = olsSlope(xDecay, yDecay)
		}

		// --- SustainLevel: median of middle-third, normalized to H1 peak.
		third := numFrames / 3
		if third < 1 {
			third = 1
		}
		midStart := third
		midEnd := 2 * third
		if midEnd > numFrames {
			midEnd = numFrames
		}
		midSlice := make([]float64, midEnd-midStart)
		copy(midSlice, tr[midStart:midEnd])
		traj.SustainLevel[k] = medianFloat64(midSlice) / h1Peak
	}

	// --- EvenOddOverTime: per-frame ratio of even vs odd harmonic amplitudes.
	// Harmonic k is "even" if k%2==0 (H2, H4, …); "odd" if k%2==1 (H1, H3, …).
	traj.EvenOddOverTime = make([]float64, numFrames)
	for fi, mag := range frames {
		evenSum := 0.0
		oddSum := 0.0
		for k := 1; k <= n; k++ {
			bin := harmonicBin[k]
			v := mag[bin]
			if k%2 == 0 {
				evenSum += v
			} else {
				oddSum += v
			}
		}
		if oddSum > 1e-12 {
			traj.EvenOddOverTime[fi] = evenSum / oddSum
		}
		// If oddSum ≈ 0, leave the ratio as 0 (guard divide-by-zero).
	}

	return traj
}

// toDBTrack converts a linear amplitude track to dB, flooring at -120 dBFS
// (1e-6) to avoid log10(0).
func toDBTrack(tr []float64) []float64 {
	out := make([]float64, len(tr))
	for i, v := range tr {
		if v < 1e-6 {
			v = 1e-6
		}
		out[i] = 20 * math.Log10(v)
	}
	return out
}

// medianFloat64 returns the median of vs (sorts a copy; does not modify vs).
// Returns 0 for an empty slice. Named to avoid conflict with vibrato.go:median.
func medianFloat64(vs []float64) float64 {
	n := len(vs)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, vs)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}
