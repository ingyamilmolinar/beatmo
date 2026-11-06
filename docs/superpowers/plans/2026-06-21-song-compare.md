# Song Compare + Tuning Implementation Plan (Plan 3 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compare a real song recording against the rendered template (Plan 2) using the song-level fingerprint (Plan 1), producing a per-axis correctness report (tempo / rhythm / key+harmony / mix balance) + a single `SongDistance`, plus tuning hints (band-attribution nudges + solo-passage re-tuning via the existing `synthmatch`), exposed through a `song-match compare` CLI and CC0 reference WAVs.

**Architecture:** A new `internal/audio/songmatch` package composes the already-built pieces: `songrender.Render` (template → master + stems), `fingerprint.SongFingerprintOf`/`SongTimelineOf`/`BestChromaShift` (analysis), and `synthmatch.Match`/`PatchSeedFile` (per-instrument tuning). All comparison logic is pure Go and fast-path testable (under `-tags test`, `songrender.Render` resolves to its deterministic stub and `synthmatch`/`DecodeWAVToPCM` resolve to their stubs). This is Plan 3 of 3; Plans 1 (MIR library) and 2 (renderer) are done.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `internal/audio/fingerprint`, `internal/audio/songrender`, `internal/audio/synthmatch`, `internal/audio` (DecodeWAVToPCM), `internal/wave`, python3 stdlib + bash for the CC0 fetch script.

## Global Constraints

- **Use the bundled Go:** `../../.tools/go/bin/go` from `src/go`.
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
- **Determinism:** no `time.Now`, no unseeded RNG; any iterative step seeds from config. Sort all map iteration in outputs.
- **Config-driven:** comparison weights + verdict tolerances live in `fingerprint.AnalysisConfig` (extend it) — no magic numbers in code.
- **Generic:** no per-song/per-instrument special-casing; works for any template id + any reference WAV + any optional `TimeManifest`.
- **Cycle safety:** `songmatch` imports `fingerprint`/`songrender`/`synthmatch`/`audio`/`wave`. Parent `audio` imports none of them.
- **Build tags:** the `songmatch` library carries NO build tag (must compile under `-tags test`). Only `cmd/song-match` is `//go:build !test && !js`.
- **PD constraint:** committed reference WAVs are CC0 classical (Bach/Mozart/Vivaldi) only; any other template uses a user-supplied `-ref` WAV. Tests skip cleanly when `songrefs/` is empty.
- **Tuning never auto-commits:** seed patches are uncommitted; the CLI patches only when NOT `-dry-run`. **Do NOT `git add`/`git commit`** — standing repo rule ([[feedback_never_stage_or_commit]]). Treat every "Commit" step as **"Checkpoint: stop, summarize, let the user review. Do NOT stage."**
- **No stray build artifacts:** `rm -f` any binary `go build ./cmd/...` drops in `src/go/`.

## Verified APIs (confirmed in the codebase — use these exact signatures)

- `fingerprint.SongFingerprint{ SampleRate int; DurationSec, TempoBPM, TempoConfidence float64; OnsetTimes []float64; Chroma [12]float64; Key, Mode int; KeyConfidence, PrimaryHz float64; Harmonics []HarmonicPeak; IntervalHistogram [12]float64; IntonationCents float64; Bands []float64; TonalPercussiveRatio float64 }`; `fingerprint.SongFingerprintOf(w wave.Wave, cfg AnalysisConfig) SongFingerprint`.
- `fingerprint.SongTimeline{ Frames []Frame; Sections []Section; FrameHz float64 }`; `Frame{ TimeSec, RMS float64; Bands []float64; OnsetDensity, RegisterCentroidHz float64; Chroma [12]float64; Key, Mode int }`; `fingerprint.SongTimelineOf(w, cfg)`.
- `fingerprint.BestChromaShift(a, b [12]float64) (shift int, corr float64)`.
- `fingerprint.AnalysisConfig` (extend with weights/tolerances/seed already present: `Seed int64`).
- `songrender.Render(stem string, bars, sampleRate int) (songrender.Rendered, error)`; `songrender.Rendered{ Arrangement songrender.Arrangement; Master []float64; Stems map[string][]float64 }`; `songrender.Arrangement{ BPM, Subdiv, Bars, SampleRate int; Instruments []songrender.InstrumentSpec; Notes []songrender.NoteEvent; TotalSamples int }`.
- `synthmatch.Match(instID string, ref wave.Wave, budget, restarts int) (synthmatch.Result, error)`; `synthmatch.Result{ InstID string; Params map[string]float64; StartLoss, BestLoss float64; Evals int; … }`; `synthmatch.PatchSeedFile(path, varName string, tuned map[string]float64) error`.
- `audio.DecodeWAVToPCM(path string) ([]float32, int, error)` (has a `-tags test` stub decoder).
- `wave.Wave{ Samples []float64; SampleRate int; Label string }`; `(wave.Wave).Slice(startMs, endMs float64) wave.Wave`; `(wave.Wave).Duration() float64`.

