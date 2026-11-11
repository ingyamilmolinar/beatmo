package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const (
	mfccFFTSize    = 16384
	mfccNumFilters = 26
)

// hzToMel converts a frequency in Hz to the mel scale.
func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

// melToHz converts a mel value back to Hz.
func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}

// buildMelFilterbank returns a [mfccNumFilters][numBins] triangular filter bank
// evenly spaced in mel between 0 Hz and sr/2 Hz.
func buildMelFilterbank(numBins int, binHz float64, sr int) [mfccNumFilters][]float64 {
	nyquist := float64(sr) / 2.0
	melLow := hzToMel(0.0)
	melHigh := hzToMel(nyquist)

	// mfccNumFilters+2 points evenly spaced in mel: edges + filter centres.
	nPoints := mfccNumFilters + 2
	melPoints := make([]float64, nPoints)
	for i := range melPoints {
		melPoints[i] = melLow + float64(i)*(melHigh-melLow)/float64(nPoints-1)
	}

	// Convert mel points to bin indices.
	binPoints := make([]int, nPoints)
	for i, m := range melPoints {
		hz := melToHz(m)
		binPoints[i] = int(math.Round(hz / binHz))
		if binPoints[i] >= numBins {
			binPoints[i] = numBins - 1
		}
	}

	var filters [mfccNumFilters][]float64
	for m := 0; m < mfccNumFilters; m++ {
		f := make([]float64, numBins)
		left := binPoints[m]
		center := binPoints[m+1]
		right := binPoints[m+2]

		// Rising slope: left → center
		if center > left {
			for k := left; k <= center; k++ {
				f[k] = float64(k-left) / float64(center-left)
			}
		}
		// Falling slope: center → right
		if right > center {
			for k := center; k <= right; k++ {
				f[k] = float64(right-k) / float64(right-center)
			}
		}
		filters[m] = f
	}
	return filters
}

// dctII computes the DCT-II of in and writes to out (same length assumed).
// out[k] = sum_{n=0}^{N-1} in[n] * cos(pi/N * (n+0.5) * k)
func dctII(in []float64, out []float64) {
	N := len(in)
	piOverN := math.Pi / float64(N)
	for k := range out {
		sum := 0.0
		for n, v := range in {
			sum += v * math.Cos(piOverN*float64(k)*(float64(n)+0.5))
		}
		out[k] = sum
	}
}

// computeMFCC returns the first n MFCCs (n<=13) of w's sustain window: power
// spectrum (from wave.MagnitudeSpectrum) → mel filterbank (26 triangular
// filters, 0..sr/2) → log energy per filter → DCT-II, keep first n. Always
// returns a [13]float64 (coeffs >= n are zero).
func computeMFCC(w wave.Wave, n int) [13]float64 {
	if n > 13 {
		n = 13
	}

	// Sustain window: shared policy via sustainWindow.
	seg := sustainWindow(w)

	var result [13]float64
	if len(seg.Samples) < 4 || seg.SampleRate == 0 {
		return result
	}

	// Power spectrum via shared MagnitudeSpectrum.
	mag, binHz := wave.MagnitudeSpectrum(seg, mfccFFTSize, wave.WindowHann)
	numBins := len(mag)

	// Build mel filterbank.
	filters := buildMelFilterbank(numBins, binHz, seg.SampleRate)

	// Apply filterbank: sum power * triangle weight per filter, then log.
	const eps = 1e-10
	logEnergies := make([]float64, mfccNumFilters)
	for m := 0; m < mfccNumFilters; m++ {
		energy := 0.0
		f := filters[m]
		for k, weight := range f {
			if weight > 0 && k < numBins {
				energy += mag[k] * mag[k] * weight
			}
		}
		if energy < eps {
			energy = eps
		}
		logEnergies[m] = math.Log(energy)
	}

	// DCT-II of log energies → keep first n coefficients.
	dctOut := make([]float64, mfccNumFilters)
	dctII(logEnergies, dctOut)

	for i := 0; i < n && i < mfccNumFilters; i++ {
		result[i] = dctOut[i]
	}
	return result
}
