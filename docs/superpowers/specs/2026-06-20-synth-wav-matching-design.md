# Analysis-Driven Synth Matching — Design

**Date:** 2026-06-20
**Status:** Draft for review
**Branch:** add-core-node-types

## 1. Goal

Make Beatmo's synthesized instruments sound more **organic** by tuning their
modular-synth parameters to **measurably approach a real instrument recording**.
Build the tooling that, given a real public-domain instrument WAV (the target)
and a synth instrument id, **searches the synth parameter space** to minimize a
perceptual/spectral distance, then **writes the tuned parameters back into the
instrument seed** for review and ear-tuning.

Concretely, ship:

1. A **tested DSP fingerprint library** (`internal/fingerprint`) built on the
   game's existing `internal/wave` package — no duplicated FFT/window/envelope
   code between game, matcher, and tests.
2. A **hardened + expanded** analysis metric set (robust F0, MFCC, log-spectral
   distance, spectral flatness/crest, tristimulus, log-attack-time, HNR, ADSR
   segmentation) with one canonical `Distance()`.
3. A **matcher/optimizer** (`internal/synthmatch`) that tunes a synth toward a
   reference WAV and **auto-patches the seed**.
4. A `cmd/synth-match` CLI + **make targets**.
5. A small committed set of **real, license-clean instrument WAVs** + a fetch
   script + manifest.

**Non-goals:** real-time matching (this is an offline dev tool); changing the C
DSP (`render_modular` stays; we only tune its parameters); replacing the ear
test as the final acceptance gate (the matcher gets us close, ears finish).

## 2. Existing infrastructure to reuse (no duplication)

This is a refactor-and-extend, not a greenfield. The "same utilities everywhere"
constraint is satisfied by building on what the game already uses:

| Capability | Existing API | Reused by |
|---|---|---|
| Wave container + slice/clone/peak | `internal/wave` `Wave` | fingerprint, matcher, tests |
| FFT | `internal/wave` `NewFFTObserver(size)` | fingerprint (replaces the private FFT in `cmd/synth-analyze`) |
| Windowing / normalize / resample / trim | `internal/wave` `WindowTransform`, `NormalizeTransform`, `ResampleTransform`, `TrimTransform` | fingerprint, matcher |
| Envelope / peak-RMS / zero-cross observers | `internal/wave` `NewEnvelopeObserver`, `NewPeakRMSObserver` | fingerprint |
| **Synthetic test signals** | `internal/wave` `Sine`, `Square`, `Noise`, `Impulse`, `DC`, `Silence` | **all fingerprint/metric unit tests** |
| WAV decode → mono PCM | `internal/audio` `DecodeWAVToPCM(path)` | matcher, CLI (wrapped into `wave.Wave`) |
| Sustained-note auto-segment | `cmd/synth-analyze` `autoSegment` (moves into fingerprint) | matcher (extract one note from multi-note refs) |
| Synth render from params | `internal/audio` `NewRecipe` + `Render` + `MergeRecipeDefaults` + `RecipeForInstrument` + `GetInstrumentParams` | matcher, CLI |
| Tunable param names/order | `internal/audio` `SynthParamSchema` + the `*Seed` `RecipeParams` maps | matcher search space |

`cmd/synth-analyze` currently carries its **own** `fft`, `peakNormalize`,
`rmsEnvelope`, Hann windowing, and is `//go:build !test` (so its DSP is **not**
unit-testable — `main_test.go` is `!js` only and would not even compile its
implementation under `-tags test`). This design **deletes that private DSP** and
re-points the CLI at `internal/wave` + `internal/fingerprint`, closing the
testability gap.

## 3. Architecture

```
internal/wave  (existing, shared DSP core: FFT, windows, transforms, testgen)
      ▲                         ▲                         ▲
      │                         │                         │
internal/audio/fingerprint  internal/analyzer       (unit tests use wave.Sine/…)
 (pure DSP metrics +         (game live meters —
  Distance; imports           already on wave;
  ONLY wave)                  may adopt shared
      ▲                        centroid helper)
      │
internal/audio/synthmatch  (optimizer + seed patcher; imports audio + fingerprint)
      ▲
      │
cmd/synth-match (CLI, //go:build !test !js)      cmd/synth-analyze (refactored CLI)
```

