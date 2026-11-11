# Calibrated Analysis Battery — Phases 1–3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `refs diagnose`'s nine hand-written axes with a generic
descriptor registry, reconnect the orphaned family analyzers through it, and
add whole-signal frame-track statistics — without changing a single existing
verdict.

**Architecture:** One `Descriptor` table in `internal/audio/fingerprint` is the
single source of truth for every measurement. A per-wave `Bundle` memoizes
shared artifacts (STFT, envelope segments, frame tracks) so nothing is computed
twice. `Measurement` carries scalars, vectors or tracks; `Delta` carries the
comparison. `cmd/refs/diagnose.go` becomes a renderer over the registry.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), `internal/wave` for all DSP
primitives, `text/tabwriter` for report rendering. No new third-party deps.

**Spec:** `docs/superpowers/specs/2026-08-22-calibrated-analysis-battery-design.md`

## Status — 2026-08-22

**Phases 1-3 COMPLETE.** 39 descriptors registered across 9 groups. All tasks
verified; every step below is done unless noted.

Two deviations from the plan as written, both discovered during execution and
both corrected in the code rather than worked around:

1. **The plan's Phase 1 gate was wrong.** It named
   `diagnose_acceptance_test.go` as the proof the refactor changed no verdicts.
   That test replays a FROZEN JSON baseline and calls only
   `instrumentDiagnosis.Axis()` — a pure map lookup — so it exercises no
   classification code and would pass even if every classifier were deleted.
   Replaced with a real gate: `cmd/refs/diagnose_classifier_replay_test.go`
   pushes the baseline's 96 measured (reference, synth) pairs through the new
   comparators and asserts byte-identical verdict + direction.

2. **`All()` returns registration order, not (Group, ID) order.** Sorting
   alphabetically scrambled the report's narrative group order. `SortedByGroup()`
   exists separately for goldens.

Also added beyond the plan, in response to a defect the running tool exposed:
`KindRate` plus per-descriptor `Drift`/`Mismatch`/`Unit` overrides. Signed rates
were typed `KindHz`, which compares as a percentage of the reference and is
meaningless for a quantity that crosses zero (trumpet centroid slope: reference
+57 Hz/s, synth −516 Hz/s, rendered "−1013%").

Known pre-existing failure, NOT caused by this work:
`TestDiagnose_ReproducesBonusVerdicts` is 3/5. Both failures are determined
entirely by values stored in the frozen baseline JSON and are what phases 4-5
address.

---

## Global Constraints

- **Never `git add` / `git commit`.** Standing user instruction. Every "Checkpoint"
  step means: run the verification command, confirm green, leave the tree dirty.
- **Never run bare `./...`.** A phantom duplicate `src/go/internal/ui` exists and
  `cmd/*` legitimately fails under `-tags test` ("function main is undeclared").
  Always name packages explicitly.
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`
- **CGo/render path** (`cmd/refs`, anything calling `RenderInstrument`): built
  `!test`, run with plain `../../.tools/go/bin/go test <pkgs>`.
- **`internal/audio/fingerprint` must NEVER import `internal/audio`.** Its package
  doc states this; `internal/audio`'s own untagged test file imports `wavref`,
  which closes the cycle. Only `go vet` **untagged** catches it.
- **`diagnose_acceptance_test.go` must pass unchanged after every task in Phase 1.**
  It is the regression gate proving the refactor altered no verdicts.
- **Axis `Group` and `Name` strings are load-bearing** — the acceptance test keys
  on them. Preserve them byte-for-byte when migrating to registry rows.
- **`gofmt` every file** with `.tools/go/bin/gofmt -w` before checkpointing.

---

## File Structure

**Create — `src/go/internal/audio/fingerprint/`:**

| File | Responsibility |
|---|---|
| `kind.go` | `Kind` enum, per-kind thresholds, `Verdict` type |
| `classify.go` | The four classifiers (fraction / seconds / hz / decay-rate), moved verbatim from `diagnose.go` |
| `measurement.go` | `Form`, `Measurement`, constructors, `NA` helpers |
| `compare.go` | `Delta`, the default comparators, `Kind`→comparator dispatch |
| `bundle.go` | `Bundle` with `sync.Once`-memoized artifact accessors |
| `descriptor.go` | `Descriptor` struct, `Applies` helper predicates |
| `registry.go` | `Registry` type, `Register`, `All`, `ByID`, filters |
| `registry_core.go` | The nine existing axes as rows (Phase 1) |
| `registry_family.go` | Snare / piano / wind / kick rows (Phase 2) |
| `tracks.go` | Frame-track extraction (Phase 3) |
| `trackstats.go` | `TrackStats` — the uniform statistic set (Phase 3) |
| `registry_tracks.go` | Track-statistic rows (Phase 3) |

**Modify:**
- `src/go/cmd/refs/diagnose.go` — becomes a registry renderer.

---

## Phase 1 — Registry foundation

### Task 1: `Kind`, `Verdict`, thresholds

**Files:**
- Create: `src/go/internal/audio/fingerprint/kind.go`
- Test: `src/go/internal/audio/fingerprint/kind_test.go`

**Interfaces:**
- Produces: `Kind` (int enum), `KindFraction`/`KindTinyFraction`/`KindSeconds`/`KindHz`/`KindSeriesDeviation`/`KindDB`/`KindRatio`; `Verdict` (string) with `VerdictOK`/`VerdictDrift`/`VerdictMismatch`; `func (k Kind) Thresholds() (drift, mismatch float64)`.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import "testing"

func TestKindThresholds(t *testing.T) {
	tests := []struct {
		kind             Kind
		drift, mismatch  float64
	}{
		{KindFraction, 0.05, 0.15},
		{KindTinyFraction, 0.005, 0.02},
		{KindSeriesDeviation, 0.15, 0.5},
		{KindSeconds, 1.3, 2.0},
		{KindHz, 10.0, 40.0},
	}
	for _, tc := range tests {
		d, m := tc.kind.Thresholds()
		if d != tc.drift || m != tc.mismatch {
			t.Errorf("%v: got (%v,%v), want (%v,%v)", tc.kind, d, m, tc.drift, tc.mismatch)
		}
	}
}

func TestVerdictOrdering(t *testing.T) {
	if !VerdictMismatch.WorseThan(VerdictDrift) || !VerdictDrift.WorseThan(VerdictOK) {
		t.Fatal("verdict severity ordering is wrong")
	}
	if VerdictOK.WorseThan(VerdictOK) {
		t.Fatal("a verdict is not worse than itself")
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run 'TestKind|TestVerdict'`
Expected: FAIL — undefined `Kind`, `KindFraction`, `Verdict`.

