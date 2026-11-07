package fingerprint

import (
	_ "embed"
	"encoding/json"
)

// Hand-guessed thresholds SATURATED; calibration sets drift/mismatch to the p50/p90 of Delta.Mag (declared units) over
// families the listener graded "y". A "probe" entry beats the corpus and a "hand" entry excludes a broken row; both
// survive recalibration. Embedded: regenerate, then rebuild.

//go:embed testdata/thresholds.json
var thresholdJSON []byte

// N is the good-group size behind the percentiles; Note gives the reason a probe/hand entry was pinned, without which
// an exclusion is indistinguishable from a leftover.
type ThresholdEntry struct {
	Drift    float64 `json:"drift"`
	Mismatch float64 `json:"mismatch"`
	N        int     `json:"n"`
	Source   string  `json:"source"`
	Note     string  `json:"note,omitempty"`
}

// ThresholdFile maps descriptor ID to its calibrated thresholds.
type ThresholdFile map[string]ThresholdEntry

// A malformed file yields an EMPTY map, not an error: the hand thresholds work, and a corrupt optimisation artefact
// must not take the tool down.
func LoadThresholdFile() ThresholdFile {
	out := ThresholdFile{}
	if len(thresholdJSON) > 0 {
		_ = json.Unmarshal(thresholdJSON, &out)
	}
	return out
}

// Interpolating: the good group is small, and at n=8 a rounded p90 is the maximum, so one outlier would BE the
// threshold.
func Percentile(sorted []float64, p float64) float64 { return percentile(sorted, p) }