---

### Task 1: Extend `AnalysisConfig` with compare weights + tolerances

**Files:**
- Modify: `src/go/internal/audio/fingerprint/analysis_config.go`
- Test: `src/go/internal/audio/fingerprint/analysis_config_test.go` (add a case)

**Interfaces:**
- Produces (new fields on `AnalysisConfig`, with defaults in `DefaultAnalysisConfig`): `CompareWeights struct { Tempo, Rhythm, Key, Mix float64 }`; `DriftTol, MismatchTol float64` (axis magnitude thresholds: `<DriftTol`=OK, `<MismatchTol`=Drift, else Mismatch).

- [ ] **Step 1: Write the failing test** (append to `analysis_config_test.go`)

```go
func TestDefaultAnalysisConfig_CompareDefaults(t *testing.T) {
	c := DefaultAnalysisConfig()
	if c.CompareWeights.Tempo <= 0 || c.CompareWeights.Key <= 0 || c.CompareWeights.Mix <= 0 || c.CompareWeights.Rhythm <= 0 {
		t.Errorf("compare weights not populated: %+v", c.CompareWeights)
	}
	if !(c.DriftTol > 0 && c.MismatchTol > c.DriftTol) {
		t.Errorf("tolerances invalid: drift=%v mismatch=%v", c.DriftTol, c.MismatchTol)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run CompareDefaults` → FAIL (undefined field).

- [ ] **Step 3: Implement** — add to the `AnalysisConfig` struct (after `Seed`):

```go
	// Compare (Plan 3) — weights per correctness axis and verdict thresholds.
	CompareWeights struct {
		Tempo, Rhythm, Key, Mix float64
	} `json:"compare_weights,omitempty"`
	DriftTol    float64 `json:"drift_tol,omitempty"`    // axis magnitude < this → OK
	MismatchTol float64 `json:"mismatch_tol,omitempty"` // < this → Drift; else Mismatch
```

and in `DefaultAnalysisConfig()` (before `return`/inside the literal), set:

```go
	c := AnalysisConfig{ /* …existing fields… */ }
	c.CompareWeights.Tempo = 1.0
	c.CompareWeights.Rhythm = 1.0
	c.CompareWeights.Key = 1.2
	c.CompareWeights.Mix = 1.0
	c.DriftTol = 0.15
	c.MismatchTol = 0.4
	return c
```

(If `DefaultAnalysisConfig` currently returns a composite literal directly, refactor to assign to `c`, set the compare fields, then `return c` — keep all existing field values unchanged.)

- [ ] **Step 4: Run to verify pass** — `… -run 'AnalysisConfig|CompareDefaults'` → PASS, plus the whole package `… ./internal/audio/fingerprint/` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 2: `songmatch` package — axis metrics + `SongDistance`

**Files:**
- Create: `src/go/internal/audio/songmatch/distance.go`
- Test: `src/go/internal/audio/songmatch/distance_test.go`