- [ ] **Step 3: Implement**

```go
package fingerprint

// Kind classifies a descriptor's natural scale. It selects both the default
// comparator (compare.go) and the drift/mismatch thresholds below. The values
// are lifted verbatim from cmd/refs/diagnose.go, where they were calibrated
// against 2026-08-22-verdict-to-metric-mapping.md's worked examples.
type Kind int

const (
	// KindFraction is a unitless fraction spanning most of (0..1) — noise
	// fraction, spectral flatness, sustain modulation. Compared as an
	// absolute delta. 0.05 is an audible shift; 0.15 is night-and-day.
	KindFraction Kind = iota
	// KindTinyFraction is a fraction whose natural scale is an order of
	// magnitude smaller — inharmonicity is <0.001 for a clean tone and
	// 0.005-0.03 for a genuinely inharmonic bar or tube.
	KindTinyFraction
	// KindSeconds is a duration, compared as a ratio: "7x short" is the
	// natural unit for a timing error (harp: 1.130s vs 0.160s).
	KindSeconds
	// KindHz is a frequency, compared as a signed percentage of the reference.
	KindHz
	// KindSeriesDeviation is PartialRatios.SeriesDeviation — 0 for a clean
	// consecutive harmonic series, 0.2-0.6 for a tuned bar/tube, several
	// units for a noisy source whose F0 is unreliable.
	KindSeriesDeviation
	// KindDB is a level or level difference in decibels.
	KindDB
	// KindRatio is a dimensionless ratio compared as a ratio, like KindSeconds
	// but without the duration semantics in its rendering.
	KindRatio
)

// Thresholds returns the (drift, mismatch) pair for k. For KindSeconds and
// KindRatio these are RATIOS of larger-to-smaller; for every other kind they
// are absolute deltas in the kind's own units (percent, for KindHz).
func (k Kind) Thresholds() (drift, mismatch float64) {
	switch k {
	case KindTinyFraction:
		return 0.005, 0.02
	case KindSeriesDeviation:
		return 0.15, 0.5
	case KindSeconds, KindRatio:
		return 1.3, 2.0
	case KindHz:
		return 10.0, 40.0
	case KindDB:
		return 1.5, 6.0
	default: // KindFraction
		return 0.05, 0.15
	}
}

// Verdict is the three-way per-axis classification. Deliberately a closed set
// of three rather than a numeric score: the mapping doc establishes that
// collapsing axes into a scalar is what let a 7x decay error cancel against a
// small spectral gain.
type Verdict string

const (
	VerdictOK       Verdict = "ok"
	VerdictDrift    Verdict = "drift"
	VerdictMismatch Verdict = "MISMATCH"
)

// severity ranks verdicts for sorting the "worst offending axes" list.
func (v Verdict) severity() int {
	switch v {
	case VerdictMismatch:
		return 2
	case VerdictDrift:
		return 1
	default:
		return 0
	}
}

// WorseThan reports whether v is strictly more severe than other.
func (v Verdict) WorseThan(other Verdict) bool { return v.severity() > other.severity() }
```

- [ ] **Step 4: Run and confirm green**

Same command as Step 2. Expected: PASS.

- [ ] **Step 5: Checkpoint** — `.tools/go/bin/gofmt -l src/go/internal/audio/fingerprint/` prints nothing. Leave uncommitted.

---

### Task 2: `Measurement` and `Form`

**Files:**
- Create: `src/go/internal/audio/fingerprint/measurement.go`
- Test: `src/go/internal/audio/fingerprint/measurement_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Form` (`FormScalar`/`FormVector`/`FormTrack`); `Measurement`; constructors `Scalar(float64) Measurement`, `Vector([]float64, []string) Measurement`, `Track([]float64, float64) Measurement`, `Unavailable(reason string) Measurement`.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import "testing"

