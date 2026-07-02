package songmatch

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// NNLS solves min‖Σ cols[i]·x[i] − b‖ with x ≥ 0 via multiplicative updates
// (Lee–Seung style): x ← x · (Aᵀb⁺)/(AᵀAx + ε), negative b entries floored at 0.
// Deterministic (fixed init), iters bounded.
func NNLS(cols [][]float64, b []float64, iters int) []float64 {
	n := len(cols)
	x := make([]float64, n)
	for i := range x {
		x[i] = 1.0 // fixed non-zero init (deterministic)
	}
	const eps = 1e-12
	bPos := make([]float64, len(b))
	for i, v := range b {
		if v > 0 {
			bPos[i] = v
		}
	}
	for it := 0; it < iters; it++ {
		// Ax (length len(b)).
		ax := make([]float64, len(b))
		for j := range cols {
			for k := range cols[j] {
				ax[k] += cols[j][k] * x[j]
			}
		}
		for j := range cols {
			num, den := 0.0, 0.0
			for k := range cols[j] {
				num += cols[j][k] * bPos[k]
				den += cols[j][k] * ax[k]
			}
			x[j] *= num / (den + eps)
		}
	}
	return x
}

// InstrumentActivity estimates each stem-instrument's per-frame presence in the
// real timeline via NNLS over per-instrument band templates. APPROXIMATE: reliable
// on solo/sparse passages, indicative on dense mixes.
func InstrumentActivity(realTL fingerprint.SongTimeline, stems map[string][]float64, sr int, cfg fingerprint.AnalysisConfig) ([]string, [][]float64) {
	ids := make([]string, 0, len(stems))
	for id := range stems {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 || len(realTL.Frames) == 0 {
		return ids, nil
	}
	// Per-instrument average band template (normalized).
	templates := make([][]float64, len(ids))
	for i, id := range ids {
		fp := fingerprint.SongFingerprintOf(wave.Wave{Samples: stems[id], SampleRate: sr}, cfg)
		templates[i] = normBands(fp.Bands)
	}
	activity := make([][]float64, len(realTL.Frames))
	for f, fr := range realTL.Frames {
		activity[f] = NNLS(templates, normBands(fr.Bands), cfg.NMFIters)
	}
	return ids, activity
}