- `internal/audio/fingerprint` depends on **`internal/wave` only** (pure, fast,
  importable by the game UI/analyzer). Lives under `audio/` per the
  in-the-audio-package preference; no cycle because it imports neither the parent
  `audio` nor `analyzer`.
- `internal/audio/synthmatch` depends on the parent `internal/audio` (render +
  seed maps) + `fingerprint`. No cycle: the parent `audio` package never imports
  this subpackage — the seed patcher edits `instrument_seeds.go` as **text** and
  calls *into* `audio` to validate, never the reverse.
- Both CLIs are thin `//go:build !test && !js` front-ends.

## 4. The fingerprint library (`internal/fingerprint`)

A `Fingerprint` struct + builder computed from a `wave.Wave`. Carries the
current metrics (ported verbatim, then hardened) **plus** the new set. Every
field has a unit test asserting it on a synthetic signal of known properties.

### 4.1 Hardening of existing metrics

- **Robust F0** — replace the "lowest spectral peak above mean+1.5σ" heuristic
  (octave-error prone) with **autocorrelation / harmonic-product-spectrum**
  cross-checked against the strongest spectral peak; pick the candidate that
  best explains the partial series. Test: `wave.Sine(220)` → F0≈220 within 1%;
  a missing-fundamental harmonic stack → still 220 (no octave error).
- **Consistent segmentation** — one documented skip/length policy applied to
  **both** reference and synth (today the skip windows differ subtly). DC
  removal + Hann window via `wave` transforms before every FFT.
- **De-duplicated `Distance`** — see §5.

### 4.2 New metrics (full set)

| Metric | What it captures | Test fixture |
|---|---|---|
| **MFCC** (13 coeffs, mel filterbank) + delta | Perceptual timbre envelope | sine vs noise differ; same signal → identical |
| **Log-spectral distance / spectral convergence** | Global spectral shape match (better than per-partial L1) | identical→0; shifted spectrum→>0 |
| **Spectral flatness** (Wiener entropy) | Tonal vs noisy (noise≈1, sine≈0) | `Sine`→~0, `Noise`→~1 |
| **Spectral crest** | Peakiness of spectrum | sine high, noise low |
| **Tristimulus** (T1/T2/T3) | Balance: fundamental vs low vs high harmonics | synthesized harmonic stacks |
| **Log-attack-time** (MPEG-7) | Attack sharpness on a log scale | ramp envelope of known length |
| **Harmonic-to-noise ratio** (cepstral) | Breathiness/bowing noise | sine high HNR; sine+noise lower |
| **ADSR segmentation** | Attack / decay / sustain-level / release times | synthesized AD envelope |
| **Spectral skewness / kurtosis** | Spectral asymmetry & peakedness | analytic checks |

(Existing kept: partials[16], spectral centroid/rolloff, even/odd, inharmonicity,
noise ratio, RMS envelope, decay slope, STFT temporal block — centroid
trajectory/slope/range, harmonic flux, spectral variance, per-partial decay
slopes, vibrato rate/depth.)

### 4.3 Shared with the game

`internal/analyzer` already computes an FFT spectrum for the live UI on top of
`wave`. Where it computes a value the fingerprint also needs (e.g. spectral
centroid), the scalar helper is **extracted into `internal/fingerprint` and
called by both**, so the number the user sees on the live Spectrum tab and the
number the matcher optimizes are computed by the same code. (Scope-limited: only
helpers with a genuine shared call site; no gratuitous analyzer rewrite.)

## 5. Distance function

One exported `Distance(ref, cand Fingerprint) Score` is the **single source of
truth** used by `cmd/synth-analyze diff`, the matcher loss, and tests. `Score`
breaks down into named, individually-weighted, **normalized** terms (each term
scaled to ~[0,1] for a typical mismatch) so weights are interpretable:

