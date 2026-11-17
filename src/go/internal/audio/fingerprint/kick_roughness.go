package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_roughness.go — SENSORY DISSONANCE (roughness) of the sustained kick body.
//
// Motivation: every existing kick descriptor (band energies, spectral centroid,
// flatness, MFCC, crest) is an ENERGY / SHAPE statistic — it answers "how much
// energy, where, how bright" but is blind to the BEATING/intermodulation between
// close partials that the ear hears as a "creak", "buzz" or "growl". A real bug
// where saturating, inharmonically-spaced body modes produced a "creaky door"
// buzz was invisible to all of those metrics because the buzz did not move the
// aggregate energy or centroid — only the pairwise spacing of partials changed.
//
// Roughness closes that gap. It is the Plomp–Levelt / Sethares sensory-dissonance
// model: each PAIR of spectral partials contributes dissonance that peaks when
// their separation is about a quarter of a critical bandwidth and falls to ~0
// when they coincide or are more than a critical band apart (e.g. an octave).
// Summed over all partial pairs, a HARMONIC / octave-spaced stack reads LOW
// (consonant) while a dense INHARMONIC cluster reads HIGH (rough/buzzy).
//
// Reference:
//   R. Plomp & W. J. M. Levelt, "Tonal Consonance and Critical Bandwidth",
//     J. Acoust. Soc. Am. 38, 548 (1965).
//   W. A. Sethares, "Local consonance and the relationship between timbre and
//     scale", J. Acoust. Soc. Am. 94, 1218 (1993); "Tuning, Timbre, Spectrum,
//     Scale" (2nd ed., Springer 2005), Appendix E — the parametric curve used
//     here.

const (
	// The roughness is measured on the sustained BODY region only (post-attack,
	// pre-tail): the beater-click attack is legitimately broadband/noisy and would
	// swamp the pairwise-partial structure, and the decayed tail has too little
	// energy for a reliable peak pick. These reuse the same perceptual boundaries
	// as the segment analyzer (kick_segments.go) so "body" means the same region
	// everywhere. Short kicks whose body is empty fall back to the whole signal.
	kickRoughBodyStartSec = segAttackEndMs / 1000 // 35 ms
	kickRoughBodyEndSec   = segBodyEndMs / 1000   // 150 ms

	// Partials below this frequency are ignored (DC / rumble that carries no
	// pitched dissonance).
	kickRoughMinHz = 30.0

	// Peak-pick at most this many partials (by amplitude). The Plomp–Levelt sum is
	// O(N²) in the partial count; 12 is plenty to capture the audible modes while
	// staying cheap and stable.
	kickRoughMaxPartials = 12

	// Sethares' parametric Plomp–Levelt curve constants (his Appendix E):
	//   d(f1,f2) = exp(-b1·s·Δf) − exp(-b2·s·Δf)
	//   s = dStar / (s1·min(f1,f2) + s2)
	// The curve rises from 0, peaks near a quarter critical band, and decays to 0.
	kickRoughB1    = 3.51
	kickRoughB2    = 5.75
	kickRoughDStar = 0.24
	kickRoughS1    = 0.0207
	kickRoughS2    = 18.96

	// Roughness distance is |Δ Roughness| × this, to bring the small raw roughness
	// range (~0–0.09) up to the same order as the other KickDistance sub-terms.
	kickRoughDistScale = 10.0
)

// spectralPartial is one peak-picked spectral component (frequency + linear
// magnitude) used by the roughness model.
type spectralPartial struct {
	Freq float64
	Amp  float64
}

// computeRoughness fills fp.Roughness from the sensory-dissonance of the body
// region's peak-picked partials. Mirrors the computeX pattern in kick_metrics.go.
func (fp *KickFingerprint) computeRoughness(samples []float64, sr int) {
	a := secToSamples(kickRoughBodyStartSec, sr)
	b := secToSamples(kickRoughBodyEndSec, sr)
	if b > len(samples) {
		b = len(samples)
	}
	if a >= b {
		// Body region empty (very short kick) → fall back to the whole signal so
		// the metric is still defined rather than silently zero.
		a, b = 0, len(samples)
	}
	if b-a < 8 {
		return
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[a:b], SampleRate: sr}, kickBandFFT, wave.WindowHann)
	partials := peakPickPartials(mag, binHz, kickRoughMinHz, kickRoughMaxPartials)
	fp.Roughness = plompLeveltRoughness(partials)
}

// peakPickPartials returns up to maxN local maxima of the magnitude spectrum
// (frequency ≥ minHz), strongest first. Sub-bin frequency is refined by
// parabolic interpolation of the three magnitudes around each peak so partial
// spacing is not quantized to the FFT grid.
func peakPickPartials(mag []float64, binHz, minHz float64, maxN int) []spectralPartial {
	if binHz <= 0 || len(mag) < 3 {
		return nil
	}
	loBin := int(minHz / binHz)
	if loBin < 1 { // skip DC
		loBin = 1
	}
	var peaks []spectralPartial
	for i := loBin; i < len(mag)-1; i++ {
		if mag[i] > mag[i-1] && mag[i] >= mag[i+1] && mag[i] > 0 {
			// Parabolic (quadratic) interpolation for sub-bin peak location.
			// delta ∈ [-0.5, 0.5] bins; amp is the interpolated vertex height.
			l, c, r := mag[i-1], mag[i], mag[i+1]
			denom := l - 2*c + r
			delta := 0.0
			amp := c
			if denom != 0 {
				delta = 0.5 * (l - r) / denom
				amp = c - 0.25*(l-r)*delta
			}
			peaks = append(peaks, spectralPartial{Freq: (float64(i) + delta) * binHz, Amp: amp})
		}
	}
	sort.SliceStable(peaks, func(a, b int) bool { return peaks[a].Amp > peaks[b].Amp })
	if len(peaks) > maxN {
		peaks = peaks[:maxN]
	}
	return peaks
}

// plompLeveltRoughness returns the total sensory dissonance of a set of partials,
// normalized to be scale-invariant (independent of overall level).
//
//	roughness = ( Σ_{i<j} a_i·a_j · d(f_i, f_j) ) / ( Σ_i a_i )²
//	d(f1,f2)  = exp(-b1·s·Δf) − exp(-b2·s·Δf),  Δf = |f2−f1|
//	s         = dStar / (s1·min(f1,f2) + s2)
//
// Amplitude weighting uses the PRODUCT a_i·a_j (rather than min) so that dividing
// by (Σa)² makes the result dimensionless and level-invariant — a requirement for
// a stable distance term, since the body region's absolute level varies. A single
// partial (or none) yields 0; a harmonic/octave-spaced stack yields ~0; a dense
// inharmonic cluster yields the largest value.
func plompLeveltRoughness(partials []spectralPartial) float64 {
	if len(partials) < 2 {
		return 0
	}
	ampTot := 0.0
	for _, p := range partials {
		ampTot += p.Amp
	}
	if ampTot <= 0 {
		return 0
	}
	rough := 0.0
	for i := 0; i < len(partials); i++ {
		for j := i + 1; j < len(partials); j++ {
			f1, f2 := partials[i].Freq, partials[j].Freq
			fmin := math.Min(f1, f2)
			s := kickRoughDStar / (kickRoughS1*fmin + kickRoughS2)
			df := math.Abs(f2 - f1)
			d := math.Exp(-kickRoughB1*s*df) - math.Exp(-kickRoughB2*s*df)
			if d < 0 {
				d = 0
			}
			rough += partials[i].Amp * partials[j].Amp * d
		}
	}
	return rough / (ampTot * ampTot)
}
