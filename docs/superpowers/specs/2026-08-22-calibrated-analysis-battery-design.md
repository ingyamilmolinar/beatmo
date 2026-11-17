# SP2 — Calibrated Analysis Battery

**Date:** 2026-08-22
**Program:** `2026-08-19-reference-grade-templates-program-design.md` (SP2)
**Depends on:** SP1 (`2026-08-19-reference-audio-corpus-design.md`) — shipped
**Builds on:** `2026-08-22-verdict-to-metric-mapping.md`,
`2026-08-21-ear-verdict-round-1-findings.md`
**Status:** DESIGN — awaiting review.

## Problem

Instrument tuning against reference recordings is measured today by a
diagnostic that is correct in shape but narrow in coverage, and by a
library of metrics that is broad but partly disconnected and entirely
unvalidated.

**The measurement surface is nine axes.** `refs diagnose` reports noise
fraction, spectral flatness, attack, ring T60, release, inharmonicity,
partial series deviation, centroid, rolloff, and sustain modulation.
Those nine were derived from ten annotated complaints. They are the axes
that explain the failures we happened to write down, not a principled
description of a waveform.

**Most of the library never runs.** `AnalyzeSnare`, `AnalyzePiano`,
`ComputeWindAttack`, `SpectralIrregularity`, `FormantBandEnergies`,
`KickAnalyze` and `KickSegmentAnalyze` have **zero non-test callers**.
The DSP exists; nothing consumes it.

**Analysis is still a one-second snapshot.** `Fingerprint` computes
spectral shape, MFCC, partials, formants, vibrato, coupling and noise
over a single window (`SustainSkipSec` 0.3 s → `SustainLenSec` 1.3 s).
Only `computeTemporal` and `MacroCentroidRMSCorr` see the whole signal.
`AGENTS.synth.md` records this exact failure — the trumpet's identity
lived in its time evolution and a mid-sustain FFT threw it away — and
the structure that caused it is unchanged.

**Orthogonality is a prose claim.** `distanceWeights` carries comments
asserting that terms are "partially redundant"; no correlation analysis
has ever been run. There are three uncoordinated notions of distance
(`Distance`, `TimbreDistance`, the family analyzers) and no measurement
of how much independent information the battery actually carries.

**Robustness is unhandled.** SP1 documented three traps in the corpus
itself: sixteen references are 128 kbps MP3 previews whose HF above
14 kHz is codec-limited, some references are truncated by
`convert.max_sec`, and `DetectF0` has reproducible octave errors on low
bowed strings and detuned-unison samples. `diagnose` handles exactly one
of these, ad hoc, for one axis.

**Nothing is calibrated.** Forty-four ear verdicts exist in
`internal/audio/testdata/ear_verdicts.jsonl`. No metric has been checked
against them. The precedent for not checking is expensive: `synth-match`
improved fingerprint distance on winds and brass by +17…+42% and every
one of those instruments was then judged bad by ear.

## Goal

One generic measurement API; one registry that is the single source of
truth for every descriptor; whole-signal, segment-resolved and
stage-resolved coverage; and three committed validation harnesses that
make "comprehensive", "orthogonal" and "robust" testable properties
rather than assertions.

## Non-goals

- **No composite score, and no fitted weights anywhere.** Forty-four
  labels against ~150 descriptors would produce spurious winners.
  `2026-08-22-verdict-to-metric-mapping.md` establishes that collapsing
  axes into one scalar is the mechanism behind the winds regression.
- **No optimizer rewiring.** `synthmatch`'s objective is untouched.
- **No mixer pipeline stages** (Synth → AntiPop → InsertFX → EQ → Sends
  → Master). For a solo-instrument A/B most of those are pass-through;
  they belong to SP5.
- **No instrument retuning.** This spec ships the instrument. SP4 does
  the tuning. Fixing the oboe "while we are here" is how this blows
  scope.

## Design

### 1. The measurement API

A `Descriptor` declares everything about one measurement. A `Bundle`
holds everything computed once per wave, so extractors are lookups and
no transform runs twice.

