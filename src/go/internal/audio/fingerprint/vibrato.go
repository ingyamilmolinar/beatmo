package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// VibratoFingerprint holds the result of a vibrato analysis pass.
type VibratoFingerprint struct {
	RateHz      float64 // dominant modulation rate of the F0 track (Hz)
	ExtentCents float64 // half the (p95−p5) peak-to-peak pitch deviation, in cents
	Jitter      float64 // cycle-to-cycle rate irregularity (std of period / mean period)
	AMDepth     float64 // relative amplitude-modulation depth synchronous with vibrato
	OnsetSec    float64 // time from window start until vibrato reaches ~half sustained extent
	Detected    bool    // band power > 3× noise floor AND ExtentCents > 2
}

// computeVibrato estimates vibrato parameters from a sustained window w,
// given the nominal fundamental f0 and analysis config cfg.
//
// Algorithm:
//  1. STFT (2048 / 512 hop) → per-frame F0 by parabolic interpolation in a
//     ±10% window around f0.
//  2. Convert F0 trajectory to cents about the median F0.
//  3. Detrend the cents track (remove linear drift via OLS).
//  4. Estimate RateHz by DFT of the detrended track restricted to 3–9 Hz.
//  5. ExtentCents = (p95 − p5) / 2 of the cents track.
//  6. Jitter = std(cycle periods) / mean(cycle periods) from zero-crossings.
//  7. AMDepth = std(frameRMS) / mean(frameRMS) over the track.
//  8. OnsetSec = first frame where a short rolling amplitude exceeds half the
//     sustained amplitude of the cents oscillation.
//  9. Detected = 3–9 Hz band power > 3× out-of-band noise floor AND ExtentCents > 2.
func computeVibrato(w wave.Wave, f0 float64, cfg AnalysisConfig) VibratoFingerprint {
	const windowSize = 2048
	const hopSize = 512

	var zero VibratoFingerprint
	if len(w.Samples) == 0 || w.SampleRate == 0 || f0 <= 0 {
		return zero
	}

	sr := w.SampleRate
	half := windowSize / 2
	binHz := float64(sr) / float64(windowSize)

	// ---- Build STFT frames ------------------------------------------------
	type frameData struct {
		f0Est float64 // interpolated fundamental (Hz)
		rms   float64 // frame RMS
	}
	var frames []frameData

	searchBins := int(0.1*f0/binHz + 1)
	if searchBins < 3 {
		searchBins = 3
	}
	targetBin := int(f0 / binHz)

	samples := w.Samples
	for start := 0; start+windowSize <= len(samples); start += hopSize {
		seg := samples[start : start+windowSize]
		fw := wave.Wave{Samples: seg, SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(fw, windowSize, wave.WindowHann)
		if len(mag) < half {
			continue
		}

		// Per-frame RMS.
		rmsSum := 0.0
		for _, s := range seg {
			rmsSum += s * s
		}
		rms := math.Sqrt(rmsSum / float64(windowSize))

		// Find peak bin near f0.
		lo := targetBin - searchBins
		hi := targetBin + searchBins
		if lo < 1 {
			lo = 1
		}
		if hi >= half-1 {
			hi = half - 2
		}
		peakBin := lo
		peakAmp := mag[lo]
		for b := lo + 1; b <= hi; b++ {
			if mag[b] > peakAmp {
				peakAmp = mag[b]
				peakBin = b
			}
		}

		// Parabolic interpolation.
		var binF float64
		if peakBin > 0 && peakBin < half-1 {
			alpha, beta, gamma := mag[peakBin-1], mag[peakBin], mag[peakBin+1]
			denom := alpha - 2*beta + gamma
			if denom < -1e-12 {
				shift := -0.5 * (gamma - alpha) / denom
				if shift < -0.5 {
					shift = -0.5
				} else if shift > 0.5 {
					shift = 0.5
				}
				binF = float64(peakBin) + shift
			} else {
				binF = float64(peakBin)
			}
		} else {
			binF = float64(peakBin)
		}
		frames = append(frames, frameData{f0Est: binF * binHz, rms: rms})
	}

	numFrames := len(frames)
	if numFrames < 8 {
		return zero
	}

	secPerFrame := float64(hopSize) / float64(sr)
	frameRate := float64(sr) / float64(hopSize)

	// ---- Convert F0 track to cents about median ---------------------------
	f0Vals := make([]float64, numFrames)
	for i, fd := range frames {
		f0Vals[i] = fd.f0Est
	}
	medianF0 := median(f0Vals)
	if medianF0 < 1 {
		medianF0 = f0
	}

	cents := make([]float64, numFrames)
	for i, fd := range frames {
		if fd.f0Est > 0 {
			cents[i] = 1200 * math.Log2(fd.f0Est/medianF0)
		}
	}

	// ---- Detrend (remove linear drift) ------------------------------------
	xs := make([]float64, numFrames)
	for i := range xs {
		xs[i] = float64(i) * secPerFrame
	}
	slope := olsSlope(xs, cents)
	cMean := 0.0
	for _, c := range cents {
		cMean += c
	}
	cMean /= float64(numFrames)
	detrended := make([]float64, numFrames)
	for i, c := range cents {
		detrended[i] = c - (cMean + slope*xs[i])
	}

	// ---- Estimate vibrato rate by DFT over 3–9 Hz -------------------------
	bestPow := 0.0
	bestFreq := 0.0
	for hz := 3.0; hz <= 9.0; hz += 0.05 {
		re, im := 0.0, 0.0
		for i, v := range detrended {
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

	// Noise floor: max power at 1–2 Hz (below vibrato range).
	noisePow := 0.0
	for hz := 1.0; hz <= 2.0; hz += 0.05 {
		re, im := 0.0, 0.0
		for i, v := range detrended {
			angle := 2 * math.Pi * hz * float64(i) / frameRate
			re += v * math.Cos(angle)
			im += v * math.Sin(angle)
		}
		pow := re*re + im*im
		if pow > noisePow {
			noisePow = pow
		}
	}

	// ---- ExtentCents = (p95 − p5) / 2 of the DETRENDED cents track ---------
	// Using the detrended track removes slow frequency drift and FFT-bin
	// quantization bias, preventing steady-tone extent inflation.
	detrendedSorted := make([]float64, len(detrended))
	copy(detrendedSorted, detrended)
	sort.Float64s(detrendedSorted)
	p5 := percentile(detrendedSorted, 5)
	p95 := percentile(detrendedSorted, 95)
	extentCents := (p95 - p5) / 2

	// ---- Jitter from zero-crossings ---------------------------------------
	jitter := 0.0
	periods := zeroCrossingPeriods(detrended, frameRate)
	if len(periods) >= 2 {
		meanP, stdP := meanStd(periods)
		if meanP > 1e-9 {
			jitter = stdP / meanP
		}
	}

	// ---- AMDepth = std(frameRMS) / mean(frameRMS) -------------------------
	rmsVals := make([]float64, numFrames)
	for i, fd := range frames {
		rmsVals[i] = fd.rms
	}
	amDepth := 0.0
	meanRMS, stdRMS := meanStd(rmsVals)
	if meanRMS > 1e-12 {
		amDepth = stdRMS / meanRMS
	}

	// ---- OnsetSec: first frame where rolling extent exceeds half sustained extent ---
	onsetSec := 0.0
	if extentCents > 0 {
		halfExtent := extentCents * 0.5
		// Rolling window of ~0.2s (or at least 3 frames).
		rollN := int(0.2 * frameRate)
		if rollN < 3 {
			rollN = 3
		}
		if rollN > numFrames {
			rollN = numFrames
		}
		for fi := rollN - 1; fi < numFrames; fi++ {
			// Compute local extent (p95-p5)/2 over rolling window.
			window := detrended[fi-rollN+1 : fi+1]
			win := make([]float64, len(window))
			copy(win, window)
			sort.Float64s(win)
			lp5 := percentile(win, 5)
			lp95 := percentile(win, 95)
			localExtent := (lp95 - lp5) / 2
			if localExtent >= halfExtent {
				onsetSec = float64(fi-rollN+1) * secPerFrame
				break
			}
		}
	}

	// ---- Detected ----------------------------------------------------------
	detected := bestPow > 3*noisePow && extentCents > 2

	return VibratoFingerprint{
		RateHz:      bestFreq,
		ExtentCents: extentCents,
		Jitter:      jitter,
		AMDepth:     amDepth,
		OnsetSec:    onsetSec,
		Detected:    detected,
	}
}

// median returns the median of a slice (does not modify the input).
func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]float64, len(vals))
	copy(cp, vals)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 0 {
		return (cp[n/2-1] + cp[n/2]) / 2
	}
	return cp[n/2]
}

// percentile returns the p-th percentile (0–100) of a pre-sorted slice.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := p / 100 * float64(n-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// meanStd returns the mean and standard deviation of vals.
func meanStd(vals []float64) (mean, std float64) {
	n := len(vals)
	if n == 0 {
		return 0, 0
	}
	for _, v := range vals {
		mean += v
	}
	mean /= float64(n)
	for _, v := range vals {
		d := v - mean
		std += d * d
	}
	if n > 1 {
		std = math.Sqrt(std / float64(n-1))
	} else {
		std = 0
	}
	return mean, std
}

// zeroCrossingPeriods finds positive-going zero-crossings in signal and
// returns the time intervals (in seconds, using frameRate) between them.
func zeroCrossingPeriods(signal []float64, frameRate float64) []float64 {
	var crossings []int
	for i := 1; i < len(signal); i++ {
		if signal[i-1] <= 0 && signal[i] > 0 {
			crossings = append(crossings, i)
		}
	}
	if len(crossings) < 2 {
		return nil
	}
	periods := make([]float64, len(crossings)-1)
	for i := range periods {
		periods[i] = float64(crossings[i+1]-crossings[i]) / frameRate
	}
	return periods
}