func TestScalarMeasurement(t *testing.T) {
	m := Scalar(0.42)
	if !m.OK || m.Form != FormScalar || m.Scalar != 0.42 {
		t.Fatalf("got %+v", m)
	}
}

func TestUnavailableCarriesReason(t *testing.T) {
	m := Unavailable("no sustain plateau")
	if m.OK {
		t.Fatal("Unavailable must not be OK")
	}
	if m.NA != "no sustain plateau" {
		t.Fatalf("reason lost: %q", m.NA)
	}
	if m.Scalar != 0 {
		t.Fatal("an unavailable measurement must not carry a misleading value")
	}
}

func TestVectorKeepsLabels(t *testing.T) {
	m := Vector([]float64{1, 0.5}, []string{"H1", "H2"})
	if m.Form != FormVector || len(m.Values) != 2 || m.Labels[1] != "H2" {
		t.Fatalf("got %+v", m)
	}
}

func TestTrackKeepsHop(t *testing.T) {
	m := Track([]float64{1, 2, 3}, 0.01)
	if m.Form != FormTrack || m.HopSec != 0.01 || len(m.Values) != 3 {
		t.Fatalf("got %+v", m)
	}
}
```

- [ ] **Step 2: Run and confirm it fails** — undefined `Scalar`, `Form`, etc.

- [ ] **Step 3: Implement**

```go
package fingerprint

// Form tags which of Measurement's payloads is populated.
type Form int

const (
	FormScalar Form = iota // one number
	FormVector             // a fixed profile: H1..H16, MFCC, band energies
	FormTrack              // a time series at HopSec resolution
)

// Measurement is the generic result of one descriptor extraction. A single
// float64 cannot carry the harmonic ladder, MFCC, band profiles or an
// envelope curve — which is why the pre-registry diagnose smuggled partial
// ratios out through a side field, outside the axis system entirely.
//
// OK=false is an HONEST not-applicable, never a misleading zero. It
// generalizes the discipline envelope_segments.go established with Silent /
// NoSustainPlateau / RingT60Extrapolated.
type Measurement struct {
	Form   Form
	Scalar float64
	Values []float64 // vector payload, or track samples
	Labels []string  // vector only; parallel to Values
	HopSec float64   // track only: seconds between Values entries
	OK     bool
	NA     string // why, when !OK
}

// Scalar builds an available scalar measurement.
func Scalar(v float64) Measurement { return Measurement{Form: FormScalar, Scalar: v, OK: true} }

// Vector builds an available vector measurement. labels may be nil.
func Vector(vs []float64, labels []string) Measurement {
	return Measurement{Form: FormVector, Values: vs, Labels: labels, OK: true}
}

// Track builds an available time-series measurement sampled every hopSec.
func Track(vs []float64, hopSec float64) Measurement {
	return Measurement{Form: FormTrack, Values: vs, HopSec: hopSec, OK: true}
}

// Unavailable builds a not-applicable measurement carrying the reason. Every
// numeric field stays zero so a caller that ignores OK cannot read a value
// that was never measured.
func Unavailable(reason string) Measurement { return Measurement{NA: reason} }
```

- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 3: `Delta` and the comparators

**Files:**
- Create: `src/go/internal/audio/fingerprint/compare.go`
- Test: `src/go/internal/audio/fingerprint/compare_test.go`
- Reference (copy classifiers from): `src/go/cmd/refs/diagnose.go:126-318`

**Interfaces:**
- Consumes: `Kind`, `Verdict`, `Measurement`.
- Produces: `Delta`; `func CompareByKind(k Kind, ref, synth Measurement) Delta`; `func classifyFraction(ref, synth, driftTh, mismatchTh float64) (Verdict, string)`; `func classifySeconds(ref, synth float64) (Verdict, string)`; `func classifyHz(ref, synth float64) (Verdict, string)`.

The three classifier functions are **moved verbatim** from `diagnose.go` — same
bodies, same thresholds, only `axisVerdict`→`Verdict` and the constant names
changed. That is what guarantees the acceptance test still passes.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import "testing"

func TestCompareSecondsRatio(t *testing.T) {
	// The mapping doc's harp example: 1.130s reference vs 0.160s synth = 7.1x short.
	d := CompareByKind(KindSeconds, Scalar(1.130), Scalar(0.160))
	if d.Verdict != VerdictMismatch {
		t.Fatalf("verdict: got %v want MISMATCH", d.Verdict)
	}
	if d.Direction != "low" {
		t.Fatalf("direction: got %q want low", d.Direction)
	}
	if d.Display != "7.1x short" {
		t.Fatalf("display: got %q", d.Display)
	}
}

func TestCompareHzPercent(t *testing.T) {
	// The mapping doc calibrates +36% as drift, not mismatch.
	d := CompareByKind(KindHz, Scalar(1000), Scalar(1360))
	if d.Verdict != VerdictDrift {
		t.Fatalf("got %v want drift", d.Verdict)
	}
	if d.Display != "+36%" {
		t.Fatalf("display: got %q", d.Display)
	}
}

func TestCompareFractionAbsolute(t *testing.T) {
	// The mapping doc's worked example: delta +0.341 -> MISMATCH.
	d := CompareByKind(KindFraction, Scalar(0.100), Scalar(0.441))
	if d.Verdict != VerdictMismatch || d.Direction != "high" {
		t.Fatalf("got %v/%q", d.Verdict, d.Direction)
	}
}

func TestCompareUnavailablePropagates(t *testing.T) {
	d := CompareByKind(KindFraction, Scalar(0.1), Unavailable("silent"))
	if d.OK {
		t.Fatal("comparing against an unavailable measurement must not be OK")
	}
	if d.NA != "silent" {
		t.Fatalf("reason lost: %q", d.NA)
	}
}

func TestCompareVectorL1Mean(t *testing.T) {
	d := CompareByKind(KindFraction,
		Vector([]float64{1, 0.5, 0.25}, nil),
		Vector([]float64{1, 0.5, 0.75}, nil))
	// mean |diff| = (0 + 0 + 0.5)/3
	if got := d.Norm; got < 0.166 || got > 0.167 {
		t.Fatalf("Norm: got %v want ~0.1667", got)
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**

- [ ] **Step 3: Implement** — `Delta` plus the dispatch. Copy
`classifyFraction`, `classifySeconds`, `classifyHz` bodies verbatim from
`diagnose.go` (lines 126–318), renaming `axisVerdict`→`Verdict`,
`verdictOK`→`VerdictOK`, `verdictDrift`→`VerdictDrift`,
`verdictMismatch`→`VerdictMismatch`, and replacing the package-level threshold
constants with `k.Thresholds()`. Then:

```go
// Delta is the comparison of one descriptor between reference and synth.
type Delta struct {
	Norm      float64 // scale-normalized magnitude, comparable ACROSS descriptors
	Signed    float64 // raw signed difference in the descriptor's own units
	Verdict   Verdict
	Direction string // "high" | "low" | "" (ok / undefined)
	Display   string // "7.1x short", "+36%", "+0.341"
	OK        bool
	NA        string
}

