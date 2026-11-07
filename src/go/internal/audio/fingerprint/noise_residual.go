package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Harmonic versus residual (bow noise, broadband) content.
type NoiseProfile struct {
	ResidualRatio      float64   // residual energy / total energy, clamped to [0,1]
	ResidualCentroidHz float64   // spectral centroid of the residual (non-harmonic) spectrum
	ResidualBandLevels []float64 // residual energy per cfg.Bands band
}

// A 16384-point spectrum: energy within ±searchBins of each k·f0 is harmonic, the rest residual (ratio, centroid,
// per-band levels). f0 == 0 or silent input returns zeros with one level per band.
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

	// The same tolerance as spectral.go.
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
