# Song-Level WAV Matching & Circuit Validation — Design

**Date:** 2026-06-21
**Status:** Draft for review
**Branch:** add-core-node-types

## 1. Goal

Validate and refine Beatmo's **template circuits** (the genre-masterpiece
recreations) and their **instruments** by comparing a rendered template against a
**real recording** of the same piece, with full **time-resolved** ("second by
second") understanding of the recording. Concretely, given a template id and a
reference song WAV, the framework:

1. Renders the template offline to a **master mix WAV + per-instrument stems**
   (template-faithful: per-instrument volume/pan/reverb-send/insert-FX).
2. Analyzes both the real recording and the rendered master with a shared,
   tested DSP/MIR library producing **musical** descriptors (tempo, onsets,
   chroma, key+mode, primary/secondary harmonics, chord/interval content,
   intonation, band balance, tonal/percussive split) **and** a per-frame
   **timeline** (sections + approximate instrument activity over time).
3. Compares them into a **CorrectnessReport** (is the circuit faithful — tempo /
   rhythm / key+harmony / mix balance) cross-checked against a second ground
   truth (the canonical `gen-showcase/specs.go` arrangement), plus **TuningHints**
   (band-attributed instrument nudges + auto-extracted solo passages fed to the
   existing per-instrument matcher).

**This extends, never duplicates,** the analysis-driven synth-matching framework
(`internal/wave`, `internal/audio/fingerprint`, `internal/audio/synthmatch`,
`cmd/synth-match`). Song-level metrics build on the same `wave` FFT/STFT core;
solo-passage tuning calls the existing `synthmatch.Match` + `PatchSeedFile`.

**Non-goals:** real-time analysis (offline dev tool); source separation /
exact polyphonic instrument transcription (we give *approximate, labeled*
instrument-presence estimates, not stems-from-mix); changing the C DSP; replacing
the ear test as the final acceptance gate.

## 2. Hard constraints (the cross-cutting contract)

Every module obeys these. They are testable invariants, not aspirations.

- **Generic.** No song-, instrument-, or template-specific logic in code. The 15
  shipped templates are *data*; the framework runs identically on any template id,
  any instrument set, and any reference WAV. Band edges, axis set, key profiles,
  and the instrument list are all data-driven.
- **Deterministic.** No `time.Now`, no unseeded RNG anywhere in library code. Any
  iterative step (e.g. NMF instrument-activity) seeds from `AnalysisConfig.Seed`.
  All map iteration in outputs is sorted. Identical inputs → **byte-identical**
  timeline, report, and score. Asserted by a determinism test.
- **Config-driven.** All behavior comes from three inputs and nothing else:
  `AnalysisConfig` (DSP + comparison knobs, documented defaults, `-config`
  override merged over defaults), `TimeManifest` (per-recording time structure,
  every field optional), and the pair `(templateID, refWavPath)`. No magic
  numbers in code — they live in `AnalysisConfig` with defaults.
- **Public-domain references only.** Committed reference WAVs are CC0 classical
  recordings (Bach / Mozart / Vivaldi). Any other template is validated against a
  **user-supplied** local WAV (nothing copyrighted is committed). Every test skips
  cleanly when `songrefs/` is empty.
- **No `git add`/`commit`** — standing repo rule; the matcher proposes, the user
  reviews `git diff` and ear-tests. Seed patches are uncommitted.

## 3. Architecture

```
internal/wave  (existing DSP core: FFT, STFT, windows, transforms, testgen)
      ▲                                   ▲
      │                                   │
internal/audio/fingerprint  ───────────── (unit tests use wave.Sine/…)
  • existing: single-note timbre (F0, partials, MFCC, ADSR, Distance)
  • NEW song-level: SongFingerprint (tempo, onset env, chroma, key+mode,
    harmonic profile, chord/interval, intonation, bands, H/P split)
    + SongTimeline (per-frame descriptors, sections)
    imports ONLY internal/wave
      ▲                         ▲
      │                         │
internal/audio/songrender       internal/audio/songmatch
  template JSON → master WAV     Compare(...) → CorrectnessReport + TuningHints
  + per-instrument stems         + SongDistance score
  (faithful: vol/pan/sends/FX,   instrument activity (templates from stems),
   offline; render_prod/stub     band attribution, solo-passage extraction →
   split like synthmatch)        synthmatch.Match. AnalysisConfig + TimeManifest.
  imports internal/audio                ▲
  + fingerprint                         │
                          cmd/song-match (CLI, //go:build !test && !js) + make
```

- `internal/audio/fingerprint` keeps importing **only** `internal/wave`. Song-level
  metrics are new files in the same package (one DSP home, no duplication).
- `internal/audio/songrender` imports the parent `internal/audio` (recipe render +
  processors) + `fingerprint`. Build-tag split (`render_prod.go` `!test` real C
  synth; `render_stub.go` `test` param-responsive Go) mirrors `synthmatch` so the
  compare/tuning tests run on the fast path.
- `internal/audio/songmatch` imports `audio` + `fingerprint` + `synthmatch` +
  `songrender`. Parent `audio` imports none of these (cycle-free, same invariant
  as the synth-matching framework).
- `cmd/song-match` is a thin `!test && !js` front-end.

## 4. Configuration & manifest (Section 6 of brainstorm)

### 4.1 `AnalysisConfig`

```go
type BandEdge struct { Name string; LoHz, HiHz float64 }

type AnalysisConfig struct {
    HopSec        float64      // timeline frame hop (default 0.5)
    FrameSec      float64      // analysis frame length (default 0.093 → 4096@44.1k)
    FFTSize       int          // default 16384 for whole-segment, 2048 per-frame
    Bands         []BandEdge   // default sub/low/mid/high/air; generic N bands
    TempoMinBPM   float64      // default 40
    TempoMaxBPM   float64      // default 240
    TempoOctaveFold bool       // ×2/÷2 equivalence (default true)
    OnsetSensitivity float64   // spectral-flux threshold (default 1.0)
    KeyProfileMajor [12]float64 // Krumhansl default, overridable
    KeyProfileMinor [12]float64
    NoveltyKernel int          // segmentation kernel size in frames (default 16)
    NoveltyThresh float64      // boundary threshold (default 0.5)
    AxisWeights   map[string]float64 // tempo/rhythm/key/mix → SongDistance weight
    AxisTolerances map[string]struct{ OK, Drift float64 } // verdict thresholds
    InstrTemplate string       // "stems" | "recipe" (default "stems")
    NMFIters      int          // default 50
    Seed          int64        // fixed RNG seed (default 1)
}
```

`DefaultAnalysisConfig()` returns the documented defaults. `LoadAnalysisConfig(path)`
unmarshals a partial JSON and **merges over defaults** (only set fields override),
mirroring `audio.MergeRecipeDefaults`. Every knob has a one-line doc comment.

### 4.2 `TimeManifest`

```go
type Span struct { StartSec, EndSec float64; Label string; Instruments []string }
type TuningPassage struct { StartSec, EndSec float64; Instrument string }
type TimeManifest struct {
    Ref         string          // reference WAV path
    Template    string          // template id
    Transpose   int             // semitones (manifest hint; else auto)
    BPM         float64         // tempo hint (0 → auto)
    CompareWindow *struct{ StartSec, EndSec float64 } // segment to compare (nil → auto)
    Sections    []Span          // ground-truth sections + expected instruments
    TuningPassages []TuningPassage // solo/exposed windows for synthmatch tuning
}
```

**Every field optional.** Present → authoritative for that span (overrides
auto-detection); absent → auto-detected. The report surfaces auto-detected-vs-
manifest divergence so the manifest itself can be validated. This is how each
unique interpretation is described and how any WAV tunes any circuit/instrument.

## 5. Offline renderer (`songrender`)

`Render(templateID string, cfg AnalysisConfig) (Rendered, error)` where
`Rendered{ Master wave.Wave; Stems map[string]wave.Wave; Arrangement Arrangement }`.

- Loads the template JSON via the existing `internal/templates` loader.
- Builds the note schedule: for each instrument row, the node graph + predictor
  determine which steps fire over `Bars` at the template BPM (reuse the engine's
  predictor; deterministic). Each firing → one note event `{instID, pitchSemis,
  startSample, volume, durationSec}`.
- Per instrument: render each note via `recipe.Render` (the `synthmatch`
  `resolveRender` path, pitch-aware), place at `startSample` into the instrument's
  stem buffer.
- **Template-faithful processing:** apply the instrument's volume, pan (stereo →
  we analyze the mono sum but keep pan energy), reverb send, and insert filters
  using the `internal/audio` processors offline; sum processed stems → master.
  Stems returned are post-FX. (`resetSendEffectsState` is called first — global
  send-reverb state must not leak between renders; see the genre-template memory.)
