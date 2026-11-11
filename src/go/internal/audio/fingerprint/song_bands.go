package fingerprint

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// BandEnergies sums magnitude² within each band's [LoHz,HiHz). One value per
// band, in band order. Bins outside all bands are ignored.
func BandEnergies(mag []float64, binHz float64, bands []BandEdge) []float64 {
	out := make([]float64, len(bands))
	if binHz <= 0 {
		return out
	}
	for i, b := range bands {
		lo := int(b.LoHz / binHz)
		hi := int(b.HiHz / binHz)
		if lo < 0 {
			lo = 0
		}
		if hi >= len(mag) {
			hi = len(mag) - 1
		}
		sum := 0.0
		for k := lo; k <= hi; k++ {
			sum += mag[k] * mag[k]
		}
		out[i] = sum
	}
	return out
}

// TonalPercussiveRatio estimates tonality as the fraction of spectral energy
// concentrated in local peaks. A pure tone puts nearly all energy in one peak
// (→~1); broadband noise spreads energy across bins (→ low). Deterministic.
func TonalPercussiveRatio(w wave.Wave, cfg AnalysisConfig) float64 {
	mag, _ := wave.MagnitudeSpectrum(w, cfg.FFTSize, wave.WindowHann)
	total := 0.0
	peak := 0.0
	for i := 1; i < len(mag)-1; i++ {
		e := mag[i] * mag[i]
		total += e
		// Local-maximum bins count as "tonal" energy.
		if mag[i] >= mag[i-1] && mag[i] >= mag[i+1] {
			peak += e
		}
	}
	if total <= 0 {
		return 0
	}
	return peak / total
}
