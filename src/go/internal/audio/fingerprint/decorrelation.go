package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Is each partial one locked sine or a living band? Phase-locked partials read as artificial; a real harmonic is a
// narrow band of detuned, randomly phased components. Measured as each partial's width and how independently
// partials move.

const (
	decorrFFTSize         = 32768
	decorrMainLobeBins    = 4.0 // Hann main lobe, -3 dB full width ≈ 1.44 bins; use 4 as the "resolution floor" to subtract
	decorrMinSharedFrames = 8
	decorrMaxModes        = 8

	// Bounds the walk from a mode's bin: on a noisy shelf the descent would run past the neighbour or past 2·Hz, where
	// the cents conversion divides by a non-positive number and Log2 yields NaN. Also clamped to half the distance to the
	// nearest other mode, so one peak never reports a neighbour's width.
	decorrMaxWalkBinsCap = 64
)

// Against the nearest OTHER mode in the full slice: one beyond decorrMaxModes is still a spectral neighbour.
func maxWalkBins(modes []Mode, i int, binHz float64) int {
	nearest := math.Inf(1)
	for j := range modes {
		if j == i {
			continue
		}
		if d := math.Abs(modes[j].Hz - modes[i].Hz); d < nearest {
			nearest = d
		}
	}
	bound := decorrMaxWalkBinsCap
	if !math.IsInf(nearest, 1) {
		if half := int(nearest / binHz / 2); half < bound {
			bound = half
		}
	}
	if bound < 0 {
		bound = 0
	}
	return bound
}

func PartialBandwidthCents(w wave.Wave, modes []Mode) (float64, bool) {
	if len(modes) < 2 {
		return 0, false
	}
	seg := sustainWindow(w)
	if seg.Len() < decorrFFTSize {
		return 0, false
	}
	mag, binHz := wave.MagnitudeSpectrum(seg, decorrFFTSize, wave.WindowHann)
	var sum float64
	n := 0
	for i, m := range modes {
		if i >= decorrMaxModes {
			break
		}
		c := int(m.Hz/binHz + 0.5)
		if c < 2 || c >= len(mag)-2 {
			continue
		}
		walk := maxWalkBins(modes, i, binHz)

		// Bounded, so it cannot walk onto a neighbour's peak.
		steps := 0
		for c > 1 && mag[c-1] > mag[c] {
			if steps >= walk {
				break
			}
			c--
			steps++
		}
		steps = 0
		for c < len(mag)-2 && mag[c+1] > mag[c] {
			if steps >= walk {
				break
			}
			c++
			steps++
		}

		half := mag[c] / math.Sqrt2
		lo, hi := c, c
		loHitBound, hiHitBound := false, false
		steps = 0
		for lo > 0 && mag[lo] > half {
			if steps >= walk {
				loHitBound = true
				break
			}
			lo--
			steps++
		}
		steps = 0
		for hi < len(mag)-1 && mag[hi] > half {
			if steps >= walk {
				hiHitBound = true
				break
			}
			hi++
			steps++
		}
		if loHitBound || hiHitBound {
			// Not reached within the bound: unresolvable, not "very wide".
			continue
		}

		widthBins := float64(hi-lo) - decorrMainLobeBins
		if widthBins < 0 {
			widthBins = 0
		}
		widthHz := widthBins * binHz
		if widthHz >= 2*m.Hz {
			// A width of 2× the frequency would make the log undefined: unresolvable instead.
			continue
		}
		sum += 1200 * math.Log2((m.Hz+widthHz/2)/(m.Hz-widthHz/2))
		n++
	}
	if n < 2 {
		return 0, false
	}
	return sum / float64(n), true
}

// Clamped to [0, 1]: a negative mean correlation is finite-sample noise, not extra aliveness.
func PartialIndependence(modes []Mode) (float64, bool) {
	if len(modes) < 2 {
		return 0, false
	}
	if len(modes) > decorrMaxModes {
		modes = modes[:decorrMaxModes]
	}
	var sum float64
	pairs := 0
	for i := 0; i < len(modes); i++ {
		for j := i + 1; j < len(modes); j++ {
			r, ok := pearsonShared(modes[i].Track, modes[j].Track)
			if ok {
				sum += r
				pairs++
			}
		}
	}
	if pairs == 0 {
		return 0, false
	}
	v := 1 - sum/float64(pairs)
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return v, true
}

// After removing each track's own trend: a shared decay (every plucked partial decays) is not correlation.
func pearsonShared(a, b []float64) (float64, bool) {
	var xs, ya, yb []float64
	for f := range a {
		if f < len(b) && !math.IsNaN(a[f]) && !math.IsNaN(b[f]) {
			xs = append(xs, float64(f))
			ya = append(ya, a[f])
			yb = append(yb, b[f])
		}
	}
	if len(xs) < decorrMinSharedFrames {
		return 0, false
	}
	da, db := detrend(xs, ya), detrend(xs, yb)
	var sab, saa, sbb float64
	for i := range da {
		sab += da[i] * db[i]
		saa += da[i] * da[i]
		sbb += db[i] * db[i]
	}
	// BOTH flat: the partials move together, so r = 1 is honest. ONE flat: the correlation is undefined, and calling it 1
	// asserts perfect lock that PartialIndependence would average in; the pair is skipped, and if every pair is skipped
	// the measurement is unavailable.
	aFlat, bFlat := saa <= 1e-12, sbb <= 1e-12
	if aFlat && bFlat {
		return 1, true
	}
	if aFlat || bFlat {
		return 0, false
	}
	return sab / math.Sqrt(saa*sbb), true
}

// So a shared decay or swell does not read as correlation.
func detrend(xs, ys []float64) []float64 {
	slope := olsSlope(xs, ys)
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(len(xs))
	my /= float64(len(ys))
	out := make([]float64, len(ys))
	for i := range ys {
		out[i] = ys[i] - my - slope*(xs[i]-mx)
	}
	return out
}
