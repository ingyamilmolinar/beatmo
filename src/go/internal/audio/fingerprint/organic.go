package fingerprint

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// organic.go — the aliveness axes and their real-world range.
//
// Identity axes (mode ratios, formants, envelope) say WHICH instrument this
// is. Organic axes say whether it sounds like a physical thing at all. An
// invented instrument has no reference for the first kind and can still be
// held to the second: its organic values should sit where real recordings
// sit. The range is computed over every real reference in the corpus and
// committed, so `refs organic` needs no reference clip.

//go:embed testdata/organic_range.json
var organicRangeJSON []byte

// OrganicIDs lists every organic-battery descriptor, in fixed report order.
//
// IDModAMBands is a vector measurement (gesture/vibrato/warble band-energy
// shares) — its placed value, like every vector descriptor's, is
// Measurement.Primary(), which returns Values[0]: the gesture-band (0.5-2Hz)
// share of the loudness modulation spectrum.
func OrganicIDs() []string {
	return []string{
		IDModAMRate, IDModAMDepth, IDModAMBands, IDModAMFlatness, IDModCentroidDepth,
		IDPartialBandwidth, IDPartialIndependence,
		IDOnsetBite, IDOnsetFlatness, IDOnsetPitchGlide,
		IDNoiseResidualCentroid, IDNoiseDecayVsTone,
		IDNoisePersistence, IDVibratoJitter, IDTrackCrestMedian,
	}
}

// OrganicMinN is the smallest per-id reference count the range is trusted
// at. Below it, Place refuses to classify: a p10/p90 band computed from a
// handful of references is not a reliable "real recordings live here" claim,
// even though the entry stays in the JSON so it can fill in as the corpus
// grows.
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