```go
type Descriptor struct {
    ID       string   // stable key: "noise.fraction", "seg.attack.centroid"
    Group    string   // "noise / harmonic balance", "temporal envelope", …
    Scope    Scope    // Whole | Segment | Band | Track | AblationStage | VoiceTap
    Repr     Repr     // Time | Spectrum | LogFreq | Mel | Cepstral | Modulation
    Kind     Kind     // Fraction | TinyFraction | Seconds | Hz | dB | Ratio | SeriesDeviation
    Drift, Mismatch float64  // thresholds in Kind's units; zero → Kind default
    Doc      string            // required, non-empty
    Applies  func(*Bundle) bool
    Extract  func(*Bundle) Measurement
    Compare  func(ref, synth Measurement) Delta  // nil → Kind's default comparator
}

// Delta is the comparison result: a scale-normalized magnitude used for
// ranking and gating, the raw signed difference, a verdict, and the
// human-readable rendering ("7.1x short", "+36%", "+0.341").
type Delta struct {
    Norm     float64      // scale-normalized magnitude, comparable across descriptors
    Signed   float64      // raw signed difference in the descriptor's own units
    Verdict  axisVerdict  // ok | drift | MISMATCH
    Direction string      // "high" | "low" | ""
    Display  string
}
```

There is deliberately **no `Nuisance` field on `Descriptor`**. An earlier
draft declared sensitivity by hand; §4(b) measures it instead and commits
the result as a golden, so a descriptor cannot claim robustness it does
not have.

Measurements are not scalars. The harmonic ladder, MFCC, band profiles,
partial ratios and envelope curves are vectors and tracks; today
`diagnose` smuggles `RefPartialRatios` out through a side field, outside
the axis system entirely. The generic form:

```go
type Form int // FormScalar | FormVector | FormTrack

type Measurement struct {
    Form   Form
    Scalar float64
    Values []float64  // vector (partials, bands, MFCC) or track samples
    Labels []string   // vector only: "H1".."H16", band names
    HopSec float64    // track only
    OK     bool
    NA     string     // why, when !OK
}
```

**Measurement is separated from comparison.** `Kind` selects a default
comparator — `AbsDelta` for fractions, `RatioDelta` for seconds,
`PctDelta` for Hz, `L1Mean` and `CosineShape` for vectors, curve-RMS for
tracks. A descriptor may override with its own `Compare`. The common
case is one declarative row with no code; anything exotic remains
expressible. This is what makes the API generic: any future analysis is
a row plus, at most, a comparator.

**`OK=false` is honest N/A, never a misleading zero.** It generalizes
the discipline `envelope_segments.go` already established with `Silent`,
`NoSustainPlateau` and `RingT60Extrapolated`. A silent clip, a
percussive hit with no plateau, or an unpitched source with no reliable
F0 suppresses its dependent descriptors and reports why.

**`Applies` gates on measured signal character** — pitched vs unpitched,
sustained vs decaying, percussive — not on a hardcoded instrument→family
map. Kick descriptors must not run on a violin, and a self-classifying
predicate cannot go stale the way a table would.

**`Bundle` uses lazily memoized typed accessors** (`b.STFT(2048, 512)`,
`b.Segments()`, `b.CQT()`, `b.Tracks()`). Go's call graph is the
dependency graph and memoization makes each artifact once-only; no DAG
engine is required. Adding an analysis is one accessor. `Bundle` is
per-wave and single-goroutine; each artifact is guarded by a
`sync.Once` so that contract is enforced rather than documented.

#### Code placement

| Package | Contents | Why |
|---|---|---|
| `internal/audio/fingerprint` | `descriptor.go`, `registry.go`, `bundle.go`, the new analyzers | Imports only `internal/wave` + stdlib, by design, to avoid the `internal/audio` cycle. Nothing in `internal/ui` imports it, so there is no WASM-size exposure. |
| `internal/audio/synthdiag` (new) | ablation runner, tap-render driver | These render audio, so they cannot live in `fingerprint` without closing the cycle its package doc exists to prevent. |
| `cmd/refs/diagnose.go` | renderer over the registry | The nine current axes survive as rows with their existing thresholds. |