// CompareByKind is the default comparator for k. Scalars dispatch to the
// classifier matching the kind; vectors reduce to mean |difference| and tracks
// to curve RMS, both then classified on the KindFraction scale since they are
// already normalized magnitudes.
func CompareByKind(k Kind, ref, synth Measurement) Delta {
	if !ref.OK {
		return Delta{NA: ref.NA}
	}
	if !synth.OK {
		return Delta{NA: synth.NA}
	}
	switch ref.Form {
	case FormVector:
		return compareVector(ref, synth)
	case FormTrack:
		return compareTrack(ref, synth)
	}
	return compareScalar(k, ref.Scalar, synth.Scalar)
}
```

`compareScalar` fills `Signed = synth-ref`, dispatches to the classifier,
computes `Display` with the same three branches as `diagnose.go`'s
`formatDelta`, and sets `Norm` to `|Signed|/mismatchTh` for absolute kinds or
`max(r,1/r)/mismatchRatio` for ratio kinds — so `Norm >= 1` means "at or past
mismatch" for every descriptor uniformly. `compareVector` reduces to mean
absolute difference over the shorter length; `compareTrack` resamples the
shorter track to the longer's length by nearest-neighbour and takes RMS
difference. Both then run `classifyFraction` with `KindFraction` thresholds.

- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 4: `Bundle`

**Files:**
- Create: `src/go/internal/audio/fingerprint/bundle.go`
- Test: `src/go/internal/audio/fingerprint/bundle_test.go`

**Interfaces:**
- Consumes: `wave.Wave`, existing `FromWave`, `AnalyzeEnvelopeSegments`.
- Produces: `func NewBundle(w wave.Wave, label string) *Bundle`; methods `Wave() wave.Wave`, `Fingerprint() Fingerprint`, `Segments() EnvelopeSegments`, `STFT(size, hop int) *STFTFrames`, `SampleRate() int`.

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func testTone(sr int, hz, secs float64) wave.Wave {
	n := int(float64(sr) * secs)
	s := make([]float64, n)
	for i := range s {
		s[i] = 0.5 * mathSin(2*mathPi*hz*float64(i)/float64(sr))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestBundleMemoizesFingerprint(t *testing.T) {
	b := NewBundle(testTone(44100, 220, 2), "t")
	a1 := b.Fingerprint()
	a2 := b.Fingerprint()
	if a1.F0Hz != a2.F0Hz {
		t.Fatal("Fingerprint() is not stable across calls")
	}
	if b.fingerprintCalls != 1 {
		t.Fatalf("Fingerprint computed %d times, want 1", b.fingerprintCalls)
	}
}

func TestBundleSTFTMemoizedPerShape(t *testing.T) {
	b := NewBundle(testTone(44100, 220, 2), "t")
	_ = b.STFT(2048, 512)
	_ = b.STFT(2048, 512)
	_ = b.STFT(1024, 256)
	if b.stftCalls != 2 {
		t.Fatalf("STFT computed %d times, want 2 (one per distinct shape)", b.stftCalls)
	}
}
```

(`mathSin`/`mathPi` are thin file-local aliases for `math.Sin`/`math.Pi` to keep
the test import block short; declare them in the test file.)

- [ ] **Step 2: Run and confirm it fails.**

