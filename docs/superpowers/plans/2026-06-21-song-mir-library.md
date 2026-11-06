# Song-Level MIR Library Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a generic, deterministic, config-driven song-level music-analysis library on the existing `internal/wave` DSP core — tempo, onsets, chroma, key+mode, primary/secondary harmonics, intervals, intonation, band balance, tonal/percussive split, plus a per-frame timeline with section segmentation — and a `song-match timeline` CLI that dumps second-by-second analysis of any WAV.

**Architecture:** New song-level files in `internal/audio/fingerprint` (which imports ONLY `internal/wave`). Two config types (`AnalysisConfig`, `TimeManifest`) drive all behavior with documented defaults and merge-over-defaults JSON loading. A thin `cmd/song-match` (`//go:build !test && !js`) front-end exposes the `timeline` subcommand. This is Plan 1 of 3; Plan 2 (`songrender`) and Plan 3 (`songmatch` compare + tuning) build on it.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `internal/wave` (FFT/STFT/windows/testgen), `internal/audio.DecodeWAVToPCM` (CLI only), Go stdlib.

## Global Constraints

- **Use the bundled Go:** `../../.tools/go/bin/go` from `src/go`.
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
- **No DSP duplication:** all FFT/window/spectrum go through `internal/wave` (`wave.MagnitudeSpectrum`, `wave.WindowHann`). Never write a private FFT. `internal/audio/fingerprint` imports **only** `internal/wave` (+ stdlib).
- **Generic:** no song-/instrument-/template-specific constants in library code. Band edges, key profiles, tolerances, seed — all live in `AnalysisConfig`.
- **Deterministic:** no `time.Now`, no unseeded RNG. Any RNG seeds from `AnalysisConfig.Seed`. Sort all map iteration in outputs.
- **Config-driven:** every magic number is an `AnalysisConfig` field with a documented default. `LoadAnalysisConfig` merges a partial JSON over `DefaultAnalysisConfig()`.
- **CLI only carries build tags:** `cmd/song-match/*.go` is `//go:build !test && !js`. The library package carries no build tags (must compile under `-tags test`).
- **Do NOT `git add`/`git commit`** — standing repo rule ([[feedback_never_stage_or_commit]]). Treat every "Commit" step below as **"Checkpoint: stop, summarize, let the user review the diff. Do NOT stage."**
- **Wave testgen signatures** (from `internal/wave`): `Sine(freqHz, amp, durSec float64, sr int) Wave`, `Noise(seed int, amp, durSec float64, sr int) Wave`, `Square(...)`, `Impulse(...)`; `Wave{Samples []float64; SampleRate int; Label string}`; `Wave.Duration() float64`; `Wave.PeakSample() float64`; `MagnitudeSpectrum(w Wave, fftSize int, window WindowType) (mag []float64, binHz float64)`.

---

### Task 1: `AnalysisConfig` + defaults + merge-over-defaults loader

**Files:**
- Create: `src/go/internal/audio/fingerprint/analysis_config.go`
- Test: `src/go/internal/audio/fingerprint/analysis_config_test.go`

**Interfaces:**
- Produces:
  - `type BandEdge struct { Name string; LoHz, HiHz float64 }`
  - `type AnalysisConfig struct { … }` (fields below)
  - `func DefaultAnalysisConfig() AnalysisConfig`
  - `func LoadAnalysisConfig(jsonBytes []byte) (AnalysisConfig, error)` — unmarshal a partial config and merge over defaults (only fields present in the JSON override).

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import "testing"

func TestDefaultAnalysisConfig_SaneValues(t *testing.T) {
	c := DefaultAnalysisConfig()
	if c.HopSec <= 0 || c.FFTSize <= 0 || len(c.Bands) == 0 {
		t.Fatalf("defaults not populated: %+v", c)
	}
	if c.Seed == 0 {
		t.Errorf("Seed default must be non-zero for determinism")
	}
	if c.TempoMinBPM <= 0 || c.TempoMaxBPM <= c.TempoMinBPM {
		t.Errorf("tempo range invalid: %v..%v", c.TempoMinBPM, c.TempoMaxBPM)
	}
}