### 2. The battery

#### (a) Whole-wave statistics

A **frame-track layer** computes per-frame tracks of RMS, crest,
centroid, rolloff, flatness, spread, skewness, kurtosis, noise fraction,
HNR, F0, F0 confidence, H1–H16 dB, tristimulus and MFCC. One uniform
`TrackStats` then applies to every track: mean, median, std, IQR,
p10/p90, range, OLS slope, curvature, monotonic fraction.

This is the direct fix for the one-second-snapshot problem, and it
subsumes existing scalars as special cases — `CentroidMean`,
`CentroidRange` and `CentroidSlope` become `stats(centroid track)` —
which is itself an orthogonality win.

#### (b) Segment-resolved metrics

`kick_segments.go` argues boundaries must be **fixed** across signals,
because envelope-derived bounds shift when the signals differ and
fabricate divergence. `envelope_segments.go` argues they must be
**per-signal**, because a harp and an oboe share no envelope shape.
Both are correct.

**Resolution: derive boundaries from the reference, apply them
onset-aligned to both signals.** Concretely: run
`AnalyzeEnvelopeSegments` on the reference to obtain segment
*durations*; detect each signal's own onset independently (first
crossing of `envAttackOnsetFrac`); then lay the reference's durations
down from each signal's own onset. Both signals are analyzed over spans
of identical length, measured from the same perceptual origin, while the
span lengths remain chosen per instrument. Neither signal's own envelope
can shift the other's boundaries, which is the property
`kick_segments.go` requires.

Segments: onset, attack, decay, sustain, release, tail. Each carries
energy fraction, centroid, flatness, noise fraction, crest, H1–H16,
MFCC, band energies and duration. This generalizes `KickSegment` off its
fixed 35/150 ms bounds; the fixed-boundary kick variant stays, gated by
a percussive `Applies`.

#### (c) Per-band decay

`BandT60` is kick-only. Generalize to per-octave-band T60, decay slope
and energy fraction. This resolves the harp / hihat / tubular-bells
complaints *per band*, and retires the strain visible in
`envelope_segments.go`'s `OverallDecayRateDBPerSec` comment, where a
bell's fast strike transient trips the plateau detector and hides the
long hum tail.

#### (d) Modulation domain

FFT of each frame-track yields dominant modulation rate, depth at that
rate, energy in 0.5–2 Hz (gesture), 2–8 Hz (vibrato/tremolo) and
8–30 Hz (warble) bands, and **modulation-spectrum flatness** — a shaped
gesture is peaky, chaos is flat.

This is the discriminator the program doc names as missing and which
failure mode 2 requires: the violin's amplitude variation was tuned to
std/mean 0.40 against a real 0.64 and still sounded wrong, because the
real signal is a slow bow-stroke shape and the synth's was fast chaos.
Magnitude matched; character was opposite. It also covers
tubular-bells' "wavy… prolongs and waves in time".

#### (e) Partial decorrelation

Per-partial bandwidth (−3 dB width relative to the analysis bin
resolution), inter-partial phase coherence, and beating rate and depth
between close partials. No metric exists today. This is the measurable
form of the PADsynth argument recorded in `AGENTS.synth.instruments.md`:
phase-locked partials read as artificial, and a real harmonic is a
narrow band of slightly detuned, randomly phased components.

#### (f) Log-frequency / constant-Q

A 16384-point linear FFT gives 2.7 Hz bins — fine at 5 kHz, relatively
coarse at 60 Hz, while the corpus spans more than three octaves. CQT
adds pitch-invariant log-spaced resolution. Descriptors: CQT
log-spectral distance, CQT centroid and spread, octave-band profile.

#### (g) Reconnecting the orphans