- Deterministic; works under `-tags test` via the stub renderer (param-responsive
  so `Compare`/tuning tests have non-trivial audio).

`master == Σ processed stems` within FX numerical tolerance — a renderer test.

## 6. Musical analysis (`fingerprint` song-level)

### 6.1 `SongFingerprint` (whole-segment, on the compare window)

| Field | Metric | Test fixture |
|---|---|---|
| `TempoBPM`, `TempoConfidence` | onset-autocorrelation / comb-filter tempo | click train @ known BPM → ±2% |
| `OnsetEnvelope []float64`, `OnsetTimes []float64` | spectral-flux onset detection | impulses at known steps → those times |
| `Chroma [12]float64` | STFT → pitch-class folding | C-major triad → C,E,G peaks |
| `Key int`, `Mode int`, `KeyConfidence` | Krumhansl–Schmuckler key profiles | C-major scale → C major; transposed → shifts |
| `PrimaryPC int`, `Harmonics []HarmonicPeak` | dominant pitch class + ranked overtone series | harmonic stack → primary + secondaries |
| `IntervalHistogram [12]float64`, `Consonance float64` | pitch-class interval distribution + roughness | triad vs cluster differ |
| `IntonationCents float64` | mean cents deviation from equal temperament | detuned sine → reports the cents |
| `Bands []float64` | energy per `cfg.Bands` band | band-limited noise → right band |
| `TonalPercussiveRatio float64` | harmonic vs percussive energy split | sine high, noise/clicks low |

