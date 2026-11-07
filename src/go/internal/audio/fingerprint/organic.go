package fingerprint

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// Identity axes say WHICH instrument; organic axes say whether it sounds like a physical thing at all, so an invented
// instrument can be held to where real recordings sit. The range is computed over the corpus and committed.

//go:embed testdata/organic_range.json
var organicRangeJSON []byte

// In fixed report order; a vector descriptor is placed by Primary(), its first component.
func OrganicIDs() []string {
	return []string{
		IDModAMRate, IDModAMDepth, IDModAMBands, IDModAMFlatness, IDModCentroidDepth,
		IDPartialBandwidth, IDPartialIndependence,
		IDOnsetBite, IDOnsetFlatness, IDOnsetPitchGlide,
		IDNoiseResidualCentroid, IDNoiseDecayVsTone,
		IDNoisePersistence, IDVibratoJitter, IDTrackCrestMedian,
	}
}

// Below it Place refuses to classify: a p10/p90 from a handful of references is not a reliable claim.
const OrganicMinN = 10

type RangeEntry struct {
	P10 float64 `json:"p10"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	N   int     `json:"n"`
}

type OrganicRange map[string]RangeEntry

func LoadOrganicRange() OrganicRange {
	out := OrganicRange{}
	if len(organicRangeJSON) == 0 {
		return out
	}
	_ = json.Unmarshal(organicRangeJSON, &out)
	return out
}

func (r OrganicRange) Place(id string, v float64) (float64, string, bool) {
	e, ok := r[id]
	if !ok || e.N < OrganicMinN {
		return 0, "", false
	}
	span := e.P90 - e.P10
	pos := 0.0
	if span > 0 {
		pos = (v - e.P50) / span
	}
	switch {
	case v < e.P10:
		return pos, "below", true
	case v > e.P90:
		return pos, "above", true
	}
	return pos, "inside", true
}

func ComputeOrganicRange(values map[string][]float64) OrganicRange {
	out := OrganicRange{}
	for id, vs := range values {
		if len(vs) == 0 {
			continue
		}
		s := append([]float64(nil), vs...)
		sort.Float64s(s)
		out[id] = RangeEntry{P10: percentile(s, 10), P50: percentile(s, 50), P90: percentile(s, 90), N: len(s)}
	}
	return out
}