**Interfaces:**
- Produces:
  - `type Score struct { Tempo, Rhythm, Key, Mix, Total float64 }`
  - `func TempoMagnitude(refBPM, candBPM float64) float64` — relative BPM diff with ×2/÷2 octave folding, clamped to [0,1].
  - `func KeyMagnitude(ref, cand fingerprint.SongFingerprint) (mag float64, shift int)` — `1-corr` from `BestChromaShift` blended with a mode-mismatch penalty; returns the implied transposition.
  - `func MixMagnitude(refBands, candBands []float64) float64` — L1 distance between sum-normalized band vectors, halved to [0,1].
  - `func RhythmMagnitude(ref, cand fingerprint.SongFingerprint, bins int) float64` — L1 between onset bar-phase histograms.
  - `func OnsetPhaseHistogram(onsetTimes []float64, bpm float64, bins int) []float64` — onsets folded into one 4/4 bar, normalized.
  - `func SongDistance(ref, cand fingerprint.SongFingerprint, cfg fingerprint.AnalysisConfig) Score`.

- [ ] **Step 1: Write the failing test**

```go
package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestTempoMagnitude_OctaveEquivalent(t *testing.T) {
	if m := TempoMagnitude(120, 120); m > 1e-9 {
		t.Errorf("identical tempo mag=%v want 0", m)
	}
	if m := TempoMagnitude(120, 240); m > 0.05 {
		t.Errorf("octave tempo mag=%v want ~0 (folded)", m)
	}
	if m := TempoMagnitude(120, 130); !(m > 0) {
		t.Errorf("near tempo mag=%v want >0", m)
	}
}

func TestMixMagnitude_IdentityAndDiff(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	if m := MixMagnitude(a, a); m > 1e-9 {
		t.Errorf("identity mix mag=%v want 0", m)
	}
	b := []float64{5, 4, 3, 2, 1}
	if MixMagnitude(a, b) <= 0 {
		t.Error("different band balance should give >0")
	}
}

func TestSongDistance_IdentityZero(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	fp := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	s := SongDistance(fp, fp, cfg)
	if s.Total > 1e-6 {
		t.Errorf("identity Total=%v want 0", s.Total)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… ./internal/audio/songmatch/ -run 'TempoMagnitude|MixMagnitude|SongDistance'` → FAIL.

- [ ] **Step 3: Implement `distance.go`**

```go
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
	s.Rhythm = RhythmMagnitude(ref, cand, 16)
	w := cfg.CompareWeights
	s.Total = w.Tempo*s.Tempo + w.Rhythm*s.Rhythm + w.Key*s.Key + w.Mix*s.Mix
	return s
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'TempoMagnitude|MixMagnitude|SongDistance'` → PASS, plus the whole package → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 3: `Compare` → CorrectnessReport

**Files:**
- Create: `src/go/internal/audio/songmatch/compare.go`
- Test: `src/go/internal/audio/songmatch/compare_test.go`

**Interfaces:**
- Consumes: `SongDistance`/`KeyMagnitude` (Task 2); `fingerprint.SongFingerprint`/`AnalysisConfig`.
- Produces:
  - `type AxisVerdict struct { Axis, State, Detail string; Magnitude float64 }`
  - `type CorrectnessReport struct { Axes []AxisVerdict; Distance Score; ImpliedTranspose int }`
  - `func Compare(ref, cand fingerprint.SongFingerprint, cfg fingerprint.AnalysisConfig) CorrectnessReport` — builds one `AxisVerdict` per axis (fixed order: tempo, rhythm, key, mix) with `State` from `cfg.DriftTol`/`MismatchTol`, plus the implied transposition.

- [ ] **Step 1: Write the failing test**

```go
package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestCompare_IdentityAllOK(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	fp := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	r := Compare(fp, fp, cfg)
	if len(r.Axes) != 4 {
		t.Fatalf("got %d axes, want 4", len(r.Axes))
	}
	for _, a := range r.Axes {
		if a.State != "OK" {
			t.Errorf("axis %s state=%s want OK (mag=%v)", a.Axis, a.State, a.Magnitude)
		}
	}
}

func TestCompare_NoiseVsToneFlagsAxes(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	ref := fingerprint.SongFingerprintOf(wave.Sine(220, 0.9, 2.0, 44100), cfg)
	cand := fingerprint.SongFingerprintOf(wave.Noise(5, 0.9, 2.0, 44100), cfg)
	r := Compare(ref, cand, cfg)
	flagged := false
	for _, a := range r.Axes {
		if a.State != "OK" {
			flagged = true
		}
	}
	if !flagged {
		t.Error("sine vs noise should flag at least one axis")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run TestCompare` → FAIL.

