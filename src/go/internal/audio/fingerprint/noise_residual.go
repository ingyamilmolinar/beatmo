package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// NoiseProfile holds metrics that separate harmonic (tonal) content from
// residual (bow-noise / broadband) content in a sustained signal.
type NoiseProfile struct {
	ResidualRatio      float64   // residual energy / total energy, clamped to [0,1]
	ResidualCentroidHz float64   // spectral centroid of the residual (non-harmonic) spectrum
	ResidualBandLevels []float64 // residual energy per cfg.Bands band
}

// computeNoiseProfile estimates how much of w's energy is non-harmonic (residual).
//
// Algorithm:
//  1. Compute the magnitude spectrum of w via a 16384-point Hann-windowed FFT.
//  2. Sum magnitude² in narrow windows (±searchBins) around each harmonic k*f0
//     (k = 1..up to Nyquist) to obtain the harmonic energy.
//  3. ResidualRatio = (totalEnergy - harmonicEnergy) / totalEnergy, clamped [0,1].
//  4. Build a residual magnitude array by zeroing harmonic bins in a copy of mag.
//  5. ResidualCentroidHz = spectral centroid of the residual magnitude array.
//  6. ResidualBandLevels = per-cfg.Bands energy of the residual magnitude² array.
//
// f0 == 0 or a silent/empty input returns a zero NoiseProfile with
// ResidualBandLevels of length len(cfg.Bands).
func computeNoiseProfile(w wave.Wave, f0 float64, cfg AnalysisConfig) NoiseProfile {
	zero := NoiseProfile{ResidualBandLevels: make([]float64, len(cfg.Bands))}

	if len(w.Samples) < 4 || f0 <= 0 {
		return zero
	}

	const fftSize = 16384
	mag, binHz := wave.MagnitudeSpectrum(w, fftSize, wave.WindowHann)
	if binHz <= 0 || len(mag) == 0 {
		return zero
	}
	half := len(mag)

	// ---- Total energy (skip DC bin 0) ----------------------------------------
	totalEnergy := 0.0
	for i := 1; i < half; i++ {
		totalEnergy += mag[i] * mag[i]
	}
	if totalEnergy < 1e-12 {
		return zero
	}

	// ---- Harmonic energy + build residual magnitude array ----------------------
	// Use the same tolerance as spectral.go (partialTolFrac = 0.013 * k * f0).
	const partialTolFrac = 0.013

	residualMag := make([]float64, half)
	copy(residualMag, mag)

	harmonicEnergy := 0.0
	nyquist := float64(w.SampleRate) / 2.0
	for k := 1; ; k++ {
		targetHz := f0 * float64(k)
		if targetHz > nyquist {
			break
		}
		harmonicNum := float64(k)
		halfWidthHz := math.Max(1.5, harmonicNum*partialTolFrac*f0)
		searchBins := int(halfWidthHz/binHz) + 1
		if searchBins < 3 {
			searchBins = 3
		}
		targetBin := int(targetHz/binHz + 0.5)
		lo := targetBin - searchBins
		hi := targetBin + searchBins
		if lo < 0 {
			lo = 0
		}
		if hi >= half {
			hi = half - 1
		}
		for i := lo; i <= hi; i++ {
			harmonicEnergy += mag[i] * mag[i]
			residualMag[i] = 0
		}
	}

	// ---- ResidualRatio --------------------------------------------------------
	residualRatio := (totalEnergy - harmonicEnergy) / totalEnergy
	if residualRatio < 0 {
		residualRatio = 0
	}
	if residualRatio > 1 {
		residualRatio = 1
	}

	// ---- ResidualCentroidHz ---------------------------------------------------
	residualCentroid := SpectralCentroid(residualMag, binHz)

	// ---- ResidualBandLevels ---------------------------------------------------
	residualBandLevels := BandEnergies(residualMag, binHz, cfg.Bands)

	return NoiseProfile{
		ResidualRatio:      residualRatio,
		ResidualCentroidHz: residualCentroid,
		ResidualBandLevels: residualBandLevels,
	}
}
