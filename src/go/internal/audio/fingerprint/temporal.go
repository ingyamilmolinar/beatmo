package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TemporalFingerprint holds STFT-based temporal metrics that capture how
// the spectrum evolves over time — key for distinguishing organic instruments
// from static synthesized tones.
type TemporalFingerprint struct {
	NumFrames         int
	CentroidMean      float64
	CentroidSlope     float64    // Hz/sec (linear regression)
	CentroidRange     float64    // max-min
	HarmonicFluxMean  float64
	SpectralVariance  float64
	PartialDecaySlopes [6]float64 // dB/sec
	VibratoDetected   bool
	VibratoRateHz     float64
	VibratoDepthCents float64
}

// computeTemporal builds the temporal fingerprint from w (sustain region) using
// the given fundamental for harmonic tracking.
//
// Port of computeTemporalFingerprint from cmd/synth-analyze/main.go, adapted to
// use wave.MagnitudeSpectrum for each STFT frame instead of a private FFT.
func computeTemporal(w wave.Wave, f0 float64) *TemporalFingerprint {
	const windowSize = 2048
	const hopSize = 512

	sr := w.SampleRate
	if sr == 0 {
		sr = 44100
	}

	// Skip first 0.1s attack.
	skipSamples := int(0.1 * float64(sr))
	if skipSamples >= len(w.Samples) {
		skipSamples = 0
	}
	analysis := w.Samples[skipSamples:]

	// Compute STFT frames via wave.MagnitudeSpectrum.
	// MagnitudeSpectrum returns n/2+1 bins; we use indices [0, half] where half=n/2.
	// The reference uses half=windowSize/2=1024 bins; MagnitudeSpectrum gives 1025.
	// We index up to half (exclusive upper bound) to match the reference exactly.
	half := windowSize / 2 // 1024 — matches reference's mag[:half]

	var frames [][]float64
	for start := 0; start+windowSize <= len(analysis); start += hopSize {
		seg := analysis[start : start+windowSize]
		frameWave := wave.Wave{Samples: seg, SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(frameWave, windowSize, wave.WindowHann)
		// Trim to half bins to match reference behaviour (discard Nyquist bin).
		frame := make([]float64, half)
		copy(frame, mag[:half])
		frames = append(frames, frame)
	}

	numFrames := len(frames)
	tf := &TemporalFingerprint{
		NumFrames: numFrames,
	}

	if numFrames == 0 {
		return tf
	}

	binHz := float64(sr) / float64(windowSize)
	secPerFrame := float64(hopSize) / float64(sr)

	// ---- CentroidTrajectory ------------------------------------------------
	centroidTraj := make([]float64, numFrames)
	for fi, mag := range frames {
		sumMag := 0.0
		weightedSum := 0.0
		for i, m := range mag {
			hz := float64(i) * binHz
			sumMag += m
			weightedSum += hz * m
		}
		if sumMag > 1e-12 {
			centroidTraj[fi] = weightedSum / sumMag
		}
	}

	// CentroidMean, CentroidRange, CentroidSlope.
	cMin, cMax := centroidTraj[0], centroidTraj[0]
	cSum := 0.0
	xs := make([]float64, numFrames)
	for i, c := range centroidTraj {
		xs[i] = float64(i) * secPerFrame
		cSum += c
		if c < cMin {
			cMin = c
		}
		if c > cMax {
			cMax = c
		}
	}
	tf.CentroidMean = cSum / float64(numFrames)
	tf.CentroidRange = cMax - cMin
	tf.CentroidSlope = olsSlope(xs, centroidTraj)

	// ---- HarmonicFluxMean --------------------------------------------------
	// Mean L1 norm of frame-to-frame magnitude difference, normalized by energy
	// of the earlier frame.
	if numFrames >= 2 {
		fluxSum := 0.0
		fluxCount := 0
		for fi := 1; fi < numFrames; fi++ {
			prev := frames[fi-1]
			curr := frames[fi]
			prevEnergy := 0.0
			l1Diff := 0.0
			for b := 0; b < half; b++ {
				prevEnergy += prev[b]
				l1Diff += math.Abs(curr[b] - prev[b])
			}
			if prevEnergy > 1e-12 {
				fluxSum += l1Diff / prevEnergy
				fluxCount++
			}
		}
		if fluxCount > 0 {
			tf.HarmonicFluxMean = fluxSum / float64(fluxCount)
		}
	}

	// ---- SpectralVariance --------------------------------------------------
	// Mean spectrum across frames, then mean variance per bin across frames.
	meanSpec := make([]float64, half)
	for _, mag := range frames {
		for b, m := range mag {
			meanSpec[b] += m
		}
	}
	for b := range meanSpec {
		meanSpec[b] /= float64(numFrames)
	}
	varSum := 0.0
	for _, mag := range frames {
		for b, m := range mag {
			d := m - meanSpec[b]
			varSum += d * d
		}
	}
	tf.SpectralVariance = varSum / float64(numFrames*half)

	// ---- PartialDecaySlopes -----------------------------------------------
	// For each of the first 6 harmonics, find the bin nearest k*f0Hz,
	// collect amplitude per frame, fit linear regression in dB/sec.
	if f0 > 1 {
		for k := 0; k < 6; k++ {
			targetHz := f0 * float64(k+1)
			bin := int(targetHz/binHz + 0.5)
			if bin < 0 {
				bin = 0
			}
			if bin >= half {
				bin = half - 1
			}
			ys := make([]float64, numFrames)
			for fi, mag := range frames {
				v := mag[bin]
				if v < 1e-9 {
					v = 1e-9
				}
				ys[fi] = 20 * math.Log10(v)
			}
			tf.PartialDecaySlopes[k] = olsSlope(xs, ys)
		}
	}

	// ---- Vibrato detection -------------------------------------------------
	// Track the fundamental bin's instantaneous frequency per frame via
	// parabolic interpolation of peak near f0Hz, yielding an f0 trajectory.
	// Then DFT of that trajectory; look for a significant peak in 3–9 Hz.
	if f0 > 1 && numFrames >= 8 {
		searchBins := int(0.1*f0/binHz + 1) // ±10% of f0
		if searchBins < 3 {
			searchBins = 3
		}
		targetBin := int(f0 / binHz)
		f0Traj := make([]float64, numFrames)
		for fi, mag := range frames {
			lo := targetBin - searchBins
			hi := targetBin + searchBins
			if lo < 1 {
				lo = 1
			}
			if hi >= half-1 {
				hi = half - 2
			}
			// Find local peak.
			peakBin := lo
			peakAmp := mag[lo]
			for b := lo + 1; b <= hi; b++ {
				if mag[b] > peakAmp {
					peakAmp = mag[b]
					peakBin = b
				}
			}
			// Parabolic interpolation (concave-down guard).
			// denom = alpha - 2*beta + gamma < 0 for a true local maximum.
			// If denom >= 0 the parabola is concave-up (not a proper peak);
			// fall back to the integer bin in that case.
			if peakBin > 0 && peakBin < half-1 {
				alpha, beta, gamma := mag[peakBin-1], mag[peakBin], mag[peakBin+1]
				denom := alpha - 2*beta + gamma
				var binF float64
				if denom < -1e-12 {
					binF = float64(peakBin) - 0.5*(gamma-alpha)/denom
					// Clamp to ±0.5 bin around the peak (valid interpolation range).
					if binF < float64(peakBin)-0.5 {
						binF = float64(peakBin) - 0.5
					} else if binF > float64(peakBin)+0.5 {
						binF = float64(peakBin) + 0.5
					}
				} else {
					binF = float64(peakBin)
				}
				f0Traj[fi] = binF * binHz
			} else {
				f0Traj[fi] = float64(peakBin) * binHz
			}
		}

		// DFT of f0 trajectory to find vibrato rate.
		// Frame rate = sr / hopSize.
		frameRate := float64(sr) / float64(hopSize)

		// Remove mean (DC).
		trajMean := 0.0
		for _, v := range f0Traj {
			trajMean += v
		}
		trajMean /= float64(numFrames)
		centered := make([]float64, numFrames)
		for i, v := range f0Traj {
			centered[i] = v - trajMean
		}

		// DFT over the vibrato range of interest (3-9 Hz).
		bestPow := 0.0
		bestFreq := 0.0
		for hz := 3.0; hz <= 9.0; hz += 0.1 {
			re, im := 0.0, 0.0
			for i, v := range centered {
				angle := 2 * math.Pi * hz * float64(i) / frameRate
				re += v * math.Cos(angle)
				im += v * math.Sin(angle)
			}
			pow := re*re + im*im
			if pow > bestPow {
				bestPow = pow
				bestFreq = hz
			}
		}

		// Compute max power at a non-vibrato frequency for significance test.
		// Check 1-2 Hz (below vibrato range) for comparison.
		noisePow := 0.0
		for hz := 1.0; hz <= 2.0; hz += 0.1 {
			re, im := 0.0, 0.0
			for i, v := range centered {
				angle := 2 * math.Pi * hz * float64(i) / frameRate
				re += v * math.Cos(angle)
				im += v * math.Sin(angle)
			}
			pow := re*re + im*im
			if pow > noisePow {
				noisePow = pow
			}
		}

		// Vibrato depth: peak-to-peak deviation of f0 trajectory / 2, in cents.
		f0Min, f0Max := f0Traj[0], f0Traj[0]
		for _, v := range f0Traj {
			if v < f0Min {
				f0Min = v
			}
			if v > f0Max {
				f0Max = v
			}
		}
		if f0Min > 1 {
			tf.VibratoDepthCents = 1200 * math.Log2(f0Max/f0Min) / 2
		}

		// Significant if vibrato power > 3x noise floor and depth > 2 cents.
		if bestPow > 3*noisePow && tf.VibratoDepthCents > 2 {
			tf.VibratoDetected = true
			tf.VibratoRateHz = bestFreq
		}
	}

	return tf
}