```
Score{ Spectral, MFCC, Partials, Temporal, Envelope, Noise, Total }
```

Fixes the current double-counting (partials L1 **and** centroid/rolloff, which
are derived from the same partials). Weights live in one documented table;
`Distance(fp, fp) == 0` and the metric is symmetric — both are tests.

## 6. Matcher / optimizer (`internal/synthmatch`)

### 6.1 Search space — `TunableSpec`

Per instrument family, a curated list of tunable parameters:

```go
type Param struct { Key string; Min, Max, Step float64; Discrete []float64; Int bool }
type TunableSpec struct { Family string; Params []Param }
```

Families: **subtractive-reed** (sax/oboe/trumpet: filter cutoff/Q, filtenv amt/decay,
formant gen freqs/gains, attack/decay/sustain, lfo rate/depth), **additive-organ**
(drawbar gains, lfo rate/depth, attack/release), **ks-pluck** (clavinet/guitar:
ks_sustain/pluck, filter cutoff/resonance, decay). Discrete params (enable flags,
filter types, gen sources) use the `Discrete` set. Bounds are derived from
`SynthParam*` ranges where available, hand-clamped otherwise. Each tunable key is
validated against the live recipe so a typo can't silently no-op (test).

### 6.2 Optimizer

Deterministic **pattern search (Hooke–Jeeves coordinate descent)** with
per-parameter step schedules that shrink on stall, plus a small number of
**seeded** random restarts (fixed `math/rand` seed → byte-reproducible). Each
evaluation:

1. Render the synth with candidate params at the **reference's detected F0**
   (pitch-aligned), same duration as the analysis window.