`SongFingerprintOf(w wave.Wave, cfg AnalysisConfig) SongFingerprint`. Reuses
`wave.MagnitudeSpectrum` / STFT; no new FFT.

### 6.2 Transposition-invariant comparison

Key/chroma comparison finds the **best circular chroma shift** between two
fingerprints and reports the implied semitone transposition + whether mode and
relative interval structure match — never an absolute-pitch mismatch (the
templates are deliberately transposed to A-based modular tuning).

## 7. Time-resolved timeline (`fingerprint` song-level)

`SongTimelineOf(w wave.Wave, cfg AnalysisConfig) SongTimeline` — per-frame
(`cfg.HopSec`) over the whole file:

```go
type Frame struct {
    TimeSec float64
    RMS float64; Bands []float64; OnsetDensity float64
    RegisterCentroidHz float64; Chroma [12]float64
    Key, Mode int; HarmonicPercussive float64
}
type Section struct { StartSec, EndSec float64; Label string }
type SongTimeline struct { Frames []Frame; Sections []Section }
```

- **Exact** per-frame: RMS/loudness, band energies, onset density, register
  centroid, chroma→key/chord, harmonic/percussive ratio.
- **Sections:** self-similarity matrix over frame feature vectors → novelty curve
  (`cfg.NoveltyKernel`) → boundaries (`cfg.NoveltyThresh`), emitted as labeled
  spans. When a `TimeManifest.Sections` is supplied, manifest spans are
  authoritative and auto-detected boundaries are reported alongside for validation.

### 7.1 Instrument activity over time (approximate, labeled)

`InstrumentActivity(real SongTimeline, templates map[string]SpectralTemplate,
cfg) map[string][]float64` (lives in `songmatch`, needs stem templates):

- Build a spectral template per instrument from `songrender` stems (or recipe
  fingerprints when `cfg.InstrTemplate=="recipe"`).
- Per frame, non-negative template matching (seeded NMF, `cfg.Seed`/`cfg.NMFIters`)
  → per-instrument activation + confidence.