func TestLoadAnalysisConfig_MergesOverDefaults(t *testing.T) {
	def := DefaultAnalysisConfig()
	got, err := LoadAnalysisConfig([]byte(`{"hop_sec":0.25,"tempo_max_bpm":300}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.HopSec != 0.25 {
		t.Errorf("HopSec=%v want 0.25 (overridden)", got.HopSec)
	}
	if got.TempoMaxBPM != 300 {
		t.Errorf("TempoMaxBPM=%v want 300 (overridden)", got.TempoMaxBPM)
	}
	// Unset fields keep defaults.
	if got.FFTSize != def.FFTSize {
		t.Errorf("FFTSize=%v want default %v (not overridden)", got.FFTSize, def.FFTSize)
	}
	if len(got.Bands) != len(def.Bands) {
		t.Errorf("Bands replaced unexpectedly: %d vs default %d", len(got.Bands), len(def.Bands))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run AnalysisConfig`
Expected: FAIL — `undefined: DefaultAnalysisConfig`.

- [ ] **Step 3: Implement `analysis_config.go`**

```go
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
	HopSec           float64    `json:"hop_sec,omitempty"`            // timeline frame hop
	FrameSec         float64    `json:"frame_sec,omitempty"`         // per-frame analysis length
	FFTSize          int        `json:"fft_size,omitempty"`          // whole-segment FFT size
	FrameFFTSize     int        `json:"frame_fft_size,omitempty"`    // per-frame FFT size
	Bands            []BandEdge `json:"bands,omitempty"`             // energy bands (generic N)
	TempoMinBPM      float64    `json:"tempo_min_bpm,omitempty"`
	TempoMaxBPM      float64    `json:"tempo_max_bpm,omitempty"`
	TempoOctaveFold  bool       `json:"tempo_octave_fold,omitempty"` // ×2/÷2 equivalence
	OnsetSensitivity float64    `json:"onset_sensitivity,omitempty"` // flux threshold multiplier
	KeyProfileMajor  [12]float64 `json:"key_profile_major,omitempty"`
	KeyProfileMinor  [12]float64 `json:"key_profile_minor,omitempty"`
	NoveltyKernel    int        `json:"novelty_kernel,omitempty"`    // section kernel (frames)
	NoveltyThresh    float64    `json:"novelty_thresh,omitempty"`
	NMFIters         int        `json:"nmf_iters,omitempty"`
	Seed             int64      `json:"seed,omitempty"`              // fixed RNG seed
}

// DefaultAnalysisConfig returns documented defaults. Krumhansl–Kessler key
// profiles; 5 perceptual bands; 40–240 BPM with octave folding.
func DefaultAnalysisConfig() AnalysisConfig {
	return AnalysisConfig{
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
```

Note: `json.Unmarshal` into the pre-filled `c` leaves absent fields untouched, which gives the merge-over-defaults behavior. (Caveat documented in Task 1 comment: a JSON that explicitly sets a field to its zero value cannot un-set it back — acceptable; defaults are non-zero.)

- [ ] **Step 4: Run test to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run AnalysisConfig`
Expected: PASS.

- [ ] **Step 5: Checkpoint** (review; do not stage).

---

### Task 2: `TimeManifest` type + JSON load + optional-field semantics

**Files:**
- Create: `src/go/internal/audio/fingerprint/time_manifest.go`
- Test: `src/go/internal/audio/fingerprint/time_manifest_test.go`

**Interfaces:**
- Produces:
  - `type Span struct { StartSec, EndSec float64; Label string; Instruments []string }`
  - `type TuningPassage struct { StartSec, EndSec float64; Instrument string }`
  - `type CompareWindow struct { StartSec, EndSec float64 }`
  - `type TimeManifest struct { Ref, Template string; Transpose int; BPM float64; CompareWindow *CompareWindow; Sections []Span; TuningPassages []TuningPassage }`
  - `func LoadTimeManifest(jsonBytes []byte) (TimeManifest, error)`

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import "testing"

func TestLoadTimeManifest_OptionalFields(t *testing.T) {
	m, err := LoadTimeManifest([]byte(`{
		"ref":"songrefs/bach-toccata.wav","template":"bach-toccata","bpm":70,
		"compare_window":{"start_sec":4,"end_sec":20},
		"sections":[{"start_sec":0,"end_sec":4,"label":"intro","instruments":["organ"]}],
		"tuning_passages":[{"start_sec":0,"end_sec":4,"instrument":"organ"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Template != "bach-toccata" || m.BPM != 70 {
		t.Errorf("scalar fields wrong: %+v", m)
	}
	if m.CompareWindow == nil || m.CompareWindow.EndSec != 20 {
		t.Errorf("compare window not parsed: %+v", m.CompareWindow)
	}
	if len(m.Sections) != 1 || m.Sections[0].Instruments[0] != "organ" {
		t.Errorf("sections not parsed: %+v", m.Sections)
	}
	if len(m.TuningPassages) != 1 || m.TuningPassages[0].Instrument != "organ" {
		t.Errorf("tuning passages not parsed: %+v", m.TuningPassages)
	}
}

func TestLoadTimeManifest_EmptyIsAllOptional(t *testing.T) {
	m, err := LoadTimeManifest([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.CompareWindow != nil || len(m.Sections) != 0 || len(m.TuningPassages) != 0 {
		t.Errorf("empty manifest should leave everything nil/zero: %+v", m)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run TimeManifest` → FAIL (undefined).

- [ ] **Step 3: Implement `time_manifest.go`**

```go
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
	Ref            string         `json:"ref,omitempty"`
	Template       string         `json:"template,omitempty"`
	Transpose      int            `json:"transpose_semitones,omitempty"`
	BPM            float64        `json:"bpm,omitempty"`
	CompareWindow  *CompareWindow `json:"compare_window,omitempty"`
	Sections       []Span         `json:"sections,omitempty"`
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
```

- [ ] **Step 4: Run to verify pass** — `… -run TimeManifest` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 3: Band energies + tonal/percussive split

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_bands.go`
- Test: `src/go/internal/audio/fingerprint/song_bands_test.go`

**Interfaces:**
- Produces:
  - `func BandEnergies(mag []float64, binHz float64, bands []BandEdge) []float64` — summed magnitude² per band (one value per band, same order).
  - `func TonalPercussiveRatio(w wave.Wave, cfg AnalysisConfig) float64` — fraction of spectral energy concentrated in peaks (tonal≈1, broadband/noisy≈low), via peak-vs-total energy on the whole-segment spectrum.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestBandEnergies_PutsSineInRightBand(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	mag, hz := wave.MagnitudeSpectrum(wave.Sine(100, 0.9, 0.5, 44100), cfg.FFTSize, wave.WindowHann)
	e := BandEnergies(mag, hz, cfg.Bands) // bands: sub,low,mid,high,air
	// 100 Hz lands in "low" (60–250), index 1; that band must dominate.
	for i := range e {
		if i != 1 && e[i] > e[1] {
			t.Errorf("band %d energy %.3g exceeds low-band %.3g for a 100 Hz tone", i, e[i], e[1])
		}
	}
}

func TestTonalPercussiveRatio_SineHighNoiseLow(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	tone := TonalPercussiveRatio(wave.Sine(440, 0.9, 0.8, 44100), cfg)
	noise := TonalPercussiveRatio(wave.Noise(7, 0.9, 0.8, 44100), cfg)
	if !(tone > noise) {
		t.Errorf("tonal(sine)=%.3f should exceed tonal(noise)=%.3f", tone, noise)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run 'BandEnergies|TonalPercussive'` → FAIL.

- [ ] **Step 3: Implement `song_bands.go`**

```go
package fingerprint

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// BandEnergies sums magnitude² within each band's [LoHz,HiHz). One value per
// band, in band order. Bins outside all bands are ignored.
func BandEnergies(mag []float64, binHz float64, bands []BandEdge) []float64 {
	out := make([]float64, len(bands))
	if binHz <= 0 {
		return out
	}
	for i, b := range bands {
		lo := int(b.LoHz / binHz)
		hi := int(b.HiHz / binHz)
		if lo < 0 {
			lo = 0
		}
		if hi >= len(mag) {
			hi = len(mag) - 1
		}
		sum := 0.0
		for k := lo; k <= hi; k++ {
			sum += mag[k] * mag[k]
		}
		out[i] = sum
	}
	return out
}

// TonalPercussiveRatio estimates tonality as the fraction of spectral energy
// concentrated in local peaks. A pure tone puts nearly all energy in one peak
// (→~1); broadband noise spreads energy across bins (→ low). Deterministic.
func TonalPercussiveRatio(w wave.Wave, cfg AnalysisConfig) float64 {
	mag, _ := wave.MagnitudeSpectrum(w, cfg.FFTSize, wave.WindowHann)
	total := 0.0
	peak := 0.0
	for i := 1; i < len(mag)-1; i++ {
		e := mag[i] * mag[i]
		total += e
		// Local-maximum bins count as "tonal" energy.
		if mag[i] >= mag[i-1] && mag[i] >= mag[i+1] {
			peak += e
		}
	}
	if total <= 0 {
		return 0
	}
	return peak / total
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'BandEnergies|TonalPercussive'` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 4: Onset envelope + tempo

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_tempo.go`
- Test: `src/go/internal/audio/fingerprint/song_tempo_test.go`

**Interfaces:**
- Produces:
  - `func OnsetEnvelope(w wave.Wave, cfg AnalysisConfig) (env []float64, frameHz float64)` — spectral-flux novelty per STFT hop (`cfg.FrameFFTSize`, hop = `cfg.HopSec`·sr); `frameHz` is frames/sec.
  - `func OnsetTimes(env []float64, frameHz float64, cfg AnalysisConfig) []float64` — times (sec) of flux peaks above an adaptive threshold scaled by `cfg.OnsetSensitivity`.
  - `func DetectTempo(env []float64, frameHz float64, cfg AnalysisConfig) (bpm, confidence float64)` — autocorrelation of the onset envelope over the lag range implied by `[TempoMinBPM,TempoMaxBPM]`.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// clickTrain builds impulses every periodSec for durSec.
func clickTrain(periodSec, durSec float64, sr int) wave.Wave {
	n := int(durSec * float64(sr))
	s := make([]float64, n)
	step := int(periodSec * float64(sr))
	for i := 0; i < n; i += step {
		s[i] = 1.0
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestDetectTempo_120BPMClickTrain(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.01 // fine hop for tempo resolution
	w := clickTrain(0.5, 4.0, 44100) // 0.5 s period → 120 BPM
	env, fhz := OnsetEnvelope(w, cfg)
	bpm, _ := DetectTempo(env, fhz, cfg)
	if math.Abs(bpm-120) > 120*0.05 && math.Abs(bpm-240) > 240*0.05 && math.Abs(bpm-60) > 60*0.05 {
		t.Errorf("bpm=%.1f want ~120 (or octave 60/240)", bpm)
	}
}

func TestOnsetTimes_FindsClicks(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.01
	w := clickTrain(0.5, 2.0, 44100) // clicks at 0,0.5,1.0,1.5
	env, fhz := OnsetEnvelope(w, cfg)
	times := OnsetTimes(env, fhz, cfg)
	if len(times) < 3 {
		t.Fatalf("found %d onsets, want >=3", len(times))
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run 'DetectTempo|OnsetTimes'` → FAIL.

- [ ] **Step 3: Implement `song_tempo.go`**

```go
package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// stftHops slices w into hop-spaced frames and returns each frame's magnitude
// spectrum (using cfg.FrameFFTSize) plus frames-per-second.
// stftHops also returns binHz — the FFT's ACTUAL resolution from
// wave.MagnitudeSpectrum (which rounds the size up via nextPow2). Callers must
// use this returned binHz, NOT sr/FrameFFTSize, which differs for non-pow2 sizes.
func stftHops(w wave.Wave, cfg AnalysisConfig) (frames [][]float64, frameHz float64, binHz float64) {
	sr := w.SampleRate
	if sr == 0 {
		return nil, 0, 0
	}
	hop := int(cfg.HopSec * float64(sr))
	if hop < 1 {
		hop = 1
	}
	frameLen := cfg.FrameFFTSize
	for start := 0; start+frameLen <= len(w.Samples); start += hop {
		seg := wave.Wave{Samples: w.Samples[start : start+frameLen], SampleRate: sr}
		mag, bh := wave.MagnitudeSpectrum(seg, frameLen, wave.WindowHann)
		binHz = bh
		frames = append(frames, mag)
	}
	return frames, float64(sr) / float64(hop), binHz
}

// OnsetEnvelope computes a spectral-flux novelty curve: per hop, the sum of
// positive magnitude increases over the previous frame.
func OnsetEnvelope(w wave.Wave, cfg AnalysisConfig) ([]float64, float64) {
	frames, fhz, _ := stftHops(w, cfg)
	if len(frames) < 2 {
		return nil, fhz
	}
	env := make([]float64, len(frames))
	for i := 1; i < len(frames); i++ {
		flux := 0.0
		prev, cur := frames[i-1], frames[i]
		for k := range cur {
			d := cur[k] - prev[k]
			if d > 0 {
				flux += d
			}
		}
		env[i] = flux
	}
	return env, fhz
}

// OnsetTimes returns the times (sec) of envelope peaks above mean+sensitivity·std.
func OnsetTimes(env []float64, frameHz float64, cfg AnalysisConfig) []float64 {
	if len(env) == 0 || frameHz <= 0 {
		return nil
	}
	mean, m2 := 0.0, 0.0
	for _, v := range env {
		mean += v
	}
	mean /= float64(len(env))
	for _, v := range env {
		m2 += (v - mean) * (v - mean)
	}
	std := math.Sqrt(m2 / float64(len(env)))
	thresh := mean + cfg.OnsetSensitivity*std
	var times []float64
	for i := 1; i < len(env)-1; i++ {
		if env[i] > thresh && env[i] >= env[i-1] && env[i] >= env[i+1] {
			times = append(times, float64(i)/frameHz)
		}
	}
	return times
}

// DetectTempo autocorrelates the onset envelope and returns the BPM of the
// strongest lag within [TempoMinBPM,TempoMaxBPM], plus a 0..1 confidence
// (peak autocorrelation / zero-lag energy).
func DetectTempo(env []float64, frameHz float64, cfg AnalysisConfig) (float64, float64) {
	n := len(env)
	if n < 4 || frameHz <= 0 {
		return 0, 0
	}
	// Lag range (in frames) from BPM range: bpm = 60*frameHz/lag.
	lagMin := int(60.0 * frameHz / cfg.TempoMaxBPM)
	lagMax := int(60.0 * frameHz / cfg.TempoMinBPM)
	if lagMin < 1 {
		lagMin = 1
	}
	if lagMax >= n {
		lagMax = n - 1
	}
	zero := 0.0
	for _, v := range env {
		zero += v * v
	}
	if zero <= 0 {
		return 0, 0
	}
	bestLag, bestAC := 0, 0.0
	for lag := lagMin; lag <= lagMax; lag++ {
		ac := 0.0
		for i := lag; i < n; i++ {
			ac += env[i] * env[i-lag]
		}
		if ac > bestAC {
			bestAC = ac
			bestLag = lag
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	return 60.0 * frameHz / float64(bestLag), bestAC / zero
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'DetectTempo|OnsetTimes'` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 5: Chroma + key/mode + transposition-invariant compare

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_key.go`
- Test: `src/go/internal/audio/fingerprint/song_key_test.go`

**Interfaces:**
- Produces:
  - `func Chroma(mag []float64, binHz float64) [12]float64` — pitch-class folded magnitude (A=440 reference); L1-normalized.
  - `func DetectKey(chroma [12]float64, cfg AnalysisConfig) (key, mode int, confidence float64)` — Krumhansl correlation over 24 rotations; mode 0=major,1=minor; key 0=C..11=B.
  - `func BestChromaShift(a, b [12]float64) (shift int, corr float64)` — the circular shift of `b` maximizing correlation with `a` (transposition-invariant comparison; `shift` semitones).

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// chromaOf renders a set of pitches (Hz) summed and returns its chroma.
func chromaOf(freqs []float64) [12]float64 {
	sr := 44100
	var s []float64
	for _, f := range freqs {
		w := wave.Sine(f, 0.5, 0.7, sr)
		if s == nil {
			s = make([]float64, len(w.Samples))
		}
		for i := range s {
			s[i] += w.Samples[i]
		}
	}
	mag, hz := wave.MagnitudeSpectrum(wave.Wave{Samples: s, SampleRate: sr}, 16384, wave.WindowHann)
	return Chroma(mag, hz)
}

func TestDetectKey_CMajorTriad(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	// C4,E4,G4 ≈ 261.63, 329.63, 392.00
	ch := chromaOf([]float64{261.63, 329.63, 392.00})
	key, mode, _ := DetectKey(ch, cfg)
	if key != 0 || mode != 0 {
		t.Errorf("key=%d mode=%d want 0(C) major", key, mode)
	}
}

func TestBestChromaShift_TranspositionInvariant(t *testing.T) {
	ch := chromaOf([]float64{261.63, 329.63, 392.00}) // C major
	// Shift the chroma up 2 semitones manually (D major shape).
	var shifted [12]float64
	for i := 0; i < 12; i++ {
		shifted[(i+2)%12] = ch[i]
	}
	shift, corr := BestChromaShift(ch, shifted)
	if shift != 2 {
		t.Errorf("shift=%d want +2 semitones", shift)
	}
	if corr < 0.99 {
		t.Errorf("corr=%.3f want ~1 for a pure transposition", corr)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run 'DetectKey|BestChromaShift'` → FAIL.

- [ ] **Step 3: Implement `song_key.go`**

```go
package fingerprint

import "math"

// Chroma folds the magnitude spectrum into 12 pitch classes (C..B), L1-normalized.
// Pitch class of bin freq f: round(12*log2(f/440)) mod 12, offset so index 0 = C.
func Chroma(mag []float64, binHz float64) [12]float64 {
	var ch [12]float64
	if binHz <= 0 {
		return ch
	}
	for k := 1; k < len(mag); k++ {
		f := float64(k) * binHz
		if f < 20 || f > 5000 {
			continue
		}
		midi := 69 + 12*math.Log2(f/440.0) // A4=440=MIDI69
		// MIDI%12 is already C-based (C=0, A=9), so it IS the chroma index.
		pc := ((int(math.Round(midi)) % 12) + 12) % 12
		ch[pc] += mag[k]
	}
	// L1 normalize.
	sum := 0.0
	for _, v := range ch {
		sum += v
	}
	if sum > 0 {
		for i := range ch {
			ch[i] /= sum
		}
	}
	return ch
}

// pearson returns the Pearson correlation of two 12-vectors.
func pearson(a, b [12]float64) float64 {
	var ma, mb float64
	for i := 0; i < 12; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= 12
	mb /= 12
	var num, da, db float64
	for i := 0; i < 12; i++ {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

// DetectKey correlates chroma against all 24 rotated Krumhansl profiles.
func DetectKey(chroma [12]float64, cfg AnalysisConfig) (int, int, float64) {
	best := -2.0
	bestKey, bestMode := 0, 0
	for mode, profile := range [2][12]float64{cfg.KeyProfileMajor, cfg.KeyProfileMinor} {
		for key := 0; key < 12; key++ {
			var rot [12]float64
			for i := 0; i < 12; i++ {
				rot[i] = profile[((i-key)%12+12)%12]
			}
			if c := pearson(chroma, rot); c > best {
				best = c
				bestKey = key
				bestMode = mode
			}
		}
	}
	conf := (best + 1) / 2 // map [-1,1] → [0,1]
	return bestKey, bestMode, conf
}

// BestChromaShift returns the circular shift s (in semitones, -5..+6) of b that
// best correlates with a, and that correlation. Transposition-invariant compare.
func BestChromaShift(a, b [12]float64) (int, float64) {
	bestShift, bestCorr := 0, -2.0
	for s := 0; s < 12; s++ {
		var rot [12]float64
		for i := 0; i < 12; i++ {
			rot[i] = b[((i+s)%12+12)%12]
		}
		if c := pearson(a, rot); c > bestCorr {
			bestCorr = c
			bestShift = s
		}
	}
	// Normalize shift to -5..+6 for readability.
	if bestShift > 6 {
		bestShift -= 12
	}
	return bestShift, bestCorr
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'DetectKey|BestChromaShift'` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 6: Primary/secondary harmonics + intervals + intonation

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_harmonics.go`
- Test: `src/go/internal/audio/fingerprint/song_harmonics_test.go`

**Interfaces:**
- Produces:
  - `type HarmonicPeak struct { Hz, Mag float64 }`
  - `func HarmonicProfile(mag []float64, binHz float64, n int) []HarmonicPeak` — the `n` strongest spectral peaks, descending by magnitude (peak[0]=primary, [1:]=secondary).
  - `func IntervalHistogram(chroma [12]float64) [12]float64` — pitch-class interval distribution (autocorrelation of chroma).
  - `func IntonationCents(mag []float64, binHz float64) float64` — mean cents deviation of strong peaks from the nearest equal-tempered note.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestHarmonicProfile_PrimaryIsLoudest(t *testing.T) {
	sr := 44100
	// 220 Hz strong + 440 Hz weaker.
	a := wave.Sine(220, 0.9, 0.7, sr).Samples
	b := wave.Sine(440, 0.3, 0.7, sr).Samples
	s := make([]float64, len(a))
	for i := range s {
		s[i] = a[i] + b[i]
	}
	mag, hz := wave.MagnitudeSpectrum(wave.Wave{Samples: s, SampleRate: sr}, 16384, wave.WindowHann)
	peaks := HarmonicProfile(mag, hz, 3)
	if len(peaks) < 2 {
		t.Fatalf("got %d peaks", len(peaks))
	}
	if math.Abs(peaks[0].Hz-220) > 10 {
		t.Errorf("primary=%.1f want ~220", peaks[0].Hz)
	}
	if math.Abs(peaks[1].Hz-440) > 15 {
		t.Errorf("secondary=%.1f want ~440", peaks[1].Hz)
	}
}

func TestIntonationCents_DetunedSine(t *testing.T) {
	sr := 44100
	// 440 Hz is exactly A4 → ~0 cents. 453 Hz is ~+50 cents sharp.
	m0, hz := wave.MagnitudeSpectrum(wave.Sine(440, 0.9, 0.7, sr), 16384, wave.WindowHann)
	mSharp, _ := wave.MagnitudeSpectrum(wave.Sine(453, 0.9, 0.7, sr), 16384, wave.WindowHann)
	if math.Abs(IntonationCents(m0, hz)) > 15 {
		t.Errorf("on-tune cents=%.1f want ~0", IntonationCents(m0, hz))
	}
	if IntonationCents(mSharp, hz) < 20 {
		t.Errorf("sharp cents=%.1f want clearly positive", IntonationCents(mSharp, hz))
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run 'HarmonicProfile|IntonationCents'` → FAIL.

- [ ] **Step 3: Implement `song_harmonics.go`**

```go
package fingerprint

import "math"

// HarmonicPeak is one spectral peak (frequency + magnitude).
type HarmonicPeak struct{ Hz, Mag float64 }

// HarmonicProfile returns the n strongest local-maximum peaks, descending by
// magnitude. peaks[0] is the primary; the rest are secondary harmonics/partials.
func HarmonicProfile(mag []float64, binHz float64, n int) []HarmonicPeak {
	var peaks []HarmonicPeak
	for i := 1; i < len(mag)-1; i++ {
		if mag[i] >= mag[i-1] && mag[i] >= mag[i+1] {
			peaks = append(peaks, HarmonicPeak{Hz: float64(i) * binHz, Mag: mag[i]})
		}
	}
	// Descending sort by magnitude (insertion sort; n is small).
	for i := 1; i < len(peaks); i++ {
		k := peaks[i]
		j := i - 1
		for j >= 0 && peaks[j].Mag < k.Mag {
			peaks[j+1] = peaks[j]
			j--
		}
		peaks[j+1] = k
	}
	if n < len(peaks) {
		peaks = peaks[:n]
	}
	return peaks
}

// IntervalHistogram is the circular autocorrelation of chroma: how much energy
// sits at each pitch-class interval (index 0 = unison, 7 = perfect fifth, …).
func IntervalHistogram(chroma [12]float64) [12]float64 {
	var h [12]float64
	for lag := 0; lag < 12; lag++ {
		s := 0.0
		for i := 0; i < 12; i++ {
			s += chroma[i] * chroma[(i+lag)%12]
		}
		h[lag] = s
	}
	return h
}

// IntonationCents returns the magnitude-weighted mean cents deviation of strong
// peaks from the nearest equal-tempered semitone (A4=440). Positive = sharp.
func IntonationCents(mag []float64, binHz float64) float64 {
	if binHz <= 0 {
		return 0
	}
	// Mean magnitude as a peak-significance floor.
	mean := 0.0
	for _, m := range mag {
		mean += m
	}
	mean /= float64(len(mag))
	var wsum, wcents float64
	for i := 1; i < len(mag)-1; i++ {
		if mag[i] < mean*4 || !(mag[i] >= mag[i-1] && mag[i] >= mag[i+1]) {
			continue
		}
		f := float64(i) * binHz
		if f < 20 || f > 5000 {
			continue
		}
		midi := 69 + 12*math.Log2(f/440.0)
		cents := (midi - math.Round(midi)) * 100
		wsum += mag[i]
		wcents += mag[i] * cents
	}
	if wsum == 0 {
		return 0
	}
	return wcents / wsum
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'HarmonicProfile|IntonationCents'` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 7: `SongFingerprint` assembly

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_fingerprint.go`
- Test: `src/go/internal/audio/fingerprint/song_fingerprint_test.go`

**Interfaces:**
- Consumes: `BandEnergies`, `TonalPercussiveRatio`, `OnsetEnvelope`, `OnsetTimes`, `DetectTempo`, `Chroma`, `DetectKey`, `HarmonicProfile`, `IntervalHistogram`, `IntonationCents` (Tasks 3–6); `wave.MagnitudeSpectrum`.
- Produces:
  - `type SongFingerprint struct { SampleRate int; DurationSec, TempoBPM, TempoConfidence float64; OnsetTimes []float64; Chroma [12]float64; Key, Mode int; KeyConfidence float64; PrimaryHz float64; Harmonics []HarmonicPeak; IntervalHistogram [12]float64; IntonationCents float64; Bands []float64; TonalPercussiveRatio float64 }`
  - `func SongFingerprintOf(w wave.Wave, cfg AnalysisConfig) SongFingerprint`

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongFingerprintOf_PopulatesAllSections(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.02
	w := clickTrain(0.5, 4.0, 44100) // reuse helper from song_tempo_test.go
	// Add a tonal component so chroma/harmonics are non-empty.
	tone := wave.Sine(261.63, 0.5, 4.0, 44100).Samples
	for i := range w.Samples {
		w.Samples[i] += tone[i]
	}
	fp := SongFingerprintOf(w, cfg)
	if fp.TempoBPM <= 0 {
		t.Errorf("TempoBPM not detected")
	}
	if len(fp.Bands) != len(cfg.Bands) {
		t.Errorf("Bands=%d want %d", len(fp.Bands), len(cfg.Bands))
	}
	if len(fp.Harmonics) == 0 {
		t.Errorf("Harmonics empty")
	}
}

func TestSongFingerprintOf_Deterministic(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	w := wave.Sine(220, 0.9, 2.0, 44100)
	a := SongFingerprintOf(w, cfg)
	b := SongFingerprintOf(w, cfg)
	if a.TempoBPM != b.TempoBPM || a.Key != b.Key || a.IntonationCents != b.IntonationCents {
		t.Errorf("non-deterministic fingerprint")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run SongFingerprintOf` → FAIL.

- [ ] **Step 3: Implement `song_fingerprint.go`**

```go
package fingerprint

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// SongFingerprint is a whole-segment musical descriptor of a recording.
type SongFingerprint struct {
	SampleRate           int
	DurationSec          float64
	TempoBPM             float64
	TempoConfidence      float64
	OnsetTimes           []float64
	Chroma               [12]float64
	Key, Mode            int
	KeyConfidence        float64
	PrimaryHz            float64
	Harmonics            []HarmonicPeak
	IntervalHistogram    [12]float64
	IntonationCents      float64
	Bands                []float64
	TonalPercussiveRatio float64
}

// SongFingerprintOf computes all whole-segment musical descriptors of w under cfg.
func SongFingerprintOf(w wave.Wave, cfg AnalysisConfig) SongFingerprint {
	fp := SongFingerprint{SampleRate: w.SampleRate, DurationSec: w.Duration()}

	mag, binHz := wave.MagnitudeSpectrum(w, cfg.FFTSize, wave.WindowHann)
	fp.Bands = BandEnergies(mag, binHz, cfg.Bands)
	fp.TonalPercussiveRatio = TonalPercussiveRatio(w, cfg)
	fp.Chroma = Chroma(mag, binHz)
	fp.Key, fp.Mode, fp.KeyConfidence = DetectKey(fp.Chroma, cfg)
	fp.Harmonics = HarmonicProfile(mag, binHz, 8)
	if len(fp.Harmonics) > 0 {
		fp.PrimaryHz = fp.Harmonics[0].Hz
	}
	fp.IntervalHistogram = IntervalHistogram(fp.Chroma)
	fp.IntonationCents = IntonationCents(mag, binHz)

	env, fhz := OnsetEnvelope(w, cfg)
	fp.OnsetTimes = OnsetTimes(env, fhz, cfg)
	fp.TempoBPM, fp.TempoConfidence = DetectTempo(env, fhz, cfg)
	return fp
}
```

- [ ] **Step 4: Run to verify pass** — `… -run SongFingerprintOf` then the full package `… ./internal/audio/fingerprint/` → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 8: `SongTimeline` + section segmentation

**Files:**
- Create: `src/go/internal/audio/fingerprint/song_timeline.go`
- Test: `src/go/internal/audio/fingerprint/song_timeline_test.go`

**Interfaces:**
- Consumes: `stftHops`, `BandEnergies`, `Chroma`, `DetectKey`, `OnsetEnvelope` (frame-level), `wave`.
- Produces:
  - `type Frame struct { TimeSec, RMS float64; Bands []float64; OnsetDensity float64; RegisterCentroidHz float64; Chroma [12]float64; Key, Mode int }`
  - `type Section struct { StartSec, EndSec float64; Label string }`
  - `type SongTimeline struct { Frames []Frame; Sections []Section; FrameHz float64 }`
  - `func SongTimelineOf(w wave.Wave, cfg AnalysisConfig) SongTimeline`

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongTimelineOf_FramesCoverDuration(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.1
	w := wave.Sine(220, 0.8, 3.0, 44100)
	tl := SongTimelineOf(w, cfg)
	if len(tl.Frames) < 20 {
		t.Fatalf("got %d frames for 3 s @0.1 hop", len(tl.Frames))
	}
	// Frames must be time-ordered.
	for i := 1; i < len(tl.Frames); i++ {
		if tl.Frames[i].TimeSec < tl.Frames[i-1].TimeSec {
			t.Fatalf("frames not time-ordered at %d", i)
		}
	}
}

func TestSongTimelineOf_DetectsSectionBoundary(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.05
	cfg.NoveltyKernel = 8
	sr := 44100
	// First half low tone, second half high tone → a boundary near the middle.
	lo := wave.Sine(150, 0.8, 2.0, sr).Samples
	hi := wave.Sine(1500, 0.8, 2.0, sr).Samples
	w := wave.Wave{Samples: append(append([]float64{}, lo...), hi...), SampleRate: sr}
	tl := SongTimelineOf(w, cfg)
	found := false
	for _, s := range tl.Sections {
		if s.StartSec > 1.5 && s.StartSec < 2.5 {
			found = true
		}
	}
	if len(tl.Sections) < 2 || !found {
		t.Errorf("expected a section boundary near 2.0 s; sections=%+v", tl.Sections)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run SongTimelineOf` → FAIL.

- [ ] **Step 3: Implement `song_timeline.go`**

```go
package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Frame is one timeline slice's descriptors. Section is a labeled span.
type Frame struct {
	TimeSec            float64
	RMS                float64
	Bands              []float64
	OnsetDensity       float64
	RegisterCentroidHz float64
	Chroma             [12]float64
	Key, Mode          int
}
type Section struct {
	StartSec, EndSec float64
	Label            string
}
type SongTimeline struct {
	Frames   []Frame
	Sections []Section
	FrameHz  float64
}

// SongTimelineOf builds per-hop frames and segments them into sections via a
// self-similarity novelty curve over per-frame feature vectors.
func SongTimelineOf(w wave.Wave, cfg AnalysisConfig) SongTimeline {
	frames, fhz, binHz := stftHops(w, cfg)
	if len(frames) == 0 {
		return SongTimeline{FrameHz: fhz}
	}
	sr := w.SampleRate
	hop := int(cfg.HopSec * float64(sr))
	// binHz is the FFT's ACTUAL resolution from stftHops — do NOT recompute it as
	// sr/FrameFFTSize, which diverges for non-power-of-two FrameFFTSize.

	env, _ := OnsetEnvelope(w, cfg) // same hop → aligned with frames

	out := SongTimeline{FrameHz: fhz}
	feats := make([][12]float64, len(frames)) // chroma feature per frame for novelty
	for i, mag := range frames {
		var f Frame
		f.TimeSec = float64(i) / fhz
		// RMS from the time-domain segment.
		start := i * hop
		end := start + cfg.FrameFFTSize
		if end > len(w.Samples) {
			end = len(w.Samples)
		}
		ss := 0.0
		for s := start; s < end; s++ {
			ss += w.Samples[s] * w.Samples[s]
		}
		if end > start {
			f.RMS = math.Sqrt(ss / float64(end-start))
		}
		f.Bands = BandEnergies(mag, binHz, cfg.Bands)
		// Spectral centroid (register).
		num, den := 0.0, 0.0
		for k, m := range mag {
			num += float64(k) * binHz * m
			den += m
		}
		if den > 0 {
			f.RegisterCentroidHz = num / den
		}
		f.Chroma = Chroma(mag, binHz)
		f.Key, f.Mode, _ = DetectKey(f.Chroma, cfg)
		if i < len(env) {
			f.OnsetDensity = env[i]
		}
		feats[i] = f.Chroma
		out.Frames = append(out.Frames, f)
	}

	out.Sections = segmentSections(feats, fhz, cfg)
	// Set EndSec of last section to track duration.
	if n := len(out.Sections); n > 0 {
		out.Sections[n-1].EndSec = w.Duration()
	}
	return out
}

// segmentSections computes a novelty curve from frame-to-frame feature distance
// (smoothed over NoveltyKernel) and emits a section at each above-threshold peak.
func segmentSections(feats [][12]float64, frameHz float64, cfg AnalysisConfig) []Section {
	n := len(feats)
	if n == 0 || frameHz <= 0 {
		return nil
	}
	novelty := make([]float64, n)
	k := cfg.NoveltyKernel
	if k < 1 {
		k = 1
	}
	for i := k; i < n-k; i++ {
		// Mean feature before vs after window i.
		var before, after [12]float64
		for j := i - k; j < i; j++ {
			for c := 0; c < 12; c++ {
				before[c] += feats[j][c]
			}
		}
		for j := i; j < i+k; j++ {
			for c := 0; c < 12; c++ {
				after[c] += feats[j][c]
			}
		}
		d := 0.0
		for c := 0; c < 12; c++ {
			diff := (before[c] - after[c]) / float64(k)
			d += diff * diff
		}
		novelty[i] = math.Sqrt(d)
	}
	// Normalize novelty 0..1.
	maxN := 0.0
	for _, v := range novelty {
		if v > maxN {
			maxN = v
		}
	}
	if maxN > 0 {
		for i := range novelty {
			novelty[i] /= maxN
		}
	}
	// Boundaries = local maxima above threshold.
	var bounds []int
	bounds = append(bounds, 0)
	for i := k + 1; i < n-k-1; i++ {
		if novelty[i] > cfg.NoveltyThresh && novelty[i] >= novelty[i-1] && novelty[i] >= novelty[i+1] {
			// Suppress near-duplicate boundaries within one kernel width.
			if i-bounds[len(bounds)-1] >= k {
				bounds = append(bounds, i)
			}
		}
	}
	var secs []Section
	for bi, b := range bounds {
		startSec := float64(b) / frameHz
		endSec := float64(n) / frameHz
		if bi+1 < len(bounds) {
			endSec = float64(bounds[bi+1]) / frameHz
		}
		secs = append(secs, Section{StartSec: startSec, EndSec: endSec, Label: sectionLabel(bi)})
	}
	return secs
}

// sectionLabel assigns generic ordinal labels (no song-specific knowledge).
func sectionLabel(i int) string {
	labels := []string{"intro", "A", "B", "C", "D", "E", "F", "G"}
	if i < len(labels) {
		return labels[i]
	}
	return "section"
}
```

- [ ] **Step 4: Run to verify pass** — `… -run SongTimelineOf` then full package → PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 9: `cmd/song-match timeline` CLI + make target

**Files:**
- Create: `src/go/cmd/song-match/main.go` (`//go:build !test && !js`)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `audio.DecodeWAVToPCM`, `fingerprint.DefaultAnalysisConfig`, `fingerprint.LoadAnalysisConfig`, `fingerprint.SongTimelineOf`, `fingerprint.SongFingerprintOf`, `wave.Wave`.

- [ ] **Step 1: Implement `main.go`**

```go
//go:build !test && !js

// song-match: song-level analysis tooling (Plan 1: the `timeline` subcommand).
//
// Usage:
//
//	song-match timeline -file <wav> [-config c.json] [-hop 0.5] [-out tl.csv]
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: song-match timeline -file <wav> [-config c.json] [-hop N] [-out tl.csv]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "timeline":
		cmdTimeline(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdTimeline(args []string) {
	fs := flag.NewFlagSet("timeline", flag.ExitOnError)
	file := fs.String("file", "", "path to WAV (required)")
	configPath := fs.String("config", "", "AnalysisConfig JSON (optional)")
	hop := fs.Float64("hop", 0, "override hop seconds (optional)")
	out := fs.String("out", "", "write per-frame CSV to this path (optional)")
	_ = fs.Parse(args)

	if *file == "" {
		fmt.Fprintln(os.Stderr, "song-match timeline: -file is required")
		os.Exit(1)
	}

	cfg := fingerprint.DefaultAnalysisConfig()
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config: %v\n", err)
			os.Exit(1)
		}
		cfg, err = fingerprint.LoadAnalysisConfig(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse config: %v\n", err)
			os.Exit(1)
		}
	}
	if *hop > 0 {
		cfg.HopSec = *hop
	}

	audio.Reset()
	audio.ResetInstruments()
	pcm, sr, err := audio.DecodeWAVToPCM(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode %q: %v\n", *file, err)
		os.Exit(1)
	}
	f64 := make([]float64, len(pcm))
	for i, v := range pcm {
		f64[i] = float64(v)
	}
	w := wave.Wave{Samples: f64, SampleRate: sr, Label: *file}

	fp := fingerprint.SongFingerprintOf(w, cfg)
	tl := fingerprint.SongTimelineOf(w, cfg)

	// Whole-segment summary to STDOUT.
	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	mode := "major"
	if fp.Mode == 1 {
		mode = "minor"
	}
	fmt.Printf("file: %s  dur=%.2fs sr=%d\n", *file, fp.DurationSec, fp.SampleRate)
	fmt.Printf("tempo: %.1f BPM (conf %.2f)\n", fp.TempoBPM, fp.TempoConfidence)
	fmt.Printf("key:   %s %s (conf %.2f)\n", noteNames[fp.Key], mode, fp.KeyConfidence)
	fmt.Printf("primary: %.1f Hz   intonation: %+.1f cents   tonal/perc: %.2f\n",
		fp.PrimaryHz, fp.IntonationCents, fp.TonalPercussiveRatio)

	// Section narrative.
	fmt.Println("sections:")
	for _, s := range tl.Sections {
		fmt.Printf("  %6.2f–%6.2f  %s\n", s.StartSec, s.EndSec, s.Label)
	}

	if *out != "" {
		if err := writeTimelineCSV(*out, tl, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %d frames to %s\n", len(tl.Frames), *out)
	}
}

func writeTimelineCSV(path string, tl fingerprint.SongTimeline, cfg fingerprint.AnalysisConfig) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	wr := csv.NewWriter(f)
	defer wr.Flush()
	header := []string{"time_sec", "rms", "onset_density", "register_hz", "key", "mode"}
	for _, b := range cfg.Bands {
		header = append(header, "band_"+b.Name)
	}
	if err := wr.Write(header); err != nil {
		return err
	}
	for _, fr := range tl.Frames {
		row := []string{
			strconv.FormatFloat(fr.TimeSec, 'f', 3, 64),
			strconv.FormatFloat(fr.RMS, 'g', 4, 64),
			strconv.FormatFloat(fr.OnsetDensity, 'g', 4, 64),
			strconv.FormatFloat(fr.RegisterCentroidHz, 'f', 1, 64),
			strconv.Itoa(fr.Key), strconv.Itoa(fr.Mode),
		}
		for _, b := range fr.Bands {
			row = append(row, strconv.FormatFloat(b, 'g', 4, 64))
		}
		if err := wr.Write(row); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: Build smoke** — `cd src/go && ../../.tools/go/bin/go build ./cmd/song-match/` → exit 0. (Do NOT leave the binary; `rm -f song-match` after, it is a build artifact.)

- [ ] **Step 3: Vet** — `cd src/go && ../../.tools/go/bin/go vet ./cmd/song-match/` → exit 0.

- [ ] **Step 4: Add make targets** to `Makefile` (next to `synth-match`):

```make
song-timeline: ## Second-by-second analysis of a WAV (FILE=, [HOP=], [CONFIG=], [OUT=])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match timeline -file $(FILE) \
		$(if $(HOP),-hop $(HOP),) $(if $(CONFIG),-config $(CONFIG),) $(if $(OUT),-out $(OUT),)
```

Add `song-timeline` to the `.PHONY` line that lists the synth targets.

- [ ] **Step 5: End-to-end check** — generate a quick WAV and run:

```bash
cd src/go && ../../.tools/go/bin/go run ./cmd/song-match timeline \
  -file ../../<any-test>.wav -hop 0.25
```
Expected: prints tempo/key/sections summary without error. (Use any committed WAV under `internal/audio/testdata/` if present; otherwise skip the live run — the build + vet are the gate.)

- [ ] **Step 6: Checkpoint** — run the full package once more: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/` → PASS.

---

## Self-Review

**Spec coverage (Plan 1 slice of the song-wav-matching spec):**
- §2 generic/deterministic/config-driven → Tasks 1, 2 (`AnalysisConfig` merge-over-defaults; `TimeManifest` all-optional) + determinism tests (Tasks 7).
- §6.1 musical analysis set → Tasks 3 (bands, tonal/percussive), 4 (onset, tempo), 5 (chroma, key+mode), 6 (primary/secondary harmonics, intervals, intonation), assembled in Task 7 (`SongFingerprint`).
- §6.2 transposition-invariant compare → Task 5 (`BestChromaShift`).
- §7 timeline + sections → Task 8 (`SongTimeline`, `segmentSections`).
- §9 CLI `timeline` subcommand + make → Task 9.
- §4.1/§4.2 config + manifest types → Tasks 1, 2.
- Deferred to Plan 2/3 (not this plan): `songrender` (§5), instrument activity (§7.1), `Compare`/report/tuning (§8), `compare`/`render` CLI subcommands, reference WAV fetch (§10). `SongDistance` and the real-recording axes live in Plan 3.

**Placeholder scan:** none — every step has concrete code + exact commands.

**Type consistency:** `AnalysisConfig`/`BandEdge` (Task 1) consumed unchanged in Tasks 3–9; `Chroma` returns `[12]float64` consumed by `DetectKey`/`IntervalHistogram`/`BestChromaShift`; `HarmonicPeak` (Task 6) used in `SongFingerprint` (Task 7); `stftHops` (Task 4) reused by Task 8; helper `clickTrain` (Task 4 test) reused by Task 7 test (same package). CLI consumes only exported `SongFingerprintOf`/`SongTimelineOf`/config loaders.

**Open verification during execution:** confirm `wave.WindowType`/`wave.WindowHann` and `wave.MagnitudeSpectrum` signatures (established in the synth-matching framework — Task 1 of that plan) and that `wave.Wave.Duration()`/`PeakSample()` exist; if a name differs, use the actual one.
