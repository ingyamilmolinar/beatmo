package fingerprint

import "encoding/json"

// Span is a labeled time region of a recording plus the instruments expected
// active in it (manifest ground truth; overrides auto-detection for that span).
type Span struct {
	StartSec    float64  `json:"start_sec"`
	EndSec      float64  `json:"end_sec"`
	Label       string   `json:"label,omitempty"`
	Instruments []string `json:"instruments,omitempty"`
}

// TuningPassage is a solo/exposed window to hand to the per-instrument matcher.
type TuningPassage struct {
	StartSec   float64 `json:"start_sec"`
	EndSec     float64 `json:"end_sec"`
	Instrument string  `json:"instrument"`
}

// CompareWindow is the segment of the reference used for whole-segment comparison.
type CompareWindow struct {
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
}

// TimeManifest describes one interpretation's time structure. Every field is
// optional: present → authoritative for that span; absent → auto-detected.
type TimeManifest struct {
	Ref            string          `json:"ref,omitempty"`
	Template       string          `json:"template,omitempty"`
	Transpose      int             `json:"transpose_semitones,omitempty"`
	BPM            float64         `json:"bpm,omitempty"`
	CompareWindow  *CompareWindow  `json:"compare_window,omitempty"`
	Sections       []Span          `json:"sections,omitempty"`
	TuningPassages []TuningPassage `json:"tuning_passages,omitempty"`
}

// LoadTimeManifest parses a manifest; an empty object yields an all-optional
// (fully auto-detect) manifest.
func LoadTimeManifest(b []byte) (TimeManifest, error) {
	var m TimeManifest
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return TimeManifest{}, err
	}
	return m, nil
}