- **Honest caveat** emitted in the report: reliable on solo/sparse passages
  (Bach organ, Mozart piano), indicative only on dense ensembles. `TimeManifest`
  instrument lists override/anchor the estimate where provided.

Narrative renderer turns frames+sections+activity into:
```
00:00–00:04  intro        organ (solo, hi-conf) · low register · sparse · D-minor
00:04–00:13  theme A      organ · mid register · dense onsets · Dm→A
```

## 8. Comparison, report & tuning (`songmatch`)

`Compare(real, master SongFingerprint, realTL SongTimeline, stems, showcase,
manifest, cfg) Result` where:

```go
type AxisVerdict struct { Axis string; State string /*OK|Drift|Mismatch*/;
    Magnitude float64; Detail string }
type TuningHint struct { Instrument string; Kind string /*band|seed*/;
    Suggestion string; SeedDeltas map[string]float64 }
type Result struct {
    Report []AxisVerdict          // tempo, rhythm, key, mix (fixed order)
    SpecDrift []string            // template JSON vs gen-showcase/specs.go
    Hints []TuningHint            // sorted by instrument id
    Distance SongDistance         // weighted, normalized, identity==0
}
```

- **Tempo axis** — BPM both, ×2/÷2 fold (`cfg.TempoOctaveFold`); reports ratio.
- **Rhythm axis** — onset envelope folded to one bar (tempo-normalized),
  cross-correlated; lag/shape mismatch = groove error.
- **Key/harmony axis** — transposition-invariant chroma match + mode + harmonic
  profile delta + intonation; reports implied transposition.
- **Mix-balance axis** — per-band energy deltas (boomy/thin/bright).
- **Two ground truths:** `SpecDrift` compares the loaded template JSON to its
  canonical `gen-showcase` showcase spec (did the committed JSON drift?), separate
  from the real-recording faithfulness axes.
- **`SongDistance`** — weighted (`cfg.AxisWeights`), normalized per term,
  `Distance(fp,fp)==0`, for regression/optimization. Verdict thresholds from
  `cfg.AxisTolerances`.

**TuningHints** (never auto-committed):
- **Band attribution** — for each off-balance band, dominant instrument(s) from
  stems → EQ/brightness nudge.
- **Solo-passage tuning** — `manifest.TuningPassages` (or auto-detected
  low-polyphony frames) extracted from the real WAV → existing `synthmatch.Match`
  → `SeedDeltas`; applied via the existing `PatchSeedFile` only when not `-dry-run`.

## 9. CLI + make targets

`cmd/song-match` (`//go:build !test && !js`), subcommands:

```
song-match timeline -file <wav> [-config c.json] [-hop 0.5] [-out tl.csv]
song-match render   -template <id> -out <dir> [-config c.json]
song-match compare  -template <id> -ref <wav> [-manifest m.json] [-config c.json]
                    [-bars N] [-tune] [-report out.md] [-dry-run] [-seedfile path]
```

`timeline` is the standalone second-by-second dump (CSV/JSON + narrative) of any
WAV. `compare` renders, analyzes both, prints the CorrectnessReport + timeline
narrative + tuning hints; `-tune` runs solo-passage `synthmatch` and (unless
`-dry-run`) patches seeds.

Make targets:
- `song-compare TEMPLATE= REF= [MANIFEST=] [CONFIG=] [TUNE=1] [DRY_RUN=1]`
- `song-timeline FILE= [HOP=0.5] [CONFIG=]`
- `song-render TEMPLATE= OUT=`
- `fetch-song-refs` — CC0 classical WAVs (§10).

## 10. Reference WAVs (CC0 classical, PD-clean)

- `src/go/internal/audio/testdata/songrefs/<template>.wav` + `MANIFEST.md`
  (instrument/source URL/license/sha256 per file).
- `scripts/fetch_song_refs.sh` fetches **CC0 classical PCM WAV** for Bach Toccata,
  Mozart K545, Vivaldi Spring only (Musopen / IMSLP CC0 where a WAV is offered).
  References must be **PCM WAV** (stdlib-decodable via the existing
  `sample_pcm_teststub.go`). If a CC0 source offers only lossy audio, the manifest
  records provenance and the user drops a local WAV; the framework is
  source-agnostic.