`AnalyzeSnare`, `AnalyzePiano`, `ComputeWindAttack`,
`SpectralIrregularity`, `FormantBandEnergies`, `KickAnalyze`,
`KickSegmentAnalyze`, and `KickFingerprint`'s roughness, flux, warble,
inharmonicity and MPEG-7 attack descriptors all become registry rows
with applicability predicates. Zero new DSP; they stop being code that
only their own tests call.

Total: roughly 150 descriptors. A violin report shows the ~60 that
apply, not all 150.

### 3. Stage attribution

Three mechanisms, sequenced cheapest-first.

#### (a) Synth-stage ablation — no engine change

Render the instrument in full, then once per stage with that stage
forced to identity, level-matched, and diff every descriptor.

**Identity is per-stage, not a flag.** `modular.h` has twelve
`*_enabled` toggles (osc, fm, env, filter, drive, post, pitchenv, lfo,
burst, filtenv, kick, formant), but several Synth-tab stages are
identity-*by-value*: ENSEMBLE is `unison_voices=1, unison_detune=0,
unison_drift_rate=0, unison_drift_depth=0`; RESONATOR is `body_mix=0`;
each gen slot is its own `genN_gain=0`. The ablation table therefore
maps stage → an identity **override set**. Ablating gen slots
individually is the highest-value case: it is the technique that solved
the violin "too much bass" bug by hand.

**RMS-match every ablation** to the full render before comparing, reusing
`computeCompare`'s `matchRMS`. Removing a stage changes loudness; without
this, every ablation reads as "everything moved". Descriptors that are
themselves level-dependent are the exception and are compared
pre-normalization, flagged as such in the registry.

**Ablation measures sensitivity, not contribution.** Stages interact —
a filter downstream of a formant — so one-at-a-time attribution is not
additive. The report says "sensitivity" and never implies a
decomposition that sums.

Output: a stage × descriptor sensitivity matrix, plus the readout that
matters — for each axis currently at MISMATCH, which stage has the most
leverage on it.

#### (b) C-level voice taps — sequenced last

`render_modular_p` is one flat function whose stages each operate on the
whole `out[]` buffer in sequence: OSC → gen bank → LFO → BURST → amp
ADSR → FILTER → FORMANT → body resonator → drive+gain → POST. A tap is
a `memcpy` of `out[]` after each block.

- New export `render_modular_tapped(out, sr, samples, params, taps,
  tapCount)`. `render_modular_p` becomes a one-line delegate passing
  `taps=NULL`, so every existing caller is untouched by construction.
- Tap IDs are an append-only enum mirrored in `modular_c.go` and
  regenerated into JS by `cmd/gen-synth-abi`, following the existing
  param-add checklist.
- **The audio goldens are the gate**: they must stay byte-identical.
  Per `AGENTS.synth.md`, C edits need `make build/libdrums.a` plus
  `go clean -cache`, or the static lib silently ships stale.
- Offline-only; the browser path never allocates tap buffers.

Sequenced last deliberately. Ablation is free and answers which stage
has leverage; taps answer what the signal looks like entering and
leaving it. Running ablation first will show which tap points are worth
the C change, so the invasive budget is spent with evidence rather than
ahead of it.

#### (c) Transform-stage reporting

Every descriptor declares `Repr`, so the report pivots by
representation: time, linear spectrum, constant-Q, mel, cepstral,
modulation. One aggregate distance per representation becomes a
first-class row (waveform distance, linear LSD, CQT LSD, MFCC L2,
cepstral distance, modulation-spectrum distance) so the pivot has a
summary line. These are views of one signal and so are correlated by
construction; §4(a) measures how much, which is an output rather than a
defect.

### 4. Validation

#### (a) Orthogonality — measured and ratcheted

Over a fixed evaluation set (55 committed references, their synth
renders, and the ablation variants), compute the Spearman |ρ| matrix
across descriptors and report clusters at |ρ| ≥ 0.9 plus the **effective
rank** — one honest number for how many independent dimensions the
battery spans. Defined as the participation ratio of the correlation
matrix's eigenvalues, `(Σλᵢ)² / Σλᵢ²`, which is continuous (so the
ratchet can detect small losses) and needs no arbitrary variance cutoff.
The raw eigenvalue spectrum is committed alongside it.

