package fingerprint

import (
	"math"
	"sort"
)

// One statistic set for every track, so whole-signal statistics are a mechanical cross-product. It subsumes
// TemporalFingerprint's centroid scalars on purpose; the orthogonality analysis says which legacy scalars are
// redundant.

// MAGNITUDE and SHAPE separable: a slow bow-stroke shape and fast chaos can share a std. Slope, Curvature and
// MonotonicFrac are the shape; Mean, Std, IQR and Range the magnitude.
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

	// Per SECOND, comparable across hop sizes.
	Slope float64

	// Units/s²: separates a steady ramp (≈0) from an accelerating bloom or a settling decay at the same slope.
	Curvature float64

	// 1.0 strictly monotonic, ~0.5 for noise: the cheapest gesture-vs-chaos discriminator.
	MonotonicFrac float64

	// 0 means every field is a zero placeholder, not a measurement.
	N int
}

// Empty or single-sample tracks yield zeros, never NaN, which propagates silently into comparisons.
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

// Cramer's rule on the 3×3 normal equations, adequate for a short well-conditioned time axis; under three distinct x
// returns 0 rather than a huge spurious coefficient.
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
	det := sx4*(sx2*n-sx*sx) - sx3*(sx3*n-sx*sx2) + sx2*(sx3*sx-sx2*sx2)
	if math.Abs(det) < 1e-18 {
		return 0
	}
	detA := sx2y*(sx2*n-sx*sx) - sx3*(sxy*n-sy*sx) + sx2*(sxy*sx-sy*sx2)
	return detA / det
}

// Exactly-zero deltas are excluded: a plateau mid-rise is still a shaped gesture.
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
		// A flat track contradicts no direction but carries no gesture.
		return 0
	}
	if up > down {
		return float64(up) / float64(moves)
	}
	return float64(down) / float64(moves)
}