- [ ] **Step 3: Implement**

```go
// Bundle holds every artifact derived from one wave, computed at most once.
// Go's call graph is the dependency graph; memoization makes each artifact
// once-only, so no separate DAG engine is needed.
//
// NOT SAFE FOR CONCURRENT USE across goroutines beyond what the sync.Once
// guards provide: the counters exist for tests and are written unguarded.
type Bundle struct {
	w     wave.Wave
	label string

	fpOnce sync.Once
	fp     Fingerprint

	segOnce sync.Once
	seg     EnvelopeSegments

	stftMu sync.Mutex
	stfts  map[stftKey]*STFTFrames

	// test-only instrumentation
	fingerprintCalls int
	stftCalls        int
}

type stftKey struct{ size, hop int }

// STFTFrames is a magnitude-spectrogram: Frames[i] is one frame's linear
// magnitude spectrum, BinHz apart, with HopSec seconds between frames.
type STFTFrames struct {
	Frames [][]float64
	BinHz  float64
	HopSec float64
}
```

`NewBundle` stores the wave and initializes `stfts`. `Fingerprint()` runs
`FromWave(b.w, b.label)` under `fpOnce` and increments `fingerprintCalls`.
`Segments()` runs `AnalyzeEnvelopeSegments(b.w)` under `segOnce`. `STFT(size,
hop)` locks `stftMu`, returns a cached entry when present, otherwise builds
frames by sliding `wave.MagnitudeSpectrum` over the samples and increments
`stftCalls`.

- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 5: `Descriptor` and `Registry`

**Files:**
- Create: `src/go/internal/audio/fingerprint/descriptor.go`, `registry.go`
- Test: `src/go/internal/audio/fingerprint/registry_test.go`

**Interfaces:**
- Produces: `Scope`, `Repr`, `Descriptor` (**including a `Name string` field** — the
  human-readable axis label the report and the acceptance test key on, distinct
  from the stable `ID`), `Registry`, `func DefaultRegistry() *Registry`, `(*Registry).Register(Descriptor)`, `(*Registry).All() []Descriptor`, `(*Registry).ByID(string) (Descriptor, bool)`, `(*Registry).Filter(ScopeFilter) []Descriptor`;
  `func (d Descriptor) CompareWith(ref, synth Measurement) Delta` — uses
  `d.Compare` when non-nil, else falls back to `CompareByKind(d.Kind, ...)`;
  helper predicates `AlwaysApplies`, `RequiresPitched`, `RequiresSustain`, `RequiresPercussive`.

- [ ] **Step 1: Write the failing test**

```go
func TestRegistryRejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	d := Descriptor{ID: "x", Group: "g", Doc: "d", Extract: func(*Bundle) Measurement { return Scalar(1) }}
	r.Register(d)
	defer func() {
		if recover() == nil {
			t.Fatal("registering a duplicate ID must panic")
		}
	}()
	r.Register(d)
}

func TestRegistryDisciplineEveryDescriptorIsComplete(t *testing.T) {
	for _, d := range DefaultRegistry().All() {
		if d.ID == "" || d.Group == "" || d.Doc == "" {
			t.Errorf("descriptor %q: ID, Group and Doc are all required", d.ID)
		}
		if d.Extract == nil {
			t.Errorf("descriptor %q: Extract is required", d.ID)
		}
		if d.Applies == nil {
			t.Errorf("descriptor %q: Applies is required (use AlwaysApplies)", d.ID)
		}
	}
}

func TestRegistryAllIsSortedAndStable(t *testing.T) {
	a := DefaultRegistry().All()
	b := DefaultRegistry().All()
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatal("All() order is not stable")
		}
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**

- [ ] **Step 3: Implement** `Scope` (`ScopeWhole`, `ScopeSegment`, `ScopeBand`,
`ScopeTrack`, `ScopeAblationStage`, `ScopeVoiceTap`), `Repr` (`ReprTime`,
`ReprSpectrum`, `ReprLogFreq`, `ReprMel`, `ReprCepstral`, `ReprModulation`),
the `Descriptor` struct from the spec, and a `Registry` backed by a slice plus
an ID set. `Register` panics on duplicate IDs — a duplicate is a programming
error, not a runtime condition. `All()` returns a defensive copy sorted by
`(Group, ID)` so report output is deterministic. `DefaultRegistry()` is a
package singleton populated by `init()` in the `registry_*.go` files.

Applicability helpers:

```go
// AlwaysApplies is the explicit no-op predicate. Descriptors must set Applies
// rather than leaving it nil, so "applies everywhere" is a decision on the
// record rather than an omission.
func AlwaysApplies(*Bundle) bool { return true }

// RequiresPitched gates descriptors whose meaning depends on a harmonic
// series existing at all. Gated on the MEASURED signal, never on an
// instrument→family table, so it cannot go stale as instruments change.
func RequiresPitched(b *Bundle) bool { return b.Fingerprint().F0Hz > 0 }

// RequiresSustain gates descriptors defined only over a sustain plateau.
func RequiresSustain(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && !s.NoSustainPlateau && s.SustainSec > 0
}

