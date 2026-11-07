package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// A scalar cannot tell a slow bow-stroke SHAPE from fast chaos with the same std/mean; the envelope's spectrum can.

const (
	modMinFrames = 32
	modLoHz      = 0.5
	modHiHz      = 30.0
)

var modBandEdgesHz = [4]float64{0.5, 2, 8, 30}

type ModSpectrum struct {
	RateHz   float64    // dominant rate in 0.5–30 Hz
	Depth    float64    // power at the dominant bin / total 0.5–30 Hz power, times the track's coefficient of variation
	Bands    [3]float64 // energy shares: gesture 0.5–2, vibrato 2–8, warble 8–30
	Flatness float64    // spectral flatness of the 0.5–30 Hz modulation spectrum
	OK       bool

	// ZERO usable bins in 0.5–30 Hz (bin width wider than the band): distinct from too few frames, so the report can name
	// "resolution collapse".
	resolutionCollapsed bool

	// The OLS detrend removes only a line and a decay is not one, so every decaying note leaks curvature into the
	// lowest bin and its "rate" pins there; the RATE row reports N/A. Depth, Bands and Flatness describe the whole
	// spectrum's shape and stay.
	rateAtFloorBin bool
}

func ModulationSpectrum(track []float64, hopSec float64) ModSpectrum {
	var out ModSpectrum
	if len(track) < modMinFrames || hopSec <= 0 {
		return out
	}
	// So a swell or decay does not read as sub-hertz modulation; the line is centred on mean(xs), the true least-squares
	// centre.
	xs := make([]float64, len(track))
	mean := 0.0
	meanX := 0.0
	for i, v := range track {
		xs[i] = float64(i) * hopSec
		mean += v
		meanX += xs[i]
	}
	mean /= float64(len(track))
	meanX /= float64(len(xs))
	if mean <= 1e-9 {
		return out
	}
	slope := olsSlope(xs, track)
	det := make([]float64, len(track))
	var ss float64
	for i, v := range track {
		det[i] = (v - mean - slope*(xs[i]-meanX)) / mean
		ss += det[i] * det[i]
	}
	cv := math.Sqrt(ss / float64(len(det)))

	frameRate := 1 / hopSec
	n := 1
	for n < len(det) {
		n <<= 1
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: det, SampleRate: int(math.Round(frameRate))}, n, wave.WindowHann)
	// CEILING: a floored lo admits a bin below 0.5 Hz into total/peak while the bucketing excludes it, so Bands stops
	// summing to 1 and a sub-0.5 Hz artefact can win RateHz.
	lo, hi := int(math.Ceil(modLoHz/binHz)), int(modHiHz/binHz)
	if hi >= len(mag) {
		hi = len(mag) - 1
	}
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		out.resolutionCollapsed = true
		return out
	}
	var total float64
	peak, pk := 0.0, lo
	for k := lo; k <= hi; k++ {
		p := mag[k] * mag[k]
		total += p
		if p > peak {
			peak, pk = p, k
		}
		f := float64(k) * binHz
		for b := 0; b < 3; b++ {
			if f >= modBandEdgesHz[b] && f < modBandEdgesHz[b+1] {
				out.Bands[b] += p
			}
		}
	}
	if total <= 0 {
		return out
	}
	for b := range out.Bands {
		out.Bands[b] /= total
	}
	out.RateHz = float64(pk) * binHz
	out.rateAtFloorBin = pk == lo
	out.Depth = (peak / total) * cv
	out.Flatness = spectralFlatnessOf(mag[lo : hi+1])
	out.OK = true
	return out
}
