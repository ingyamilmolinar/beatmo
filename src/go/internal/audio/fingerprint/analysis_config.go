package fingerprint

import "encoding/json"

// BandEdge is one named frequency band for energy-balance analysis.
type BandEdge struct {
	Name string  `json:"name"`
	LoHz float64 `json:"lo_hz,omitempty"`
	HiHz float64 `json:"hi_hz,omitempty"`
}

// AnalysisConfig holds every tunable knob for song-level analysis. All behavior
// is driven from here; library code contains no magic numbers. JSON tags allow a
// partial override file merged over DefaultAnalysisConfig via LoadAnalysisConfig.
type AnalysisConfig struct {
	HopSec           float64     `json:"hop_sec,omitempty"`        // timeline frame hop
	FrameSec         float64     `json:"frame_sec,omitempty"`      // per-frame analysis length
	FFTSize          int         `json:"fft_size,omitempty"`       // whole-segment FFT size
	FrameFFTSize     int         `json:"frame_fft_size,omitempty"` // per-frame FFT size
	Bands            []BandEdge  `json:"bands,omitempty"`          // energy bands (generic N)
	TempoMinBPM      float64     `json:"tempo_min_bpm,omitempty"`
	TempoMaxBPM      float64     `json:"tempo_max_bpm,omitempty"`
	TempoOctaveFold  bool        `json:"tempo_octave_fold,omitempty"` // ×2/÷2 equivalence
	OnsetSensitivity float64     `json:"onset_sensitivity,omitempty"` // flux threshold multiplier
	KeyProfileMajor  [12]float64 `json:"key_profile_major,omitempty"`
	KeyProfileMinor  [12]float64 `json:"key_profile_minor,omitempty"`
	NoveltyKernel    int         `json:"novelty_kernel,omitempty"` // section kernel (frames)
	NoveltyThresh    float64     `json:"novelty_thresh,omitempty"`
	NMFIters         int         `json:"nmf_iters,omitempty"`
	Seed             int64       `json:"seed,omitempty"` // fixed RNG seed
	// Compare (Plan 3) — weights per correctness axis and verdict thresholds.
	CompareWeights struct {
		Tempo, Rhythm, Key, Mix float64
	} `json:"compare_weights,omitempty"`
	DriftTol    float64 `json:"drift_tol,omitempty"`    // axis magnitude < this → OK
	MismatchTol float64 `json:"mismatch_tol,omitempty"` // < this → Drift; else Mismatch
	RhythmBins  int     `json:"rhythm_bins,omitempty"`  // onset bar-phase histogram resolution
}

// DefaultAnalysisConfig returns documented defaults. Krumhansl–Kessler key
// profiles; 5 perceptual bands; 40–240 BPM with octave folding.
func DefaultAnalysisConfig() AnalysisConfig {
	c := AnalysisConfig{
		HopSec:       0.5,
		FrameSec:     0.093,
		FFTSize:      16384,
		FrameFFTSize: 2048,
		Bands: []BandEdge{
			{"sub", 20, 60}, {"low", 60, 250}, {"mid", 250, 2000},
			{"high", 2000, 6000}, {"air", 6000, 20000},
		},
		TempoMinBPM:      40,
		TempoMaxBPM:      240,
		TempoOctaveFold:  true,
		OnsetSensitivity: 1.0,
		KeyProfileMajor:  [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88},
		KeyProfileMinor:  [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17},
		NoveltyKernel:    16,
		NoveltyThresh:    0.5,
		NMFIters:         50,
		Seed:             1,
	}
	c.CompareWeights.Tempo = 1.0
	c.CompareWeights.Rhythm = 1.0
	c.CompareWeights.Key = 1.2
	c.CompareWeights.Mix = 1.0
	c.DriftTol = 0.15
	c.MismatchTol = 0.4
	c.RhythmBins = 16
	return c
}

// LoadAnalysisConfig unmarshals partialJSON over a defaults base, so only the
// fields present in the JSON override defaults (mirrors audio.MergeRecipeDefaults).
func LoadAnalysisConfig(partialJSON []byte) (AnalysisConfig, error) {
	c := DefaultAnalysisConfig()
	if len(partialJSON) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(partialJSON, &c); err != nil {
		return AnalysisConfig{}, err
	}
	return c, nil
}
