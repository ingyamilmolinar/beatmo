package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// modulation.go — the modulation spectrum of a frame-wise track.
//
// The program spec's failure mode 2: the violin's amplitude variation was
// tuned to std/mean 0.40 against a real 0.64 and still sounded wrong, because
// the real signal is a slow bow-stroke SHAPE and the synth's was fast chaos.
// A scalar cannot see the difference; the spectrum of the envelope can.

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

	// resolutionCollapsed is set only when the hop/frame-count combination
	// leaves ZERO usable bins inside 0.5–30 Hz (bin width wider than the
	// whole analysis band). This is a distinct failure from "fewer than
	// modMinFrames frames" -- callers past modApplies's own frame-count gate
	// still hit it, so registry_modulation.go reads it to report an accurate
	// reason ("resolution collapse") instead of reusing the frame-count
	// message that gate already ruled out.
	resolutionCollapsed bool

	// rateAtFloorBin is set when the dominant bin IS the lowest usable bin
	// (lo). Every decaying note leaks residual envelope curvature into that
	// bin -- the OLS detrend can only remove a straight LINE, and an
	// exponential decay is not one -- so the "dominant rate" of a plain
	// struck note pins to the bottom of the band and reports whatever
	// modLoHz/binHz happens to round to. That is a measurement of the
	// analysis window, not of a modulation, so registry_modulation.go turns
	// it into an honest N/A on the RATE row. Depth/Bands/Flatness stay
	// available: they describe the shape of the whole spectrum and remain
	// meaningful even when its peak sits at the floor.
	rateAtFloorBin bool
}

func ModulationSpectrum(track []float64, hopSec float64) ModSpectrum {
	var out ModSpectrum
	if len(track) < modMinFrames || hopSec <= 0 {
		return out
	}
	// Detrend: remove mean and OLS line so a swell or decay does not read as
	// a sub-hertz modulation; then normalise by the mean so depth is relative.
	// The OLS line is centred on mean(xs) (not the middle SAMPLE index) so it
	// is the actual least-squares centre of mass of the time axis, matching
	// what olsSlope itself assumes.
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
	// lo must CEILING to modLoHz/binHz, not truncate: a floored lo can admit
	// one bin strictly BELOW 0.5 Hz into total/peak while the band-bucketing
	// loop below correctly excludes it (its f < modBandEdgesHz[0]), so Bands
	// silently stops summing to 1 and a sub-0.5Hz artifact (e.g. residual
	// curvature from an imperfectly-detrended decay) can win RateHz outright.
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