2. Fingerprint it (reusing the ref's segmentation policy).
3. `loss = Distance(refFP, candFP).Total`.

Bounded by a `Budget` (max evaluations, default ~400). Returns best params, the
loss trajectory, and the before/after fingerprints. **Convergence test:** start
from a seed, perturb N known params, confirm the optimizer drives loss below a
threshold and recovers the perturbed params within tolerance — deterministic
with a fixed seed.

### 6.3 Seed auto-patcher

`seedpatch.go`: given (`seedVarName`, tuned `map[string]float64`), rewrite the
specific `var <name>Seed = RecipeParams{ … }` block in
`internal/audio/instrument_seeds.go`:

- Update values for keys present; append new keys before the closing brace.
- **Preserve** comments, key order, and untouched keys.
- `gofmt` the result; write in place (uncommitted — user reviews `git diff` and
  ear-tests; never auto-committed, per project rule).

Implemented as a structured edit over the file text scoped to the one `var`
block, with a **golden round-trip test** (patch a fixture, assert exact output;
patch-then-reparse equals the intended map).

## 7. CLI + make targets

`cmd/synth-match` (`//go:build !test && !js`):

```
synth-match -inst <id> -ref <wav> [-budget N] [-restarts K] [-dry-run] [-report out.md]
```

Prints: ref fingerprint, baseline synth fingerprint + distance, the optimized
distance, a per-parameter before→after table, and (unless `-dry-run`) patches the
seed. Make targets (Makefile):

- `make synth-match INST=sax REF=src/go/internal/audio/testdata/refs/sax_a3.wav [BUDGET=400] [DRY_RUN=1]`
- `make synth-fingerprint FILE=<wav>` — ad-hoc analysis (wraps `synth-analyze ref`).
- `make fetch-synth-refs` — runs the fetch script (§8).

## 8. Reference WAVs (real, license-clean)

- **Primary source: University of Iowa Musical Instrument Samples (MIS)** —
  real anechoic recordings, mono 16-bit/44.1 kHz AIFF, published "free … for any
  projects, without restrictions." Covers violin, cello, sax, trumpet, horn,
  flute, oboe, guitar, piano (the orchestral + sax seeds).
- **Organ / clavinet**: not in MIS — sourced from another documented PD/CC0
  source during implementation (Wikimedia Commons PD / Freesound CC0). The
  framework is **source-agnostic**: any WAV works.
- **Conversion**: AIFF → mono 16-bit WAV via a **pure-python-stdlib** script
  (`aifc` read → `wave` write); no ffmpeg/sox dependency (neither is installed).
  Multi-note source files are handled by `autoSegment` (one sustained note).
- **Layout**: `src/go/internal/audio/testdata/refs/<inst>_<note>.wav` +
  `refs/MANIFEST.md` (one row per file: instrument, note, source URL, license,
  sha256). `scripts/fetch_synth_refs.sh` downloads + converts + writes the
  manifest so the set is reproducible.

**Commit policy:** the WAVs are small (≤~1 s mono) and license-clean, so they are
committed. If any source's redistribution turns out ambiguous at implementation,
that file is left out of git and `fetch-synth-refs` regenerates it locally; the
manifest always records provenance.

## 9. Testing strategy (thorough; shared fixtures)

The DSP and the game share `internal/wave`; the tests share its **generators**:

- **Fingerprint metric tests** (`internal/fingerprint`): each metric asserted on
  `wave.Sine` / `wave.Noise` / `wave.Square` / `wave.Impulse` with analytically
  known answers (e.g. `Sine(220)`→F0≈220, flatness≈0; `Noise`→flatness≈1,
  low HNR; ramp→known attack/LAT). Identity & symmetry tests for `Distance`.
- **Optimizer test** (`internal/synthmatch`): perturb-and-recover convergence,
  deterministic under a fixed seed; `TunableSpec` keys validated against live
  recipes; budget is respected.
- **Seed-patcher test**: golden round-trip; comment/order preservation; gofmt
  stability; new-key insertion.
- **Refs smoke test**: every WAV in `refs/` decodes via `DecodeWAVToPCM` and
  fingerprints within sane ranges — **skips cleanly if the dir is empty**, so the
  suite is green before `fetch-synth-refs` is ever run (no binary dependency in
  CI).
- **CLI build smoke**: `synth-match`/`synth-analyze` compile under `!test !js`.
- All fast-path: `go test -tags test -modfile=go.test.mod ./internal/fingerprint/… ./internal/synthmatch/…`.

## 10. Risks & open questions

- **Local optima**: pattern search can stall; mitigated by seeded restarts +
  step schedules. If a family matches poorly, the report flags the residual.
- **Metric ≠ ears**: a low distance is necessary, not sufficient. The patched
  seed is always a *proposal* for the ear test (the real gate).
- **Organ/clav sourcing**: no MIS coverage; depends on finding a clean PD/CC0
  recording. If none is found, those two are matched against a user-supplied WAV
  and only the manifest/framework ship.
- **Seed-patch fragility**: text rewrite of a Go literal. Mitigated by scoping to
  one `var` block + golden test; if a seed is hand-formatted oddly the patcher
  fails loudly rather than corrupting (test).
- **Pitch coverage**: matching at one note tunes timbre at that pitch; key-
  tracking params (filter follow) may need a second reference pitch later. Out of
  scope for v1.

## 11. Decision log

- Reuse `internal/wave` as the one DSP core; delete `cmd/synth-analyze`'s private
  FFT/window/envelope (user: shared utilities, no duplication).
- Fetch real recordings myself; primary = Iowa MIS (real, unrestricted) (user).
- Full metric set, not a subset (user).
- Auto-patch the seed in place, uncommitted for review (user).
- Single-instrument-vs-one-WAV make target; batch deferred (user).
- Tests use synthetic `wave` fixtures and skip-if-absent for binaries, so the
  suite never depends on files that can't be reproduced in CI.
- Packages live under `audio/` as `internal/audio/fingerprint` +
  `internal/audio/synthmatch` (user preference), verified cycle-free: parent
  `audio` never imports either subpackage; the seed patcher edits source text and
  calls into `audio`, not vice-versa.
- Seed auto-patch via structured text-rewrite of the `var …Seed` block (user),
  golden-tested for validation and scalable across all seeds.
