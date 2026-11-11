package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// decorrelation.go — is each partial a single locked sine, or a living band?
//
// AGENTS.synth.instruments.md records the PADsynth argument verbatim:
// phase-locked partials read as artificial; a real harmonic is a narrow band
// of slightly detuned, randomly phased components. Two measurements make that
// checkable: how wide each partial is, and how independently the partials
// move.

const (
	decorrFFTSize         = 32768
	decorrMainLobeBins    = 4.0 // Hann main lobe, -3 dB full width ≈ 1.44 bins; use 4 as the "resolution floor" to subtract
	decorrMinSharedFrames = 8
	decorrMaxModes        = 8

	// decorrMaxWalkBinsCap bounds how far, in fine (32768-pt) bins, the
	// hill-climb and the -3 dB descent are allowed to travel from a mode's
	// nominal bin. Without a cap, a mode sitting on a noisy shelf (no local
	// minimum before the spectrum floor) walks the descent arbitrarily far —
	// even past the neighbouring mode, or past 2*m.Hz, at which point the
	// width-to-cents conversion divides by a non-positive number and
	// math.Log2 silently returns NaN. The cap is also clamped to at most
	// half the fine-bin distance to the NEAREST OTHER mode: a peak's -3 dB
	// skirt must never be allowed to cross into a neighbour's peak, or two
	// close partials would each report the other's width.
	decorrMaxWalkBinsCap = 64
)

// maxWalkBins bounds the hill-climb/half-max walk for modes[i]: at most
// decorrMaxWalkBinsCap fine bins, and never more than half the fine-bin
// distance to the nearest OTHER mode in the full modes slice (not just the
// ones this call will measure — a neighbour beyond decorrMaxModes is still a
// real spectral neighbour to stay clear of).
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

		// climb to the local max near the mode, bounded so it cannot walk
		// onto a neighbouring mode's peak.
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
			// The -3 dB point was never reached within the bound (a noisy
			// shelf, or the neighbour-clearance cap) — this mode's width is
			// not resolvable, not "very wide". Skip rather than fabricate.
			continue
		}

		widthBins := float64(hi-lo) - decorrMainLobeBins
		if widthBins < 0 {
			widthBins = 0
		}
		widthHz := widthBins * binHz
		if widthHz >= 2*m.Hz {
			// A width at or past 2x the mode's own frequency makes
			// (m.Hz - widthHz/2) non-positive, which would poison
			// math.Log2 with a NaN or an undefined logarithm — treat it as
			// unresolvable instead of fabricating a value.
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

// PartialIndependence returns 1 minus the mean pairwise Pearson correlation
// of the strong partials' detrended dB tracks, clamped to [0, 1]: a
// negative mean correlation is finite-sample noise (nothing in the model
// means "more independent than independent"), not a signal of extra
// aliveness, so it is folded back to 1 rather than reported as-is.
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

// pearsonShared correlates two dB tracks over the frames where both are
// present, after removing each one's own OLS trend (a shared decay must not
// read as correlation — every partial of a plucked note decays).
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
	// A flat residual has no variance for a correlation to be defined
	// against, so the two cases must be separated.
	//
	// BOTH flat: the two partials really do move together -- neither
	// deviates from its own trend at all -- so r = 1 ("perfectly locked") is
	// the honest reading.
	//
	// ONE flat: the correlation is undefined (a zero denominator), and
	// calling it 1 asserts perfect lock between a partial that moves and one
	// that does not. That is not a weak claim, it is a WRONG one, and
	// PartialIndependence averages it in as though it were measured -- so a
	// single perfectly-steady synth partial dragged the whole independence
	// score toward "locked" no matter what the others were doing. The pair
	// is skipped instead; if every pair is skipped, PartialIndependence
	// reports unavailable, which is the correct answer.
	aFlat, bFlat := saa <= 1e-12, sbb <= 1e-12
	if aFlat && bFlat {
		return 1, true
	}
	if aFlat || bFlat {
		return 0, false
	}
	return sab / math.Sqrt(saa*sbb), true
}

// detrend removes the OLS line (removing the mean and slope over xs) from
// ys, so a shared decay or swell does not read as correlation between two
// otherwise-independent tracks.
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