- Skip-if-absent everywhere: the suite is green before `fetch-song-refs` runs.

## 11. Testing strategy (thorough; shared `wave` fixtures)

Fast path: `go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/…
./internal/audio/songrender/… ./internal/audio/songmatch/…`.

- **Song-fingerprint metrics** (`fingerprint`): click-train→BPM±2%; impulses→onset
  times; C-major triad→key C major; transposed triad→same mode, reported shift;
  harmonic stack→primary PC + secondaries; band-limited noise→right band;
  detuned sine→intonation cents; sine vs clicks→tonal/percussive split.
- **Timeline** (`fingerprint`): concatenated segments of differing
  loudness/spectrum → section boundaries at the joins; deterministic frame count.
- **Instrument activity** (`songmatch`): mix two known synthetic stems → activation
  curves track each; manifest instrument list anchors the estimate.
- **Renderer** (`songrender`): 1-bar 2-instrument template → `master == Σ stems`
  within FX tolerance; deterministic across two runs; runs under stub renderer.
- **Compare** (`songmatch`): identical inputs → all-OK report, `SongDistance≈0`;
  transposed template → key axis reports shift (not Mismatch); tempo-doubled → ×2
  equivalence; per-axis verdict thresholds honor `cfg.AxisTolerances`.
- **Spec drift**: template JSON matching its `gen-showcase` spec → no drift;
  perturbed → flagged (reuses `templates_test` patterns).
- **Config**: `LoadAnalysisConfig` merges partial JSON over defaults; unset fields
  keep defaults. **Manifest**: optional fields absent → auto-detection path;
  present → authoritative.
- **Determinism**: two full `compare` runs on the same inputs → byte-identical
  report + timeline + score.
- **Refs smoke**: every committed CC0 WAV decodes + plausible tempo/key; skips when
  `songrefs/` empty.
- **CLI build smoke**: `song-match` compiles under `!test !js`.

## 12. Risks & open questions

- **Polyphonic instrument ID is approximate.** Without source separation, activity
  estimates are reliable only on solo/sparse passages; the report labels
  confidence and the manifest can anchor it. Stated, not hidden.
- **Transposition & tempo equivalence** could mask real errors if over-aggressive;
  mitigated by reporting the implied shift/ratio so the human sees it.
- **CC0 WAV sourcing**: classical CC0 recordings are often lossy-only; the
  framework stays source-agnostic and skip-if-absent, so coverage degrades
  gracefully to "user supplies a local WAV."
- **Renderer fidelity vs the live engine**: offline processing reuses the `audio`
  processors but is not a bit-exact capture of the real-time 3-phase mixer; good
  enough for mix-balance trends, not sample-exact mastering.
- **Metric ≠ ears**: every hint is a *proposal* for the ear test (the real gate).

## 13. Decision log

- Extend the existing `wave`/`fingerprint`/`synthmatch` framework; reuse the one
  DSP core; no duplicated FFT/STFT/onset code (user).
- Validate against PD references: commit CC0 classical (Bach/Mozart/Vivaldi),
  support arbitrary user `-ref` WAVs for the rest; commit nothing copyrighted (user).
- Correctness axes: tempo, rhythm/onset, key/harmony, spectral/mix balance, plus
  additional musical metrics — key+mode, primary/secondary harmonics,
  chord/interval, intonation, tonal/percussive (user).
- Tuning loop composes all three: solo-passage `synthmatch`, mix-band attribution,
  and an always-on human-readable report (user).
- Render the template-faithful master (vol/pan/sends/insert-FX) offline (user).
- Add second-by-second timeline + approximate instrument activity over time, with
  honest confidence labeling (user).
- Everything generic, deterministic, and config-driven via `AnalysisConfig` +
  `TimeManifest` so any WAV + manifest tunes any circuit/instrument (user).
- Packages under `audio/` as `internal/audio/songrender` + `internal/audio/songmatch`;
  song-level metrics in `internal/audio/fingerprint`; cycle-free (parent `audio`
  imports none).