- [ ] **Step 3: Implement `compare.go`**

```go
package songmatch

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

type AxisVerdict struct {
	Axis      string
	State     string // "OK" | "Drift" | "Mismatch"
	Magnitude float64
	Detail    string
}

type CorrectnessReport struct {
	Axes             []AxisVerdict
	Distance         Score
	ImpliedTranspose int
}

func stateFor(mag float64, cfg fingerprint.AnalysisConfig) string {
	switch {
	case mag < cfg.DriftTol:
		return "OK"
	case mag < cfg.MismatchTol:
		return "Drift"
	default:
		return "Mismatch"
	}
}

// Compare builds the per-axis correctness report (tempo, rhythm, key, mix) plus
// the weighted SongDistance and the implied key transposition.
func Compare(ref, cand fingerprint.SongFingerprint, cfg fingerprint.AnalysisConfig) CorrectnessReport {
	s := SongDistance(ref, cand, cfg)
	keyMag, shift := KeyMagnitude(ref, cand)
	modeStr := "same mode"
	if ref.Mode != cand.Mode {
		modeStr = "mode differs"
	}
	axes := []AxisVerdict{
		{Axis: "tempo", Magnitude: s.Tempo, State: stateFor(s.Tempo, cfg),
			Detail: fmt.Sprintf("ref %.1f BPM vs template %.1f BPM", ref.TempoBPM, cand.TempoBPM)},
		{Axis: "rhythm", Magnitude: s.Rhythm, State: stateFor(s.Rhythm, cfg),
			Detail: "onset bar-phase histogram L1"},
		{Axis: "key", Magnitude: keyMag, State: stateFor(keyMag, cfg),
			Detail: fmt.Sprintf("implied transpose %+d semitones, %s", shift, modeStr)},
		{Axis: "mix", Magnitude: s.Mix, State: stateFor(s.Mix, cfg),
			Detail: "per-band energy balance"},
	}
	return CorrectnessReport{Axes: axes, Distance: s, ImpliedTranspose: shift}
}
```

- [ ] **Step 4: Run to verify pass** — `… -run TestCompare` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 4: Band-attribution tuning hints (from stems)

**Files:**
- Create: `src/go/internal/audio/songmatch/attribution.go`
- Test: `src/go/internal/audio/songmatch/attribution_test.go`

**Interfaces:**
- Consumes: `fingerprint.SongFingerprintOf`/`BandEnergies`-style band vectors; `songrender.Rendered`.
- Produces:
  - `type TuningHint struct { Instrument, Kind, Suggestion string; SeedDeltas map[string]float64 }`
  - `func BandAttribution(ref fingerprint.SongFingerprint, rendered songrender.Rendered, cfg fingerprint.AnalysisConfig) []TuningHint` — for each band where the rendered master deviates from the reference (sum-normalized), find the stem with the most energy in that band and emit a brighten/cut hint, sorted by instrument id.

- [ ] **Step 1: Write the failing test**

```go
package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/songrender"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestBandAttribution_NamesDominantInstrument(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	// ref is bright (high tone); rendered master is dull (low tone) → mid/low over-represented.
	ref := fingerprint.SongFingerprintOf(wave.Sine(4000, 0.9, 1.0, 44100), cfg)
	sr := 44100
	low := wave.Sine(120, 0.9, 1.0, sr).Samples
	rendered := songrender.Rendered{
		Master: low,
		Stems:  map[string][]float64{"bass": low},
	}
	hints := BandAttribution(ref, rendered, cfg)
	if len(hints) == 0 {
		t.Fatal("expected at least one band-attribution hint")
	}
	named := false
	for _, h := range hints {
		if h.Instrument == "bass" {
			named = true
		}
	}
	if !named {
		t.Errorf("expected the dominant 'bass' stem to be named; got %+v", hints)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run BandAttribution` → FAIL.

- [ ] **Step 3: Implement `attribution.go`**

```go
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
			Suggestion: fmt.Sprintf("%s the %s band (template %+.0f%% vs ref in %q)", dir, name, delta*100, name),
		})
	}
	return hints
}
```