Every ≥0.9 cluster must either collapse to one representative or carry a
written justification. Noise fraction vs spectral flatness is the
precedent for keeping a correlated pair: the mapping doc's snare
correction showed they are diagnostic precisely when they disagree.

Committed as `testdata/orthogonality.json` with a ratchet — effective
rank may not drop, and no new ≥0.9 pair may appear unannounced. This
mirrors the `token_discipline_test.go` pattern already in the repo.

This is empirical redundancy *on this corpus*, not mathematical
independence, and Spearman over 55 points is noisy. The threshold is
0.9, not 0.7, for that reason.

#### (b) Robustness — sensitivity is measured, not declared

Sweep every descriptor against transformations that must not change a
timbre judgment: ±12 dB level, ±3 semitones, truncation to 1/2/3 s, a
128 kbps MP3 round-trip (modelling the sixteen preview references),
44.1 → 48 → 44.1 kHz resampling, an added −60 dBFS noise floor, and a
±20 ms onset shift.

A descriptor whose nuisance sensitivity exceeds its between-instrument
spread is **fragile** and is demoted to diagnostic-only. The resulting
sensitivity table is committed as a golden.

Three distinct gates exist and must not be conflated:

| Gate | Question | Effect |
|---|---|---|
| `Applies` (§1) | Does this descriptor mean anything for *this kind of signal*? | Row omitted entirely |
| `OK=false` (§1) | Was it measurable on *this clip*? | Row shown as N/A with a reason |
| Fragility (§4b) / discrimination (§4c) | Is this descriptor *trustworthy*? | Row shown, but excluded from the "worst offending axes" ranking |

Additionally, a per-clip **suppression** applies when a specific
nuisance is known present rather than merely possible — an MP3-preview
reference suppresses the HF-sensitive descriptors the golden identifies,
a truncated reference suppresses the decay-tail descriptors, and a
low-F0-confidence clip suppresses the harmonic-series descriptors. These
surface as `OK=false` with the reason named.

#### (c) Calibration — per-axis gate, two tests

`ear_verdicts.jsonl` holds 44 entries in **two label types**:

- **37 absolute grades** — 7 `y`, 21 `p`, 9 `n`.
- **7 preferences** — `conga`, `flute`, `french-horn`, `guitar-nylon`,
  `tom`, `trumpet`, `violin`, where the listener chose among variants.

**Test 1 (absolute):** Spearman ρ between each descriptor's `Delta.Norm`
— already scale-normalized by its `Kind` comparator, which is what makes
values comparable across descriptors — and the ordinal grade (y=0, p=1,
n=2), plus ✅-vs-❌ AUC.

**Test 2 (preference):** the descriptor must rank the chosen variant
closer to the reference than the rejected one. These seven are
within-instrument comparisons with pitch, level and reference held
constant, so they are far less confounded than absolute grades. Test 2
is the stronger gate and is weighted accordingly.

An axis that fails to separate is **demoted, not deleted** — still
computed and displayed, but excluded from the "worst offending axes"
ranking that directs tuning attention. It may be the right axis for an
instrument this corpus does not cover.

Anti-overfitting is structural: there are no fitted weights anywhere.
Promotion additionally requires the AUC to survive leave-one-family-out.

**Calibration decay.** These verdicts describe the renders as they
sounded on 2026-08-21. When SP4 retunes an instrument its verdict goes
stale. The golden records a recipe hash per instrument, and the test
warns when a graded instrument's recipe has changed since its verdict.

#### (d) The existing acceptance gate stays hard

`diagnose_acceptance_test.go`'s ten-instrument primary and bonus verdict
tables remain the functional gate: the battery must independently
rediscover what the listener said, with the correct sign. Growing from
nine to ~150 descriptors must not regress those ten.

### 5. Report surface

