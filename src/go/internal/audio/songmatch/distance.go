// Package songmatch compares a real recording's fingerprint against a rendered
// template's fingerprint into a per-axis correctness report + tuning hints. It
// composes internal/audio/fingerprint, songrender, and synthmatch; all logic is
// pure Go and fast-path testable.
package songmatch

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

type Score struct {
	Tempo, Rhythm, Key, Mix, Total float64
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// TempoMagnitude is the relative BPM difference with ×2/÷2 octave folding.
func TempoMagnitude(refBPM, candBPM float64) float64 {
	if refBPM <= 0 || candBPM <= 0 {
		return 0
	}
	best := math.Abs(refBPM-candBPM) / refBPM
	for _, f := range []float64{0.5, 2.0} {
		if d := math.Abs(refBPM-candBPM*f) / refBPM; d < best {
			best = d
		}
	}
	return clamp01(best)
}

// KeyMagnitude returns (1-corr) blended with a mode-mismatch penalty + the shift.
func KeyMagnitude(ref, cand fingerprint.SongFingerprint) (float64, int) {
	// Identical chroma (including both all-zero / silent, where pearson() returns
	// 0 not 1 for two zero-variance vectors) is a perfect key match at shift 0.
	if ref.Chroma == cand.Chroma {
		return 0, 0
	}
	shift, corr := fingerprint.BestChromaShift(ref.Chroma, cand.Chroma)
	mag := clamp01((1 - corr) / 2) // corr in [-1,1] → [0,1]
	if ref.Mode != cand.Mode {
		mag = clamp01(mag + 0.25)
	}
	return mag, shift
}

// MixMagnitude is half the L1 distance between sum-normalized band vectors (0..1).
func MixMagnitude(refBands, candBands []float64) float64 {
	n := len(refBands)
	if n == 0 || len(candBands) != n {
		return 0
	}
	norm := func(b []float64) []float64 {
		sum := 0.0
		for _, v := range b {
			sum += v
		}
		out := make([]float64, len(b))
		if sum > 0 {
			for i, v := range b {
				out[i] = v / sum
			}
		}
		return out
	}
	ra, ca := norm(refBands), norm(candBands)
	l1 := 0.0
	for i := range ra {
		l1 += math.Abs(ra[i] - ca[i])
	}
	return clamp01(l1 / 2)
}

// OnsetPhaseHistogram folds onset times into one 4/4 bar (bins buckets), normalized.
func OnsetPhaseHistogram(onsetTimes []float64, bpm float64, bins int) []float64 {
	h := make([]float64, bins)
	if bpm <= 0 || bins <= 0 || len(onsetTimes) == 0 {
		return h
	}
	barSec := 60.0 / bpm * 4.0
	for _, t := range onsetTimes {
		phase := math.Mod(t, barSec) / barSec
		b := int(phase * float64(bins))
		if b >= bins {
			b = bins - 1
		}
		if b < 0 {
			b = 0
		}
		h[b]++
	}
	sum := 0.0
	for _, v := range h {
		sum += v
	}
	if sum > 0 {
		for i := range h {
			h[i] /= sum
		}
	}
	return h
}

// RhythmMagnitude is half the L1 distance between onset bar-phase histograms.
func RhythmMagnitude(ref, cand fingerprint.SongFingerprint, bins int) float64 {
	rh := OnsetPhaseHistogram(ref.OnsetTimes, ref.TempoBPM, bins)
	ch := OnsetPhaseHistogram(cand.OnsetTimes, cand.TempoBPM, bins)
	l1 := 0.0
	for i := range rh {
		l1 += math.Abs(rh[i] - ch[i])
	}
	return clamp01(l1 / 2)
}

// SongDistance combines the four axes into a weighted Score (identity == 0).
func SongDistance(ref, cand fingerprint.SongFingerprint, cfg fingerprint.AnalysisConfig) Score {
	var s Score
	s.Tempo = TempoMagnitude(ref.TempoBPM, cand.TempoBPM)
	s.Key, _ = KeyMagnitude(ref, cand)
	s.Mix = MixMagnitude(ref.Bands, cand.Bands)
	bins := cfg.RhythmBins
	if bins <= 0 {
		bins = 16
	}
	s.Rhythm = RhythmMagnitude(ref, cand, bins)
	w := cfg.CompareWeights
	s.Total = w.Tempo*s.Tempo + w.Rhythm*s.Rhythm + w.Key*s.Key + w.Mix*s.Mix
	return s
}