- [ ] **Step 4: Run to verify pass** — `… -run BandAttribution` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 5: Instrument activity over time (template matching)

**Files:**
- Create: `src/go/internal/audio/songmatch/activity.go`
- Test: `src/go/internal/audio/songmatch/activity_test.go`

**Interfaces:**
- Produces:
  - `func NNLS(cols [][]float64, b []float64, iters int) []float64` — non-negative least squares (multiplicative updates) solving `min‖Σ cols[i]·x[i] − b‖`, `x≥0`. `cols` are the per-instrument template vectors (same length as `b`).
  - `func InstrumentActivity(realTL fingerprint.SongTimeline, stems map[string][]float64, sr int, cfg fingerprint.AnalysisConfig) (instIDs []string, activity [][]float64)` — builds each stem's average band template, then per real frame runs `NNLS` over the templates → `activity[frame][inst]`. `instIDs` is sorted; approximate (documented).

- [ ] **Step 1: Write the failing test**

```go
package songmatch

import (
	"math"
	"testing"
)

func TestNNLS_RecoversMixture(t *testing.T) {
	// Two orthogonal templates; b is 2*t0 + 3*t1 → recover ~[2,3].
	t0 := []float64{1, 0, 0}
	t1 := []float64{0, 1, 0}
	b := []float64{2, 3, 0}
	x := NNLS([][]float64{t0, t1}, b, 200)
	if math.Abs(x[0]-2) > 0.2 || math.Abs(x[1]-3) > 0.2 {
		t.Errorf("NNLS recovered %v want ~[2,3]", x)
	}
}

func TestNNLS_NonNegative(t *testing.T) {
	t0 := []float64{1, 1, 1}
	b := []float64{-5, -5, -5} // best non-negative fit is ~0
	x := NNLS([][]float64{t0}, b, 100)
	if x[0] < 0 {
		t.Errorf("NNLS returned negative coeff %v", x[0])
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run TestNNLS` → FAIL.

- [ ] **Step 3: Implement `activity.go`**

```go
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
```

- [ ] **Step 4: Run to verify pass** — `… -run 'TestNNLS|InstrumentActivity'` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 6: Solo-passage tuning (extract passage → `synthmatch.Match`)

**Files:**
- Create: `src/go/internal/audio/songmatch/tune.go`
- Test: `src/go/internal/audio/songmatch/tune_test.go`

**Interfaces:**
- Consumes: `fingerprint.TimeManifest`/`TuningPassage`; `synthmatch.Match`; `wave.Wave.Slice`.
- Produces:
  - `func SeedVarName(instID string) string` — `"guitar-electric"` → `"guitarElectricSeed"` (camelCase + "Seed"); replicates the synth-match CLI convention.
  - `func TunePassages(refWav wave.Wave, manifest fingerprint.TimeManifest, budget, restarts int) ([]TuningHint, error)` — for each `manifest.TuningPassages`, slice the passage from `refWav` (ms = sec·1000), run `synthmatch.Match(p.Instrument, passage, budget, restarts)`, and emit a `Kind:"seed"` hint with `SeedDeltas = result.Params` and the seed var name. Sorted by instrument id.

- [ ] **Step 1: Write the failing test**

```go
package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSeedVarName(t *testing.T) {
	cases := map[string]string{"sax": "saxSeed", "guitar-electric": "guitarElectricSeed", "french-horn": "frenchHornSeed"}
	for in, want := range cases {
		if got := SeedVarName(in); got != want {
			t.Errorf("SeedVarName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTunePassages_ProducesSeedHint(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	// A 2 s reference; tune the sax against its [0,1] s passage.
	ref := wave.Sine(220, 0.9, 2.0, 44100)
	m := fingerprint.TimeManifest{
		TuningPassages: []fingerprint.TuningPassage{{StartSec: 0, EndSec: 1.0, Instrument: "sax"}},
	}
	hints, err := TunePassages(ref, m, 80, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || hints[0].Instrument != "sax" || hints[0].Kind != "seed" {
		t.Fatalf("want one sax seed hint, got %+v", hints)
	}
	if len(hints[0].SeedDeltas) == 0 {
		t.Error("seed hint has no tuned params")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run 'SeedVarName|TunePassages'` → FAIL.