// RequiresPercussive gates the kick/snare-family descriptors: a signal that
// peaks early and never establishes a plateau.
func RequiresPercussive(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && s.NoSustainPlateau
}
```

- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 6: Migrate the nine existing axes to registry rows

**Files:**
- Create: `src/go/internal/audio/fingerprint/registry_core.go`
- Test: `src/go/internal/audio/fingerprint/registry_core_test.go`

**Interfaces:**
- Consumes: `Descriptor`, `Bundle`, `Kind`.
- Produces: nine registered descriptors with these exact IDs — `noise.fraction`, `noise.flatness`, `env.attack`, `env.ring_t60`, `env.release`, `partial.inharmonicity`, `partial.series_deviation`, `bright.centroid`, `bright.rolloff`, `mod.sustain_variation`.

**Group and Name strings must match `diagnose.go` byte-for-byte** — the
acceptance test keys on them: groups `"noise / harmonic balance"`,
`"temporal envelope"`, `"partial structure"`, `"brightness / formant"`,
`"modulation"`; names `"noise fraction"`, `"spectral flatness"`, `"attack"`,
`"ring T60"`, `"release"`, `"inharmonicity"`, `"partial series deviation"`,
`"centroid"`, `"rolloff"`, `"sustain modulation"`.

- [ ] **Step 1: Write the failing test**

```go
func TestCoreDescriptorsCoverTheNineLegacyAxes(t *testing.T) {
	want := map[string]struct{ group, name string }{
		"noise.fraction":            {"noise / harmonic balance", "noise fraction"},
		"noise.flatness":            {"noise / harmonic balance", "spectral flatness"},
		"env.attack":                {"temporal envelope", "attack"},
		"env.ring_t60":              {"temporal envelope", "ring T60"},
		"env.release":               {"temporal envelope", "release"},
		"partial.inharmonicity":     {"partial structure", "inharmonicity"},
		"partial.series_deviation":  {"partial structure", "partial series deviation"},
		"bright.centroid":           {"brightness / formant", "centroid"},
		"bright.rolloff":            {"brightness / formant", "rolloff"},
		"mod.sustain_variation":     {"modulation", "sustain modulation"},
	}
	for id, w := range want {
		d, ok := DefaultRegistry().ByID(id)
		if !ok {
			t.Errorf("missing descriptor %q", id)
			continue
		}
		if d.Group != w.group || d.Name != w.name {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", id, d.Group, d.Name, w.group, w.name)
		}
	}
}

