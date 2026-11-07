package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// How strongly brightness tracks loudness, plus the centroid's modulation depth and rate: bowed strings, brass and
// voice brighten as they get louder; an organ barely couples.
type DynamicCoupling struct {
	CentroidRMSCorr     float64 // Pearson corr between per-frame centroid and per-frame RMS
	BrightnessModDepth  float64 // std(centroid)/mean(centroid) over sustain frames
	BrightnessModRateHz float64 // dominant modulation rate of the detrended centroid (0-15 Hz)
	// Over the whole input wave (normalised, not sustain-windowed): the whole-note swell when given the whole note,
	// window-local and weaker on a clip. CentroidRMSCorr is the steady-sustain micro coupling.
	MacroCentroidRMSCorr float64
}

// w is already the sustain window; the zero struct for silent or too-short input.
func computeCoupling(w wave.Wave, cfg AnalysisConfig) DynamicCoupling {
	const windowSize = 2048
	const hopSize = 512

	sr := w.SampleRate
	if sr == 0 {
		sr = 44100
	}

	// Build per-frame spectral centroid and RMS series.
	half := windowSize / 2 // 1024 bins (discard Nyquist), matches temporal.go
	binHz := float64(sr) / float64(windowSize)

	var centroids []float64
	var rmsVals []float64

	for start := 0; start+windowSize <= len(w.Samples); start += hopSize {
		seg := w.Samples[start : start+windowSize]
		frameWave := wave.Wave{Samples: seg, SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(frameWave, windowSize, wave.WindowHann)

		// Trim to half bins to match temporal.go convention.
		frameMag := mag[:half]

		// Per-frame RMS.
		sumSq := 0.0
		for _, s := range seg {
			sumSq += s * s
		}
		rms := math.Sqrt(sumSq / float64(windowSize))

		// Per-frame spectral centroid (Hz), skipping DC bin i=0.
		centroid := SpectralCentroid(frameMag, binHz)

		centroids = append(centroids, centroid)
		rmsVals = append(rmsVals, rms)
	}

	n := len(centroids)
	if n < 2 {
		return DynamicCoupling{}
	}

	// Guard silent input: if all RMS values are near-zero, return zero struct.
	maxRMS := 0.0
	for _, r := range rmsVals {
		if r > maxRMS {
			maxRMS = r
		}
	}
	if maxRMS < 1e-12 {
		return DynamicCoupling{}
	}

	var dc DynamicCoupling

	// Below this coefficient of variation a series is constant and the correlation is numerical noise, so an organ reads 0.
	const minCV = 0.005 // 0.5 % relative variation required in each series
	cMeanForCV, cStdForCV := meanStd(centroids)
	rMean, rStdVal := meanStd(rmsVals)
	cCV := 0.0
	if cMeanForCV > 1e-12 {
		cCV = cStdForCV / cMeanForCV
	}
	rCV := 0.0
	if rMean > 1e-12 {
		rCV = rStdVal / rMean
	}
	if cCV < minCV || rCV < minCV {
		// One or both series are nearly constant — coupling is 0.
		dc.CentroidRMSCorr = 0
	} else {
		dc.CentroidRMSCorr = pearsonCorr(centroids, rmsVals)
	}

	cMean := cMeanForCV
	cStd := cStdForCV
	if cMean > 1e-12 {
		dc.BrightnessModDepth = cStd / cMean
	}

	// The centroid series is detrended first.
	frameRate := float64(sr) / float64(hopSize) // frames per second
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i) / frameRate
	}
	slope := olsSlope(xs, centroids)
	detrended := make([]float64, n)
	for i, c := range centroids {
		detrended[i] = c - (cMean + slope*(xs[i]-xs[n/2]))
	}
	// A near-constant centroid has no modulation rate; dominantFreq would pick one from the noise floor.
	if cCV >= minCV {
		dc.BrightnessModRateHz = dominantFreq(detrended, frameRate, 0, 15)
	}

	return dc
}

// 0 when either series is constant (CV below minCV) or there are fewer than 2 frames.
func centroidRMSCorrFromSamples(w wave.Wave) float64 {
	const windowSize = 2048
	const hopSize = 512
	const minCV = 0.005

	sr := w.SampleRate
	if sr == 0 {
		sr = 44100
	}
	half := windowSize / 2
	binHz := float64(sr) / float64(windowSize)

	var centroids []float64
	var rmsVals []float64

	for start := 0; start+windowSize <= len(w.Samples); start += hopSize {
		seg := w.Samples[start : start+windowSize]
		frameWave := wave.Wave{Samples: seg, SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(frameWave, windowSize, wave.WindowHann)
		frameMag := mag[:half]

		sumSq := 0.0
		for _, s := range seg {
			sumSq += s * s
		}
		rms := math.Sqrt(sumSq / float64(windowSize))
		centroid := SpectralCentroid(frameMag, binHz)

		centroids = append(centroids, centroid)
		rmsVals = append(rmsVals, rms)
	}

	n := len(centroids)
	if n < 2 {
		return 0
	}

	// Guard: silent input.
	maxRMS := 0.0
	for _, r := range rmsVals {
		if r > maxRMS {
			maxRMS = r
		}
	}
	if maxRMS < 1e-12 {
		return 0
	}

	// CV guard: near-constant series → correlation is numerical noise.
	cMean, cStd := meanStd(centroids)
	rMean, rStd := meanStd(rmsVals)
	cCV := 0.0
	if cMean > 1e-12 {
		cCV = cStd / cMean
	}
	rCV := 0.0
	if rMean > 1e-12 {
		rCV = rStd / rMean
	}
	if cCV < minCV || rCV < minCV {
		return 0
	}
	return pearsonCorr(centroids, rmsVals)
}

// Over the ENTIRE wave, attack to decay, not a sustain window.
func WholeSignalCentroidRMSCorr(w wave.Wave, _ AnalysisConfig) float64 {
	return centroidRMSCorrFromSamples(w)
}

// 0 for zero variance or fewer than 2 elements.
func pearsonCorr(xs, ys []float64) float64 {
	n := len(xs)
	if n < 2 || n != len(ys) {
		return 0
	}
	fn := float64(n)
	sumX, sumY, sumXY, sumX2, sumY2 := 0.0, 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}
	num := fn*sumXY - sumX*sumY
	denX := fn*sumX2 - sumX*sumX
	denY := fn*sumY2 - sumY*sumY
	denom := math.Sqrt(denX * denY)
	if denom < 1e-12 {
		return 0
	}
	r := num / denom
	// Clamp to [-1, 1] to handle floating-point rounding.
	if r > 1 {
		r = 1
	} else if r < -1 {
		r = -1
	}
	return r
}

// A DFT at 0.1 Hz steps; 0 when flat or loHz >= hiHz.
func dominantFreq(series []float64, frameRate, loHz, hiHz float64) float64 {
	n := len(series)
	if n < 4 || loHz >= hiHz {
		return 0
	}

	bestPow := 0.0
	bestFreq := 0.0

	for hz := loHz; hz <= hiHz; hz += 0.1 {
		re, im := 0.0, 0.0
		for i, v := range series {
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

	// The best power must clearly beat a near-DC reference, or the series is flat.
	refRe, refIm := 0.0, 0.0
	for i, v := range series {
		angle := 2 * math.Pi * 0.05 * float64(i) / frameRate
		refRe += v * math.Cos(angle)
		refIm += v * math.Sin(angle)
	}
	refPow := refRe*refRe + refIm*refIm

	// Require the peak to be at least 2× the DC reference to be meaningful.
	if bestPow < 2*refPow {
		return 0
	}

	return bestFreq
}