- [ ] **Step 3: Implement `tune.go`**

```go
package songmatch

import (
	"sort"
	"strings"
	"unicode"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// SeedVarName converts an instrument id to its camelCase "…Seed" variable name.
func SeedVarName(instID string) string {
	parts := strings.FieldsFunc(instID, func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) })
	var sb strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			sb.WriteString(strings.ToLower(p))
		} else {
			r := []rune(p)
			r[0] = unicode.ToUpper(r[0])
			sb.WriteString(string(r))
		}
	}
	sb.WriteString("Seed")
	return sb.String()
}

// TunePassages runs the per-instrument matcher on each manifest tuning passage,
// returning a "seed" TuningHint per passage (sorted by instrument id). Never patches.
func TunePassages(refWav wave.Wave, manifest fingerprint.TimeManifest, budget, restarts int) ([]TuningHint, error) {
	passes := append([]fingerprint.TuningPassage(nil), manifest.TuningPassages...)
	sort.Slice(passes, func(i, j int) bool { return passes[i].Instrument < passes[j].Instrument })
	var hints []TuningHint
	for _, p := range passes {
		passage := refWav.Slice(p.StartSec*1000, p.EndSec*1000)
		res, err := synthmatch.Match(p.Instrument, passage, budget, restarts)
		if err != nil {
			return nil, err
		}
		hints = append(hints, TuningHint{
			Instrument: p.Instrument, Kind: "seed",
			Suggestion: "re-tune " + SeedVarName(p.Instrument) + " toward the solo passage (review git diff + ear)",
			SeedDeltas: res.Params,
		})
	}
	return hints, nil
}
```

- [ ] **Step 4: Run to verify pass** — `… -run 'SeedVarName|TunePassages'` → PASS (may take a few seconds — `Match` runs the optimizer).

- [ ] **Step 5: Checkpoint**

---

### Task 7: `song-match compare` CLI + make target

**Files:**
- Modify: `src/go/cmd/song-match/main.go` (add `compare` subcommand)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `audio.DecodeWAVToPCM`, `songrender.Render`, `fingerprint.SongFingerprintOf`/`SongTimelineOf`/`LoadAnalysisConfig`/`LoadTimeManifest`, `songmatch.Compare`/`BandAttribution`/`TunePassages`/`PatchSeedFile`.

- [ ] **Step 1: Add the `compare` subcommand** — add `case "compare": cmdCompare(os.Args[2:])` to the switch in `main()`, then add:

