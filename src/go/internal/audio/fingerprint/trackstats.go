package fingerprint

import (
	"math"
	"sort"
)

// trackstats.go — ONE statistic set, applied uniformly to every frame-wise
// track.
//
// This is what turns "measure the whole signal" from a pile of bespoke
// scalars into a mechanical cross-product. Rather than hand-writing
// CentroidMean, CentroidRange and CentroidSlope (which TemporalFingerprint
// does today) and then hand-writing the equivalents for RMS, flatness, noise
// fraction and sixteen harmonic tracks, every track gets the same reduction.
//
// It also subsumes existing scalars: TemporalFingerprint.CentroidMean,
// .CentroidRange and .CentroidSlope are exactly StatsOf(tracks.Centroid)'s
// Mean, Range and Slope. That overlap is deliberate and is what the Phase 6
// orthogonality analysis is for -- it will show which of the legacy scalars
// are now redundant, with evidence rather than by assertion.

// TrackStats is the uniform reduction of one frame-wise track.
//
// The set is chosen so that MAGNITUDE and SHAPE are separable. That
// distinction is the whole point: the violin's amplitude variation was tuned
// to std/mean 0.40 against a real 0.64 and still sounded wrong, because the
// real signal's variation is a slow bow-stroke SHAPE while the synth's was
// fast chaos. Std alone cannot tell those apart. Slope, Curvature and
// MonotonicFrac describe the shape; Mean, Std, IQR and Range describe the
// magnitude. (Separating RATE as well needs the modulation spectrum, which is
// a later phase.)
type TrackStats struct {
	Mean   float64
	Median float64
	Std    float64
	IQR    float64 // p75 - p25: spread that a single outlier frame cannot inflate
	P10    float64
	P90    float64
	Min    float64
	Max    float64
	Range  float64 // Max - Min

	// Slope is the OLS trend in units per SECOND (not per frame), so it is
	// comparable across analyses at different hop sizes.
	Slope float64

	// Curvature is the quadratic coefficient of a second-order least-squares
	// fit, in units per second squared. It separates a steady ramp (≈0) from
	// an accelerating bloom or a decaying settle at the same overall slope.
	Curvature float64

	// MonotonicFrac is the fraction of consecutive deltas sharing the
	// majority sign: 1.0 for a strictly monotonic track, ~0.5 for noise. It
	// is the cheapest "shaped gesture vs chaos" discriminator available
	// without a modulation spectrum.
	MonotonicFrac float64

	// N is the number of frames the statistics were computed over. 0 means
	// every field above is a zero placeholder, not a measurement.
	N int
}

// StatsOf reduces one track sampled every hopSec seconds.
//
// An empty or single-sample track yields zeros, never NaN: a report row
// reading NaN is strictly worse than one reading 0 with N=0 alongside it, and
// NaN propagates silently through any downstream comparison.
func StatsOf(track []float64, hopSec float64) TrackStats {
	s := TrackStats{N: len(track)}
	if len(track) == 0 {
		return s
	}

	sorted := append([]float64(nil), track...)
	sort.Float64s(sorted)

	s.Min = sorted[0]
	s.Max = sorted[len(sorted)-1]
	s.Range = s.Max - s.Min
	// percentile takes p in 0-100, not a 0-1 fraction (see vibrato.go).
	s.Median = percentile(sorted, 50)
	s.P10 = percentile(sorted, 10)
	s.P90 = percentile(sorted, 90)
	s.IQR = percentile(sorted, 75) - percentile(sorted, 25)
	s.Mean, s.Std = meanStd(track)

	if len(track) < 2 || hopSec <= 0 {
		return s
	}

	xs := make([]float64, len(track))
	for i := range xs {
		xs[i] = float64(i) * hopSec
	}
	s.Slope = olsSlope(xs, track)
	s.Curvature = quadraticCoeff(xs, track)
	s.MonotonicFrac = monotonicFraction(track)
	return s
}

// quadraticCoeff returns the `a` coefficient of the least-squares fit
// y = a*x^2 + b*x + c.
//
// Solved via the 3x3 normal equations with Cramer's rule. That is adequate
// here because x is a short, evenly-spaced, well-conditioned time axis; it is
// not a general-purpose polynomial fitter. A near-singular system (fewer than
// three distinct x values) returns 0 rather than a huge spurious coefficient.
func quadraticCoeff(xs, ys []float64) float64 {
	n := float64(len(xs))
	if len(xs) < 3 || len(xs) != len(ys) {
		return 0
	}
	var sx, sx2, sx3, sx4, sy, sxy, sx2y float64
	for i, x := range xs {
		x2 := x * x
		sx += x
		sx2 += x2
		sx3 += x2 * x
		sx4 += x2 * x2
		sy += ys[i]
		sxy += x * ys[i]
		sx2y += x2 * ys[i]
	}
	// | sx4 sx3 sx2 | |a|   |sx2y|
	// | sx3 sx2 sx  | |b| = |sxy |
	// | sx2 sx  n   | |c|   |sy  |
	det := sx4*(sx2*n-sx*sx) - sx3*(sx3*n-sx*sx2) + sx2*(sx3*sx-sx2*sx2)
	if math.Abs(det) < 1e-18 {
		return 0
	}
	detA := sx2y*(sx2*n-sx*sx) - sx3*(sxy*n-sy*sx) + sx2*(sxy*sx-sy*sx2)
	return detA / det
}

// monotonicFraction returns the share of consecutive deltas that move in the
// majority direction. Exactly-zero deltas are excluded rather than counted
// against monotonicity: a track that plateaus mid-rise is still a shaped
// gesture, and counting its flat frames as "not monotonic" would penalize
// exactly the envelope shapes worth recognizing.
func monotonicFraction(track []float64) float64 {
	up, down := 0, 0
	for i := 1; i < len(track); i++ {
		switch d := track[i] - track[i-1]; {
		case d > 0:
			up++
		case d < 0:
			down++
		}
	}
	moves := up + down
	if moves == 0 {
		// A perfectly flat track is trivially "monotonic" in the sense that
		// nothing contradicts a direction, but it carries no gesture. Report
		// 0 so it cannot masquerade as a shaped one.
		return 0
	}
	if up > down {
		return float64(up) / float64(moves)
	}
	return float64(down) / float64(moves)
}