`refs diagnose` gains `-scope`, `-group` and `-repr` filters, plus
`-ablate`, `-all` and `-json`. The **default output stays today's
compact per-group table**: triage must not get worse because the battery
got bigger.

### 6. Testing

- Known-answer unit tests per analyzer using `internal/wave/testgen.go`
  — a pure sine has flatness ≈ 0; a 5 Hz tremolo's modulation spectrum
  peaks at 5 Hz.
- A registry discipline test: every descriptor has a non-empty `Doc`,
  thresholds, an `Applies`, and an entry in the calibration golden.
- The three corpus goldens above.
- Fast path via `-tags test -modfile=go.test.mod`. Render-dependent
  parts are `!test` CGo, matching `physical_models_test.go`.
- **Never bare `./...`** — phantom duplicate `internal/ui`, and `cmd/*`
  legitimately fails under `-tags test`. Name packages explicitly.

## Implementation order

This spec is large enough to need a spine. Each phase is independently
shippable and leaves the tool working.

| Phase | Contents | Gate |
|---|---|---|
| 1 | `Descriptor` / `Measurement` / `Delta` / `Bundle`; registry; `diagnose` refactored to render it; the nine current axes migrated as rows | `diagnose_acceptance_test.go` passes unchanged — zero verdict changes |
| 2 | Reconnect the orphans (§2g) as rows with `Applies` predicates | Snare/piano/wind/kick rows appear for the right instruments and nowhere else |
| 3 | Frame-track layer + `TrackStats` (§2a); existing scalars re-expressed as track statistics | Re-expressed scalars reproduce their former values within tolerance |
| 4 | Segment grid (§2b) and per-band decay (§2c) | Harp / hihat / tubular-bells decay complaints localize to a band and a segment |
| 5 | Modulation domain (§2d), partial decorrelation (§2e), constant-Q (§2f) | Known-answer tests: a 5 Hz tremolo peaks at 5 Hz; phase-locked additive scores minimum decorrelation |
| 6 | Validation harnesses (§4a–c) + goldens | Effective rank committed; every descriptor carries a fragility and a discrimination result |
| 7 | Synth-stage ablation (§3a) | Sensitivity matrix reproduces the violin gen-slot finding that was previously found by hand |
| 8 | C-level voice taps (§3b) | Audio goldens byte-identical |

Phases 6 and 7 are where the spec pays for itself, and phase 8 is the
only one carrying engine risk. If phase 7's sensitivity matrix shows the
stage attribution is already actionable without sample-exact taps, phase
8 should be re-evaluated before it is started rather than executed on
momentum.

## Risks

| Risk | Mitigation |
|---|---|
| ~150 descriptors is an unreadable report | `Applies` gating, group/scope/repr filters, compact default view |
| C tap work breaks audio goldens | Sequenced last; `render_modular_p` delegates with `taps=NULL`; goldens are the gate |
| Calibration decays as SP4 retunes | Recipe-hash provenance in the golden; test warns on drift |
| Spurious calibration winners from 44 labels | No fitted weights; leave-one-family-out required for promotion |
| Registry refactor changes existing verdicts | `diagnose_acceptance_test.go` is the regression gate for the nine current axes |

## Success criteria

1. Every measurement in `internal/audio/fingerprint` — including the
   currently orphaned family analyzers — is reachable as a registry
   descriptor through the generic API.
2. Whole-signal, segment-resolved, per-band and modulation-domain
   coverage exists for any instrument, not only kicks.
3. The battery's effective rank is measured and committed, and every
   |ρ| ≥ 0.9 cluster is either collapsed or justified in writing.
4. Every descriptor's nuisance sensitivity is measured and committed;
   fragile descriptors are demoted.
5. Every descriptor carries a measured discrimination result against
   both verdict label types; non-discriminating axes are demoted rather
   than driving tuning attention.
6. Stage attribution answers "which synthesis stage has the most
   leverage on this axis" for every MISMATCH axis.
7. The ten-instrument acceptance gate still passes.