```go
func cmdCompare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	stem := fs.String("template", "", "template stem id (required)")
	ref := fs.String("ref", "", "reference WAV path (required)")
	manifestPath := fs.String("manifest", "", "optional TimeManifest JSON")
	configPath := fs.String("config", "", "optional AnalysisConfig JSON")
	bars := fs.Int("bars", 8, "bars to render")
	srFlag := fs.Int("sr", 44100, "sample rate")
	tune := fs.Bool("tune", false, "run solo-passage tuning from the manifest")
	dryRun := fs.Bool("dry-run", false, "with -tune, do not patch the seed file")
	seedfile := fs.String("seedfile", "internal/audio/instrument_seeds.go", "seed file to patch")
	_ = fs.Parse(args)
	if *stem == "" || *ref == "" {
		fmt.Fprintln(os.Stderr, "song-match compare: -template and -ref are required")
		os.Exit(1)
	}

	cfg := fingerprint.DefaultAnalysisConfig()
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err == nil {
			cfg, err = fingerprint.LoadAnalysisConfig(b)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
	}
	var manifest fingerprint.TimeManifest
	if *manifestPath != "" {
		b, err := os.ReadFile(*manifestPath)
		if err == nil {
			manifest, err = fingerprint.LoadTimeManifest(b)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "manifest: %v\n", err)
			os.Exit(1)
		}
	}

	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(*ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode ref: %v\n", err)
		os.Exit(1)
	}
	rf := make([]float64, len(pcm))
	for i, v := range pcm {
		rf[i] = float64(v)
	}
	refWav := wave.Wave{Samples: rf, SampleRate: sr, Label: *ref}
	refFP := fingerprint.SongFingerprintOf(refWav, cfg)

	rendered, err := songrender.Render(*stem, *bars, *srFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render template: %v\n", err)
		os.Exit(1)
	}
	candFP := fingerprint.SongFingerprintOf(wave.Wave{Samples: rendered.Master, SampleRate: *srFlag}, cfg)

	rep := songmatch.Compare(refFP, candFP, cfg)
	fmt.Printf("\n=== compare: template %q vs %q ===\n", *stem, *ref)
	for _, a := range rep.Axes {
		fmt.Printf("  %-7s %-9s %6.3f  %s\n", a.Axis, a.State, a.Magnitude, a.Detail)
	}
	fmt.Printf("  %-7s %18.3f\n", "TOTAL", rep.Distance.Total)

	for _, h := range songmatch.BandAttribution(refFP, rendered, cfg) {
		fmt.Printf("  hint [%s] %s: %s\n", h.Kind, h.Instrument, h.Suggestion)
	}

	if *tune {
		hints, err := songmatch.TunePassages(refWav, manifest, 400, 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tune: %v\n", err)
			os.Exit(1)
		}
		for _, h := range hints {
			fmt.Printf("  hint [seed] %s: %s\n", h.Instrument, h.Suggestion)
			if !*dryRun {
				if err := synthmatch.PatchSeedFile(*seedfile, songmatch.SeedVarName(h.Instrument), h.SeedDeltas); err != nil {
					fmt.Fprintf(os.Stderr, "patch %s: %v\n", h.Instrument, err)
					os.Exit(1)
				}
				fmt.Printf("  patched %s in %s\n", songmatch.SeedVarName(h.Instrument), *seedfile)
			}
		}
		if *dryRun {
			fmt.Println("  (dry-run: seeds not modified)")
		}
	}
}
```

Add imports `github.com/ingyamilmolinar/beatmo/internal/audio/songrender`, `github.com/ingyamilmolinar/beatmo/internal/audio/songmatch`, `github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch` (keep existing). Update the usage string to mention `compare`. (`songrender` may already be imported from Plan 2's `render` subcommand — don't duplicate.)

- [ ] **Step 2: Build smoke + cleanup** — `cd src/go && ../../.tools/go/bin/go build ./cmd/song-match/` → exit 0; then `rm -f song-match`.

- [ ] **Step 3: Vet** — `../../.tools/go/bin/go vet ./cmd/song-match/` → exit 0.

- [ ] **Step 4: Add make target** — add `song-compare` to the `.PHONY` line and:

```make
song-compare: ## Compare a template to a reference WAV (TEMPLATE=, REF=, [MANIFEST=], [CONFIG=], [TUNE=1], [DRY_RUN=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match compare -template $(TEMPLATE) -ref $(REF) \
		$(if $(MANIFEST),-manifest $(MANIFEST),) $(if $(CONFIG),-config $(CONFIG),) \
		$(if $(TUNE),-tune,) $(if $(DRY_RUN),-dry-run,)
```

- [ ] **Step 5:** `make -n song-compare TEMPLATE=bach-toccata REF=/tmp/x.wav` expands cleanly. Checkpoint.

---

### Task 8: CC0 reference WAVs — fetch script, manifest, smoke test

**Files:**
- Create: `scripts/fetch_song_refs.sh`
- Create: `src/go/internal/audio/testdata/songrefs/MANIFEST.md`
- Create: `src/go/internal/audio/songmatch/refs_smoke_test.go`

**Interfaces:** none (tooling + data).

- [ ] **Step 1: Write the skip-clean smoke test**

