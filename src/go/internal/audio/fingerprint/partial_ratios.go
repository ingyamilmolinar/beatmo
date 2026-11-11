package fingerprint

import (
	"math"
	"sort"
)

// partial_ratios.go — the STRING-vs-BAR/TUBE discriminator.
//
// Motivation: neither Fingerprint.Inharmonicity (spectral.go, which only
// searches a narrow ±~1.3%-of-harmonic-number window AROUND each expected
// k·f0 bin) nor the generalized nearest-integer inharmonicity() (used by
// KickFingerprint, kick_inharmonicity.go: n = round(f/f0), deviation =
// |f-n·f0|/(n·f0)) can tell a plucked STRING from a struck wooden BAR or a
// metal TUBE, because a tuned bar's overtones sit close to INTEGER multiples
// of f0 -- just not CONSECUTIVE ones. A marimba/kalimba bar is deliberately
// tuned so its first overtone lands near 4·f0 (undercut tuning aims for
// ~1:4:9-ish ratios), and a tubular bell/chime's strike partials are dense
// and often near-integer but skip most of the series. Both of the above
// metrics measure "how far is this partial from its NEAREST integer
// multiple" -- 4.02·f0 scores as nearly ZERO mistuning under that test
// (round(4.02)=4), even though "the 2nd-strongest partial by frequency sits
// at 4x, not 2x" is exactly the audible tell a listener described as
// "sounds like a plucked string ... needs the wooden marimba feel"
// (kalimba) and "resonant wavy metallic ring" vs "simple clean tone"
// (tubular-bells).
//
// SeriesDeviation instead asks a different question: does the k-th
// strongest-by-FREQUENCY significant partial sit near the k-th harmonic
// (1,2,3,4,...), i.e. does the found partial SEQUENCE look like a
// consecutive harmonic series at all? A string's partials are 1,2,3,4 (near
// 0 deviation); a tuned bar's are 1,4,9 (rank 2's partial sits at 4, not 2 —
// large deviation); a bell/tube's are dense and non-integer (large
// deviation from every angle). This is deliberately a NEW, separate axis
// from Inharmonicity/kick's inharmonicity() -- report all three rather than
// replacing any of them, since "near its own nearest integer" and "matches
// the consecutive series" are different (and both useful) questions.

const (
	// partialRatioMaxCount bounds how many of the strongest-by-amplitude
	// partials feed the ratio list -- enough to catch a bar's first two or
	// three overtones (or a bell's dense strike cluster) without dragging
	// in noise-floor spectral bins for a simple, mostly-monopartial tone.
	partialRatioMaxCount = 8

	// partialRatioMinHzFrac: only partials at or above this fraction of f0
	// are considered, so sub-harmonic noise-floor bins below the
	// fundamental never enter the ratio list (a ratio < 1 has no defined
	// "expected consecutive rank").
	partialRatioMinHzFrac = 0.5

	// partialRatioCandidatePool: how many raw local-maxima candidates
	// peakPickPartials is asked for before the significance filter below
	// runs. peakPickPartials's local-max test only requires mag[i]>0 (no
	// amplitude floor), so at maxN=partialRatioMaxCount it would otherwise
	// happily pad the result out with floating-point-noise bins that carry
	// no real energy whenever a clean tone has fewer than maxN genuine
	// partials. Asking for a bigger pool first, then filtering by relative
	// amplitude, keeps the ratio list to real partials only.
	partialRatioCandidatePool = 32

	// partialRatioMinRelAmp: a candidate partial must be at least this
	// fraction of the STRONGEST found partial's amplitude to count as
	// significant -- keeps noise-floor ripple out of the ratio list.
	partialRatioMinRelAmp = 0.05
)

// PartialRatios holds the strongest partials' frequency ratios to f0 and a
// scalar summary of how far that sequence deviates from a true consecutive
// harmonic series -- the string-vs-bar/tube discriminator (see file doc).
type PartialRatios struct {
	// Ratios[i] is the (i+1)-th strongest-by-FREQUENCY significant
	// partial's frequency / f0, ascending by frequency (so Ratios[0] is
	// normally ~1.0, the fundamental itself, for any tone where the
	// fundamental is a significant partial). Nil when f0<=0 or no
	// significant partial was found.
	Ratios []float64

	// SeriesDeviation is the mean, over all found partials, of
	// |Ratios[i] - (i+1)| / (i+1) -- i.e. how far the i-th-by-frequency
	// partial sits from the harmonic number a CONSECUTIVE series would
	// place there. 0 for a perfect harmonic series (1,2,3,4,...); large
	// (often well above 1) whenever the found partials skip ranks, as a
	// tuned bar's ~1,4,9 or a dense inharmonic bell/tube spectrum do.
	SeriesDeviation float64
}

// AnalyzePartialRatios peak-picks the strongest significant partials of the
// magnitude spectrum mag (same input shape as SpectralCentroid/computeSpectral)
// and reports their ratios to f0 plus the series-deviation scalar. Returns a
// zero-value PartialRatios when f0<=0, binHz<=0, or the spectrum is too short
// to peak-pick.
func AnalyzePartialRatios(mag []float64, binHz, f0 float64) PartialRatios {
	if f0 <= 0 || binHz <= 0 || len(mag) < 3 {
		return PartialRatios{}
	}
	minHz := f0 * partialRatioMinHzFrac
	peaks := peakPickPartials(mag, binHz, minHz, partialRatioCandidatePool)
	peaks = significantPeaks(peaks, partialRatioMinRelAmp, partialRatioMaxCount)
	return partialRatiosFromPeaks(peaks, f0)
}

// significantPeaks filters peaks (already strongest-first, per
// peakPickPartials's own sort) down to those at least minRelAmp of the
// strongest peak's amplitude, then caps the result to maxN.
func significantPeaks(peaks []spectralPartial, minRelAmp float64, maxN int) []spectralPartial {
	if len(peaks) == 0 {
		return nil
	}
	threshold := peaks[0].Amp * minRelAmp
	var out []spectralPartial
	for _, p := range peaks {
		if p.Amp < threshold {
			continue
		}
		out = append(out, p)
		if len(out) >= maxN {
			break
		}
	}
	return out
}

// partialRatiosFromPeaks is the pure, FFT-free core (mirrors
// kick_inharmonicity.go's inharmonicity(partials, f0) split): sorts peaks by
// frequency ascending, then compares each rank's ratio-to-f0 against the
// harmonic number a consecutive series would occupy at that rank.
func partialRatiosFromPeaks(peaks []spectralPartial, f0 float64) PartialRatios {
	if f0 <= 0 || len(peaks) == 0 {
		return PartialRatios{}
	}
	ps := make([]spectralPartial, len(peaks))
	copy(ps, peaks)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Freq < ps[j].Freq })

	ratios := make([]float64, len(ps))
	var devSum float64
	for i, p := range ps {
		r := p.Freq / f0
		ratios[i] = r
		expected := float64(i + 1)
		devSum += math.Abs(r-expected) / expected
	}
	return PartialRatios{Ratios: ratios, SeriesDeviation: devSum / float64(len(ps))}
}