func TestSilentWaveYieldsUnavailableNotZero(t *testing.T) {
	b := NewBundle(wave.Wave{Samples: make([]float64, 44100), SampleRate: 44100}, "silent")
	d, _ := DefaultRegistry().ByID("env.attack")
	m := d.Extract(b)
	if m.OK {
		t.Fatal("a silent wave must yield an unavailable envelope measurement")
	}
	if m.NA == "" {
		t.Fatal("unavailable measurements must carry a reason")
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**

- [ ] **Step 3: Implement** the nine rows in an `init()`. Each is a literal;
extraction reads `b.Fingerprint()` or `b.Segments()`. The envelope rows guard
on `Silent` and return `Unavailable("signal is silent")`. Example row:

```go
func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID:    "noise.fraction",
		Group: "noise / harmonic balance",
		Name:  "noise fraction",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Doc: "Fingerprint.NoiseRatio: 1 - harmonic energy / total energy, using " +
			"narrow +/-1-bin harmonic notches. Preferred over " +
			"NoiseProfile.ResidualRatio, whose wider tolerance bands fold real " +
			"noise into 'harmonic' and under-detect breathiness on winds.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement { return Scalar(b.Fingerprint().NoiseRatio) },
	})
}
```

The remaining eight rows follow the identical literal shape. Their extractions,
in order: `noise.flatness` → `b.Fingerprint().SpectralFlatness` (KindFraction);
`env.attack` → `b.Segments().AttackSec` (KindSeconds, guard `Silent`);
`env.ring_t60` → `b.Segments().RingT60Sec` (KindSeconds, guard `Silent`, custom
`Compare`); `env.release` → `b.Segments().ReleaseSec` (KindSeconds, guard
`Silent`); `partial.inharmonicity` → `b.Fingerprint().Inharmonicity`
(KindTinyFraction); `partial.series_deviation` →
`b.Fingerprint().PartialRatioProfile.SeriesDeviation` (KindSeriesDeviation);
`bright.centroid` → `b.Fingerprint().SpectralCentroid` (KindHz);
`bright.rolloff` → `b.Fingerprint().SpectralRolloff` (KindHz);
`mod.sustain_variation` → `b.Segments().SustainVariation` (KindFraction, guard
`Silent`). Each carries a `Doc` naming the field it reads and why that field
rather than a neighbouring one.

`env.ring_t60` keeps its bespoke `Compare` — the both-sides-extrapolated
escalation to `OverallDecayRateDBPerSec` from `ringT60Axis`, moved in as the
descriptor's `Compare` override rather than special-cased in the renderer.

- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 7: Rewire `diagnose` onto the registry

**Files:**
- Modify: `src/go/cmd/refs/diagnose.go`
- Test (must pass UNCHANGED): `src/go/cmd/refs/diagnose_acceptance_test.go`

**Interfaces:**
- Consumes: `DefaultRegistry()`, `NewBundle`, `CompareByKind`, `Verdict`.
- Produces: unchanged CLI output.

- [ ] **Step 1: Add type aliases so existing code compiles untouched**

```go
type axisVerdict = fingerprint.Verdict
type axisKind = fingerprint.Kind

const (
	verdictOK       = fingerprint.VerdictOK
	verdictDrift    = fingerprint.VerdictDrift
	verdictMismatch = fingerprint.VerdictMismatch
)
```

- [ ] **Step 2: Run the acceptance test before touching anything else**

`cd src/go && ../../.tools/go/bin/go test ./cmd/refs/ -run TestDiagnoseAcceptance -v`
Expected: PASS (baseline — record the output).

- [ ] **Step 3: Replace the hand-written axis block with a registry loop**

Delete the nine `mkAxis`/`out.Axes = append(...)` calls in `diagnoseInstrument`
and the now-dead `classifyFraction`/`classifySeconds`/`classifyHz`/`mkAxis`
bodies (they live in `fingerprint` now). Replace with:

```go
refB := fingerprint.NewBundle(refWave, res.Ref.ID)
synthB := fingerprint.NewBundle(synthWave, inst)

for _, d := range fingerprint.DefaultRegistry().All() {
	if !d.Applies(refB) {
		continue
	}
	delta := d.CompareWith(d.Extract(refB), d.Extract(synthB))
	out.Axes = append(out.Axes, axisResult{
		Group: d.Group, Name: d.Name, Kind: d.Kind,
		Ref: /* scalar or NaN */, Synth: /* … */,
		Verdict: delta.Verdict, Direction: delta.Direction,
		Note: axisNote(d.Name, delta.Direction),
	})
}
```

Applicability is evaluated on the **reference** bundle: the reference defines
what kind of instrument this is. Keep the existing series-deviation caveat
logic and the `RefPartialRatios`/`SynthPartialRatios` fields for now — Task 8
of the Phase 4–8 plan converts those to vector descriptors.

- [ ] **Step 4: Run the acceptance test and diff against the Step 2 baseline**

`cd src/go && ../../.tools/go/bin/go test ./cmd/refs/ -run TestDiagnoseAcceptance -v`
Expected: PASS, with byte-identical verdicts. **If any verdict changed, the
refactor is wrong — do not adjust the test.**

- [ ] **Step 5: Run the whole `cmd/refs` suite**

`cd src/go && ../../.tools/go/bin/go test ./cmd/refs/`
Expected: PASS.

- [ ] **Step 6: Verify no import cycle was introduced**

`cd src/go && ../../.tools/go/bin/go vet ./internal/audio/fingerprint/ ./internal/audio/ ./cmd/refs/`
(untagged — this is the ONLY way the cycle is visible)

- [ ] **Step 7: Checkpoint.**

---

## Phase 2 — Reconnect the orphaned analyzers

### Task 8: Snare, piano and wind descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/registry_family.go`
- Test: `src/go/internal/audio/fingerprint/registry_family_test.go`

**Interfaces:**
- Consumes: `AnalyzeSnare`, `AnalyzePiano`, `ComputeWindAttack`,
  `SpectralIrregularity`, `FormantBandEnergies` — all currently with zero
  non-test callers.
- Produces: `Bundle` accessors `Snare()`, `Piano()`, `WindAttack()`, plus
  registered descriptors under groups `"percussive body"`, `"struck string"`,
  `"wind attack"`.

- [ ] **Step 1: Write the failing test** asserting that (a) a percussive
      synthetic burst gets snare descriptors and a sustained tone does not,
      and (b) every family descriptor's `Applies` is non-nil and gated:

```go
func TestFamilyDescriptorsAreGated(t *testing.T) {
	tone := NewBundle(testTone(44100, 220, 2), "tone")
	for _, d := range DefaultRegistry().All() {
		if d.Group != "percussive body" {
			continue
		}
		if d.Applies(tone) {
			t.Errorf("%s must not apply to a sustained tone", d.ID)
		}
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**
- [ ] **Step 3: Add the `Bundle` accessors (memoized, same pattern as Task 4)
      and register the rows** with `RequiresPercussive` / `RequiresPitched`
      gating. No new DSP — these are wrappers over existing functions.
- [ ] **Step 4: Run the fingerprint suite; confirm green.**
- [ ] **Step 5: Run `cmd/refs` acceptance again** — the new rows must not
      change any of the ten graded instruments' existing axis verdicts.
- [ ] **Step 6: Checkpoint.**

---

## Phase 3 — Whole-wave frame tracks

### Task 9: Frame-track extraction

**Files:**
- Create: `src/go/internal/audio/fingerprint/tracks.go`
- Test: `src/go/internal/audio/fingerprint/tracks_test.go`

**Interfaces:**
- Produces: `type Tracks struct { HopSec float64; RMS, Crest, Centroid, Rolloff, Flatness, NoiseFraction, F0, F0Confidence []float64; Harmonics [16][]float64 }`; `(*Bundle) Tracks() *Tracks`.

- [ ] **Step 1: Write the failing known-answer test**

```go
func TestCentroidTrackIsFlatForSteadyTone(t *testing.T) {
	tr := NewBundle(testTone(44100, 440, 2), "t").Tracks()
	_, std := meanStd(tr.Centroid)
	mean, _ := meanStd(tr.Centroid)
	if mean <= 0 {
		t.Fatal("centroid track is empty or zero")
	}
	if std/mean > 0.05 {
		t.Fatalf("steady tone centroid should be near-constant; CoV=%v", std/mean)
	}
}

func TestRMSTrackFollowsAnAmplitudeRamp(t *testing.T) {
	w := testTone(44100, 440, 2)
	for i := range w.Samples { // linear fade-in
		w.Samples[i] *= float64(i) / float64(len(w.Samples))
	}
	tr := NewBundle(w, "ramp").Tracks()
	if tr.RMS[0] >= tr.RMS[len(tr.RMS)-1] {
		t.Fatal("RMS track did not follow the ramp")
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**
- [ ] **Step 3: Implement** over `b.STFT(2048, 512)`, reusing the existing
      `SpectralCentroid`, `spectralFlatnessOf` and band helpers rather than
      re-deriving them. Guard every division with the existing `1e-12` epsilon
      convention.
- [ ] **Step 4: Run and confirm green.**
- [ ] **Step 5: Checkpoint.**

---

### Task 10: `TrackStats` and the track descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/trackstats.go`, `registry_tracks.go`
- Test: `src/go/internal/audio/fingerprint/trackstats_test.go`

**Interfaces:**
- Produces: `type TrackStats struct { Mean, Median, Std, IQR, P10, P90, Min, Max, Range, Slope, Curvature, MonotonicFrac float64 }`; `func StatsOf(track []float64, hopSec float64) TrackStats`; one descriptor per (track × reported statistic).

- [ ] **Step 1: Write the failing known-answer test**

```go
func TestStatsOfLinearRamp(t *testing.T) {
	// 0..1 over 1 second at hop 0.1s => slope 1.0/sec, monotonic, median 0.5.
	tr := []float64{0, .1, .2, .3, .4, .5, .6, .7, .8, .9, 1}
	s := StatsOf(tr, 0.1)
	if math.Abs(s.Slope-1.0) > 1e-9 {
		t.Errorf("slope: got %v want 1.0", s.Slope)
	}
	if math.Abs(s.Median-0.5) > 1e-9 {
		t.Errorf("median: got %v want 0.5", s.Median)
	}
	if math.Abs(s.MonotonicFrac-1.0) > 1e-9 {
		t.Errorf("monotonic fraction: got %v want 1.0", s.MonotonicFrac)
	}
	if math.Abs(s.Range-1.0) > 1e-9 {
		t.Errorf("range: got %v want 1.0", s.Range)
	}
}

func TestStatsOfEmptyTrackIsZeroNotNaN(t *testing.T) {
	s := StatsOf(nil, 0.1)
	if math.IsNaN(s.Mean) || math.IsNaN(s.Slope) || math.IsNaN(s.Std) {
		t.Fatal("empty track must yield zeros, never NaN")
	}
}
```

- [ ] **Step 2: Run and confirm it fails.**
- [ ] **Step 3: Implement.** Reuse the existing `olsSlope`, `median`,
      `percentile` and `meanStd` helpers in `signal.go`/`vibrato.go` — do not
      write new copies. `Curvature` is the quadratic coefficient of a
      second-order least-squares fit; `MonotonicFrac` is the fraction of
      consecutive deltas sharing the majority sign.
- [ ] **Step 4: Register the track descriptors** under group
      `"whole-signal statistics"`, IDs `track.<name>.<stat>` (e.g.
      `track.centroid.slope`). Register only the statistics that carry
      diagnostic meaning per track — the full cross-product is available to
      the Phase 6 orthogonality analysis, which decides what to keep.
- [ ] **Step 5: Verify the re-expression invariant**

```go
func TestCentroidSlopeMatchesLegacyTemporalFingerprint(t *testing.T) {
	b := NewBundle(testTone(44100, 440, 2), "t")
	legacy := b.Fingerprint().Temporal.CentroidSlope
	got := StatsOf(b.Tracks().Centroid, b.Tracks().HopSec).Slope
	if rel := math.Abs(got-legacy) / math.Max(math.Abs(legacy), 1); rel > 0.20 {
		t.Fatalf("re-expressed centroid slope %v diverges from legacy %v", got, legacy)
	}
}
```

The 20% tolerance is deliberate: `computeTemporal` skips the first 0.1 s and
trims to 1024 bins, while `Tracks` covers the whole signal at 1025. They should
agree in magnitude and sign, not exactly.

- [ ] **Step 6: Run the full fingerprint + cmd/refs suites; confirm green.**
- [ ] **Step 7: Checkpoint.**

---

## Verification (run after every phase)

```bash
cd src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/
../../.tools/go/bin/go test ./cmd/refs/
../../.tools/go/bin/go vet ./internal/audio/fingerprint/ ./internal/audio/ ./cmd/refs/
../../.tools/go/bin/gofmt -l internal/audio/fingerprint/ cmd/refs/
```

All four must be clean. The `vet` line is untagged on purpose — it is the only
way the `fingerprint` → `internal/audio` import cycle becomes visible.