```go
package songmatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongRefs_DecodeAndFingerprint(t *testing.T) {
	dir := filepath.Join("..", "testdata", "songrefs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("no songrefs dir; run scripts/fetch_song_refs.sh")
	}
	var wavs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".wav") {
			wavs = append(wavs, e.Name())
		}
	}
	if len(wavs) == 0 {
		t.Skip("no reference WAVs present; run scripts/fetch_song_refs.sh")
	}
	audio.Reset()
	audio.ResetInstruments()
	cfg := fingerprint.DefaultAnalysisConfig()
	for _, name := range wavs {
		pcm, sr, err := audio.DecodeWAVToPCM(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: decode: %v", name, err)
			continue
		}
		f := make([]float64, len(pcm))
		for i, v := range pcm {
			f[i] = float64(v)
		}
		fp := fingerprint.SongFingerprintOf(wave.Wave{Samples: f, SampleRate: sr}, cfg)
		if fp.TempoBPM <= 0 || fp.TempoBPM > 400 {
			t.Errorf("%s: implausible tempo %.1f BPM", name, fp.TempoBPM)
		}
	}
}
```

- [ ] **Step 2: Run to verify it skips cleanly** — `… -run TestSongRefs` → PASS (skipped, dir absent).

- [ ] **Step 3: Write the fetch script + manifest** — `scripts/fetch_song_refs.sh`: a bash script that, for each `(stem, url)` of a CC0 classical WAV (Bach Toccata, Mozart K545, Vivaldi Spring — sourced from Musopen/IMSLP CC0 where a PCM WAV is offered), probes the URL with `curl -sI` (skip non-200), downloads to `src/go/internal/audio/testdata/songrefs/<stem>.wav`, and appends a `MANIFEST.md` row (stem, source URL, license, sha256). Seed `MANIFEST.md` with the CC0 license header and the per-stem source list. Mark clearly that references must be **PCM WAV** (the stdlib/test decoder requires it); if a source offers only lossy audio, leave the row documented and the WAV absent for the user to supply locally.

- [ ] **Step 4: (Optional, network) run the fetch then the smoke test for real** — `bash scripts/fetch_song_refs.sh` then `… -run TestSongRefs`. If a URL 404s or is lossy-only, the manifest still records provenance and the test still skips/passes on whatever PCM WAVs landed. (CI never depends on this — the skip-clean test is the gate.)

- [ ] **Step 5: Checkpoint** — then, if any CC0 WAV landed, an end-to-end real compare: `make song-compare TEMPLATE=bach-toccata REF=src/go/internal/audio/testdata/songrefs/bach-toccata.wav` and review the report.

---

## Self-Review

**Spec coverage (Plan 3 slice of the song-wav-matching spec §7.1/§8/§9/§10):**
- §8 Compare/report + SongDistance → Tasks 2 (axes + distance), 3 (CorrectnessReport). Config weights/tolerances → Task 1.
- §8 TuningHints: band attribution → Task 4; solo-passage `synthmatch` → Task 6.
- §7.1 instrument activity over time → Task 5 (NNLS template matching; approximate, documented).
- §9 `compare` CLI + make → Task 7.
- §10 CC0 reference WAVs + skip-clean smoke → Task 8.
- The "JSON vs canonical spec" second ground truth is **already covered** by `cmd/gen-showcase/drift_test.go` (TestDrift) — Plan 3 deliberately does not reimplement it; the report focuses on the real-recording axes.
- Transposition-invariance (§6.2) is honored via `KeyMagnitude`/`BestChromaShift` reporting the implied shift rather than flagging it.

**Placeholder scan:** none — every step carries concrete code or an exact command. Task 8's URLs are filled in during execution against live CC0 sources (the framework is source-agnostic and skip-clean, so a wrong/lossy URL degrades gracefully, never breaks CI).

**Type consistency:** `Score` (Task 2) consumed by `Compare` (Task 3) and printed by the CLI (Task 7); `TuningHint` (Task 4) reused by `TunePassages` (Task 6) and the CLI; `fingerprint.SongFingerprint`/`SongTimeline`/`AnalysisConfig`/`TimeManifest` are Plan 1 types; `songrender.Rendered`/`Render` are Plan 2; `synthmatch.Match`/`PatchSeedFile` are the existing matcher. `SeedVarName` (Task 6) reused by the CLI's patch call (Task 7).

**Open verification during execution:** confirm `DefaultAnalysisConfig` can be refactored to set the new compare fields without disturbing existing values (Task 1); confirm `songrender`/`synthmatch` imports aren't already present in `cmd/song-match/main.go` before re-adding (Task 7); confirm live CC0 WAV URLs (Task 8).
