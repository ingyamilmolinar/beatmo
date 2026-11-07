package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Exported so the game analyser shares the exact scalar the matcher optimises. DC skipped; 0 for an empty spectrum.
func SpectralCentroid(mag []float64, binHz float64) float64 {
	weightedSum := 0.0
	totalMag := 0.0
	for i := 1; i < len(mag); i++ {
		hz := float64(i) * binHz
		weightedSum += hz * mag[i]
		totalMag += mag[i]
	}
	if totalMag < 1e-12 {
		return 0
	}
	return weightedSum / totalMag
}

// fp.F0Hz must already be set; the whole wave when the window is under 4 samples.
func computeSpectral(w wave.Wave, fp *Fingerprint) {
	// Sustain window: skip attack, analyze sustained material.
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return
	}

	mag, binHz := wave.MagnitudeSpectrum(seg, 16384, wave.WindowHann)
	// A fixed 16384 FFT, so ref and cand slices are always equal length.
	fp.SustainMag = mag
	half := len(mag)
	f0 := fp.F0Hz

	// Scaled with harmonic number so vibrato-smeared upper partials are not missed.
	const partialTolFrac = 0.013 // fraction of F0 per harmonic
	if f0 > 0 {
		for k := 0; k < 16; k++ {
			harmonicNum := float64(k + 1)
			targetHz := f0 * harmonicNum
			targetBin := int(targetHz / binHz)
			halfWidthHz := math.Max(1.5, harmonicNum*partialTolFrac*f0)
			searchBins := int(halfWidthHz/binHz) + 1
			if searchBins < 3 {
				searchBins = 3
			}
			lo := targetBin - searchBins
			hi := targetBin + searchBins
			if lo < 0 {
				lo = 0
			}
			if hi >= half {
				hi = half - 1
			}
			localMax := 0.0
			for i := lo; i <= hi; i++ {
				if mag[i] > localMax {
					localMax = mag[i]
				}
			}
			fp.Partials[k] = localMax
		}
		// Normalize so Partials[0]=1.
		if fp.Partials[0] > 1e-12 {
			inv := 1.0 / fp.Partials[0]
			for i := range fp.Partials {
				fp.Partials[i] *= inv
			}
		}
	}

	// ---- Spectral centroid via exported helper ----------------------------
	fp.SpectralCentroid = SpectralCentroid(mag, binHz)

	// ---- Spectral rolloff (85% cumulative energy) -------------------------
	totalEnergy := 0.0
	for i := 1; i < half; i++ {
		totalEnergy += mag[i] * mag[i]
	}
	cumEnergy := 0.0
	rolloffDone := false
	for i := 1; i < half; i++ {
		cumEnergy += mag[i] * mag[i]
		if !rolloffDone && cumEnergy >= 0.85*totalEnergy {
			fp.SpectralRolloff = float64(i) * binHz
			rolloffDone = true
		}
	}

	// partials[k] is partial k+1, so even partials sit at odd k.
	if f0 > 0 {
		var evenSum, oddSum float64
		for k := 1; k < 16; k++ {
			partialNum := k + 1
			if partialNum%2 == 0 {
				evenSum += fp.Partials[k]
			} else {
				oddSum += fp.Partials[k]
			}
		}
		if oddSum > 1e-12 {
			fp.EvenOddRatio = evenSum / oddSum
		}
	}

	// ---- Inharmonicity ----------------------------------------------------
	if f0 > 0 {
		// Compute mean+1.5σ threshold for significance check.
		sum, sum2 := 0.0, 0.0
		for _, m := range mag {
			sum += m
			sum2 += m * m
		}
		mean := sum / float64(half)
		variance := sum2/float64(half) - mean*mean
		if variance < 0 {
			variance = 0
		}
		threshold := mean + 1.5*math.Sqrt(variance)

		var inharmSum float64
		inharmN := 0
		for k := 1; k < 16; k++ {
			harmonicNumI := float64(k + 1)
			targetHz := f0 * harmonicNumI
			targetBin := int(targetHz / binHz)
			halfWidthHzI := math.Max(1.5, harmonicNumI*partialTolFrac*f0)
			searchBinsI := int(halfWidthHzI/binHz) + 1
			if searchBinsI < 3 {
				searchBinsI = 3
			}
			lo := targetBin - searchBinsI
			hi := targetBin + searchBinsI
			if lo < 0 {
				lo = 0
			}
			if hi >= half {
				hi = half - 1
			}
			// The threshold below blocks measuredHz when no real peak is found, so the default is never read for an empty band.
			bestBin := targetBin
			bestAmp := 0.0
			for i := lo; i <= hi; i++ {
				if mag[i] > bestAmp {
					bestAmp = mag[i]
					bestBin = i
				}
			}
			if bestAmp > threshold*0.1 {
				measuredHz := float64(bestBin) * binHz
				inharmSum += math.Abs(measuredHz-targetHz) / targetHz
				inharmN++
			}
		}
		if inharmN > 0 {
			fp.Inharmonicity = inharmSum / float64(inharmN)
		}
	}

	// ---- Partial ratios (string-vs-bar/tube discriminator) ----------------
	fp.PartialRatioProfile = AnalyzePartialRatios(mag, binHz, f0)

	// ---- Noise ratio -------------------------------------------------------
	if f0 > 0 && totalEnergy > 1e-12 {
		var harmonicEnergy float64
		for k := 0; k < 16; k++ {
			targetBin := int(f0*float64(k+1)/binHz + 0.5)
			for delta := -1; delta <= 1; delta++ {
				b := targetBin + delta
				if b >= 0 && b < half {
					harmonicEnergy += mag[b] * mag[b]
				}
			}
		}
		fp.NoiseRatio = 1.0 - harmonicEnergy/totalEnergy
		if fp.NoiseRatio < 0 {
			fp.NoiseRatio = 0
		}
	}
}
