package songmatch

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/songrender"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

type TuningHint struct {
	Instrument string
	Kind       string // "band" | "seed"
	Suggestion string
	SeedDeltas map[string]float64
}

func normBands(b []float64) []float64 {
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

// BandAttribution finds, per imbalanced band, the dominant rendered stem and emits
// a brighten/cut hint. Deterministic (instruments sorted).
func BandAttribution(ref fingerprint.SongFingerprint, rendered songrender.Rendered, cfg fingerprint.AnalysisConfig) []TuningHint {
	sr := rendered.Arrangement.SampleRate
	if sr == 0 {
		sr = 44100
	}
	// Master band balance.
	masterFP := fingerprint.SongFingerprintOf(wave.Wave{Samples: rendered.Master, SampleRate: sr}, cfg)
	refN := normBands(ref.Bands)
	candN := normBands(masterFP.Bands)
	if len(refN) != len(candN) || len(refN) == 0 {
		return nil
	}
	// Per-stem band energy, for picking the dominant stem in a band.
	stemBands := make(map[string][]float64)
	ids := make([]string, 0, len(rendered.Stems))
	for id, s := range rendered.Stems {
		fp := fingerprint.SongFingerprintOf(wave.Wave{Samples: s, SampleRate: sr}, cfg)
		stemBands[id] = fp.Bands
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var hints []TuningHint
	for b := range refN {
		delta := candN[b] - refN[b] // >0 → template has too much energy here
		if math.Abs(delta) < cfg.DriftTol {
			continue
		}
		// Dominant stem in this band.
		bestID, bestE := "", -1.0
		for _, id := range ids {
			if b < len(stemBands[id]) && stemBands[id][b] > bestE {
				bestE = stemBands[id][b]
				bestID = id
			}
		}
		if bestID == "" {
			continue
		}
		name := cfg.Bands[b].Name
		dir := "cut"
		if delta < 0 {
			dir = "boost"
		}
		hints = append(hints, TuningHint{
			Instrument: bestID, Kind: "band",
			Suggestion: fmt.Sprintf("%s the %s band (rendered %+.0f%% vs ref in %q)", dir, name, delta*100, name),
		})
	}
	return hints
}
