package fingerprint

import (
	"math"
	"sort"
)

// The string-vs-bar/tube discriminator: a tuned bar's overtones sit near INTEGER multiples of f0, just not
// CONSECUTIVE ones (1:4:9), so nearest-integer mistuning scores 4.02·f0 as in tune. SeriesDeviation asks whether the
// k-th partial by frequency sits near harmonic k; a separate axis, since both questions are useful.

const (
	// Enough for a bar's first overtones or a bell's strike cluster without dragging in noise-floor bins.
	partialRatioMaxCount = 8

	// Sub-harmonic noise below the fundamental never enters: a ratio < 1 has no expected rank.
	partialRatioMinHzFrac = 0.5

	// peakPickPartials has no amplitude floor and pads with noise bins when a tone has fewer real partials, so a larger
	// pool is filtered by relative amplitude.
	partialRatioCandidatePool = 32

	// Relative to the strongest partial, keeping noise ripple out.
	partialRatioMinRelAmp = 0.05
)

type PartialRatios struct {
	// Ascending by frequency, so Ratios[0] is normally ~1.0; nil when f0 <= 0 or nothing significant was found.
	Ratios []float64

	// Mean of |Ratios[i] − (i+1)| / (i+1): 0 for 1,2,3,4…, large whenever partials skip ranks.
	SeriesDeviation float64
}

// Zero for f0 <= 0, binHz <= 0 or a spectrum too short to peak-pick.
func AnalyzePartialRatios(mag []float64, binHz, f0 float64) PartialRatios {
	if f0 <= 0 || binHz <= 0 || len(mag) < 3 {
		return PartialRatios{}
	}
	minHz := f0 * partialRatioMinHzFrac
	peaks := peakPickPartials(mag, binHz, minHz, partialRatioCandidatePool)
	peaks = significantPeaks(peaks, partialRatioMinRelAmp, partialRatioMaxCount)
	return partialRatiosFromPeaks(peaks, f0)
}

// peaks arrive strongest first.
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

// The FFT-free core.
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
