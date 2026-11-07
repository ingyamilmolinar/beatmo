package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Every other kick descriptor is an energy or shape statistic, blind to the beating between close partials the ear
// hears as a creak or buzz. Plomp–Levelt / Sethares dissonance sums each partial PAIR's contribution, peaking at a
// quarter critical band: a harmonic stack reads low, a dense inharmonic cluster high.

const (
	// Body only: the click is broadband and swamps the pairwise structure, the tail too quiet for a reliable peak pick.
	// Short kicks fall back to the whole signal.
	kickRoughBodyStartSec = segAttackEndMs / 1000 // 35 ms
	kickRoughBodyEndSec   = segBodyEndMs / 1000   // 150 ms

	// DC and rumble carry no pitched dissonance.
	kickRoughMinHz = 30.0

	// The sum is O(N²); 12 captures the audible modes cheaply and stably.
	kickRoughMaxPartials = 12

	// Sethares' Appendix E: d = exp(-b1·s·Δf) − exp(-b2·s·Δf), s = dStar / (s1·min(f1,f2) + s2).
	kickRoughB1    = 3.51
	kickRoughB2    = 5.75
	kickRoughDStar = 0.24
	kickRoughS1    = 0.0207
	kickRoughS2    = 18.96

	// Brings the small raw range to the order of the other KickDistance terms.
	kickRoughDistScale = 10.0
)

type spectralPartial struct {
	Freq float64
	Amp  float64
}

func (fp *KickFingerprint) computeRoughness(samples []float64, sr int) {
	a := secToSamples(kickRoughBodyStartSec, sr)
	b := secToSamples(kickRoughBodyEndSec, sr)
	if b > len(samples) {
		b = len(samples)
	}
	if a >= b {
		// So the metric is still defined, not silently zero.
		a, b = 0, len(samples)
	}
	if b-a < 8 {
		return
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[a:b], SampleRate: sr}, kickBandFFT, wave.WindowHann)
	partials := peakPickPartials(mag, binHz, kickRoughMinHz, kickRoughMaxPartials)
	fp.Roughness = plompLeveltRoughness(partials)
}

// Strongest first; parabolic sub-bin refinement, so partial spacing is not quantised to the FFT grid.
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
			// delta ∈ [-0.5, 0.5] bins; amp is the vertex height.
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

// (Σ_{i<j} a_i·a_j·d) / (Σa)²: the PRODUCT weighting over (Σa)² makes it dimensionless and level-invariant, which a
// stable distance term needs. One partial gives 0, a harmonic stack ~0, a dense inharmonic cluster the most.
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
