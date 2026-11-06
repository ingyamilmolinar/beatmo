# WAV clone — analysis → resynthesis, vertical slice 1: struck / modal

Date: 2026-10-12
Status: design approved in conversation 2026-10-12; revised 2026-10-12 after
spec review (render-parity semantics, normalised storage instead of "fitted"
state, identifiable decay law, pitch of the fit, slice-1 scope, no gonum,
output dir, reuse of existing kernels). Implementation plans:
`docs/superpowers/plans/2026-10-12-wav-clone-1-render-parity-and-analysis.md`
(plan 1 of 3: render parity + the analysis half); plan 2 = tables + engine
modules (`2026-10-13-wav-clone-2-modal-engine-modules.md`); plan 3 = invert /
loss / refine / CLI adopt / Synth tab (`2026-10-13-wav-clone-3-invert-refine-adopt.md`).
**All three implemented 2026-10-13** — deviations are noted in place
("Implemented:").
Builds on: `2026-10-04-instrument-config-documents-design.md` (every instrument
is ONE document → `instrument.Flatten` → `render_modular_p`), the reference
corpus tooling (`cmd/refs`: adopt / compare / diagnose / session / validate),
and the fingerprint descriptor registry.
Literature survey (local, not committed): session scratchpad
`dsp_literature_survey.md`; the load-bearing references are listed at the end.

## Problem

Beatmo can *measure* a recording (≈82 registry descriptors, `AnalyzeModes`, YIN
`DetectF0Detailed`, `UsableAsPitch`) and *compare* a synth against it
(`refs compare`, `synthdiag.Ablate`, the ear-verdict loop), but it cannot turn a
recording into an instrument. Every shipped voice is hand-tuned:
`synthmatch` runs a single-threaded Hooke-Jeeves over 12 hand-curated knobs
against `fingerprint.Distance`, a hand-weighted metric that was never validated
against the ear verdicts — and two of the descriptors it leans on (ring T60,
centroid spread) are known to be ANTI-correlated with them.

The engine also cannot express what a recording of a struck instrument
measures: the additive path has 12 instant-attack partials, the cymbal modal
plate draws a random mode layout, and the body tables are hardcoded. So even a
perfect analysis has nowhere to go. The ear verdicts show the cost: tubular
bells (`n`, "resonant wavy metallic ring … scrape this"), kalimba (`n`, "needs
wooden marimba-esque feel"), glockenspiel and cowbell (`p`), tom and conga
(`1`).

There is also a correctness bug under the comparison tools:
`audio.RenderInstrumentWithFields` skips the voice's document FX, gate tail and
normalisation that `renderDocIntoCache` applies, so `refs compare` and
`refs diagnose` (via `synthmatch.RenderWithParams`) analyse a different sound
than the one that plays for the 19 shipped documents carrying FX.

## Goal

An offline developer tool that takes a WAV of a struck / modal instrument and
produces a **candidate instrument document** that sounds like it, which the
developer auditions by ear in the existing A/B flow and, on a `y`, ships in the
pack. The document contains only **measured model numbers that drive synth
modules** — never audio.

This is vertical slice 1 of a larger programme (strings, winds, drums get their
own specs). It must build the parts later slices reuse: the render-parity fix,
the clone harness (prepare → decompose → estimate → invert → refine → report),
the table-valued module mechanism, the validated loss, and the optimizer.

## Decisions taken with the user

| Question | Decision |
|---|---|
| Purpose | Dev tooling → shipped instrument documents. Reference audio stays local (`$BEATMO_WAV_REFS`), never committed or shipped. |
| What may ship | "Only existing and new synth functionality, never copy literal audio data from the original." Mode lists, noise-band levels/envelopes, strike params, decay laws. **No PCM, no extracted wavetables, no commuted excitation tables.** |
| First family | Struck / modal. |
| Success criterion | The user's `y` verdict in A/B (`make ab` / `refs session`). Losses rank candidates; they never accept one. |
| Approach | A: analysis-first sub-band ESPRIT → new table-valued `modal` module → seeded parallel CMA-ES refine. |
| Process | This spec = clone harness + modal family. Later families = later specs. |

## Non-goals

- Other families (waveguide strings, winds, drum heads, noise instruments).
  Drum heads glide in pitch as they decay (tension modulation — the physical
  kick models it, `modular_stages.c` variant 8); a fixed-frequency mode model
  cannot express that, so tom and conga are NOT gated in slice 1 (§6b).
- Multi-note fitting / key maps of mode tables (one reference note per instrument).
- Any learned model (DDSP, neural codecs, NoiseBandNet training). Analytic only.
- Shipping or committing any audio, wavetable or excitation table.
- Moving `synthmatch` onto the new optimizer (it only moves onto `RenderAsShipped`).
- Runtime (in-app) cloning; the tool is developer-side only.

## Architecture

New package `internal/audio/clone` (pure stages, each testable alone) and
command `cmd/clone`.

```
WAV ──Prepare──▶ Prepared ──Decompose/Estimate──▶ Analysis ──Invert──▶ Doc₀
                                                             │
                     Report ◀── Refine (CMA-ES over Doc₀ corrections, RenderAsShipped + Loss)
                       │
          clones/<id>/ ──▶ make ab-session CLONE=1 ──▶ verdict ──▶ clone adopt (only on y)
```

| Unit | Input → output | Depends on |
|---|---|---|
| `clone.Prepare` | WAV path or `[]float32`+rate → `Prepared` (mono at the file's own rate, onset, floor, peak, loudness) | `wavgen.DecodeWAV`, fingerprint |
| `clone.EstimateModes` | `Prepared` → `[]Mode` + diagnostics | `clone/linalg` (in-package) |
| `clone.EstimateStrike` | `Prepared`, modes → `Strike` | — |
| `clone.MeasureResidual` | `Prepared`, modes → `[]NoiseBand` | — |
| `clone.FitDecayLaw` | modes → `DecayLaw` | — |
| `clone.Analyze` | composes the four above → `Analysis` (JSON-serialisable) | — |
| `clone.Invert` | `Analysis`, options → `instrument.Doc` | `instrument`, audio registry |
| `clone.Loss` | reference + candidate render → `LossReport` (per-term) | fingerprint registry |
| `clone.Refine` | `Doc₀`, `Prepared`, options → `Doc`, trace | `RenderAsShipped`, `async.Pool` |
| `clone.Report` | everything → `report.md` + `report.json` | — |
| `cmd/clone` | CLI, file layout, `adopt` | `wavref`, `cmd/refs` session plumbing |

Reused, not rebuilt: fingerprint registry + `Compare`, `DetectF0Detailed` /
`UsableAsPitch`, `AnalyzeModes` (cross-check only), `wavref` (resolution,
catalogue `measured.semis_from_a3`), `refs session` / `make ab` (listening,
verdict rows), `instrument.SetField` / `Validate`, `wavgen` (test audio),
`async.Pool`.

### Linear algebra — in-package, no new dependency

ESPRIT needs a complex Hermitian eigendecomposition (the signal subspace) and
the eigenvalues of a small general complex matrix (the poles). gonum has
neither for complex matrices (its `CDense` carries no decompositions), so the
review dropped it. `internal/audio/clone/linalg` holds ~300 lines: cyclic
Jacobi for complex Hermitian matrices, Hessenberg reduction + shifted QR for
complex eigenvalues, and complex least squares via the normal equations
(Cholesky). Matrices are ≤ 128 × 128; known-answer tests pin each routine.
Nothing outside `internal/audio/clone` and `cmd/clone` imports the package;
`TestCloneIsDevOnly` (in `internal/audio/clone`) asserts no package reachable from
`cmd/beatmo.go` or the WASM main imports `internal/audio/clone`.

## 1. Render parity — `audio.RenderAsShipped`

Two functions, split along the line the playback code already draws:

- `renderDocVoice(d instrument.Doc, bpm, sampleRate int, pitch float64, pitched bool) []float32`
  — the VOICE BUFFER, factored out of `renderDocIntoCache` (which then calls
  it, so there is one implementation): Flatten (pitch written when pitched) →
  `render_modular_p` → document FX (`applyDefaultFX`) → gate tail →
  peak-normalise × `d.Render.Amplitude`. The amplitude comes from the
  DOCUMENT (`normalizeAndScaleAmp(buf, amp)`; `normalizeAndScale(buf, id)`
  becomes a thin wrapper), because a candidate document is never registered
  and an id lookup would read some other instrument's amplitude. Length =
  `render.beats × 60 / bpm` — the desktop voice length.
- `RenderAsShipped(d instrument.Doc, bpm, sampleRate int, pitch float64) []float32`
  — what a trigger of `d` at (pitch, duration 1) puts into the mix, the
  document twin of `RenderAsPlayed`: melodic documents (`render.melodic`)
  render AT pitch and play at rate 1; percussion renders at pitch 0 and is
  resampled by 2^(pitch/12) through the same `resampleVoice` playback uses
  (`playedVoice`, `engine_play.go`). No cache, no registration, no globals
  written — safe to call from several goroutines.
- Guard (`!test`, real C): for every shipped instrument at pitch 0 and one
  non-zero pitch, `RenderAsShipped(doc)` is byte-identical to
  `RenderAsPlayed(id)` — an independent path, so the guard is not tautological.
- Callers: `refs compare` / `refs diagnose` and the clone harness move onto it
  (new `synthmatch.RenderShipped`). `RenderInstrumentWithFields` STAYS the raw
  C render: `cmd/export_audio` (the cross-platform reference compares raw C on
  both sides), `make measure-loudness` (it computes `render.amplitude`, so
  measuring after it would be circular), `gen-fundamentals` and the organic
  calibration keep using it.
- **Pre-existing platform divergence (reported, not fixed here):** the browser
  sizes a voice by `render.seconds` (`audio.js` `info.seconds`), desktop by
  `render.beats × 60/bpm`. 7 of the 9 slice-1 instruments ship
  `beats == seconds`, so at 120 BPM desktop plays half the ring the browser
  plays (tubular bells: 1.5 s vs 3 s). Clone documents write both
  consistently at 120 BPM (§5a); unifying the two platforms is its own change.

The C engine is re-entrant after the lazily built wavetables exist (they
initialise through `beatmo_once`), so `Refine` performs one warm-up render
before fanning out.

## 2. Analysis

### 2a. Prepare

- Fold to mono at the file's OWN sample rate — no resampling. Modes are
  measured in Hz and seconds, so the rate never enters the model; the loss
  renders the candidate at the reference's rate instead (the repo's only
  resampler, `wave.ResampleTransform`, is linear interpolation and would
  colour the high modes).
- Onset: spectral-flux peak (found without any level threshold), refined to
  the first sample above floor + 20 dB (capped at −20 dB re the peak).
  Sample-accurate; everything downstream is onset-relative.
- Noise floor: the median 10 ms-frame RMS of the pre-roll before the onset
  when there are ≥ 20 ms of it, else the median of the quietest 10% of
  frames (implementation found the quietest-10% rule alone reads the ring
  itself as the floor in a short clip that rings to its end). Peak, RMS of the
  first 400 ms.
- Nominal pitch: wavref `measured.semis_from_a3` when `UsableAsPitch` passes;
  else `-f0`; else **unpitched** (modes kept in absolute Hz). Tubular bells use
  a pinned virtual (strike-note) pitch.
- Glide diagnostic: the strongest mode's frequency in the first vs the last
  third of the decay window; a drift > 10 cents is reported ("pitch glide —
  a membrane? out of slice-1 scope"), never silently fitted.

### 2b. Modes — sub-band ESPRIT poles, one joint amplitude fit

(Revised during plan 1 against the local VCSL corpus — every rule below
replaced one that shipped broken first; see `AGENTS.synth.instruments.md`
"Cloning a struck instrument".)

- **Poles per band.** 24 log-spaced bands, 30 Hz – min(20 kHz, 0.45·sr).
  Each band is heterodyned to baseband (phasor recursion, re-anchored every
  1024 samples), Blackman low-passed (flat to ±0.5w, stop from ±1.5w) and
  decimated to ≥ 2w. The analysis window starts 5 ms after the 2 ms envelope
  first comes within 3 dB of its early maximum, and each band's data is cut
  where it falls to 6 dB above that band's noise (the lower of its share of
  the recording floor and its own quietest moment). Hankel rows
  L = min(N/3, 128), ≤ 384 strided snapshots, L × L covariance → Hermitian
  eigendecomposition (Householder tridiagonalisation + implicit QL).
- **Order.** Candidates = eigenvalues ≥ 6 dB above the noise median (≤ 24
  per band) → poles = eig(Φ) → **orthogonal matching pursuit** keeps a pole
  only while it explains ≥ 20σ² of the band (noise explains ~σ² by chance).
  ESTER at a fixed bar (‖W↑ − W↓Φ‖_F ≤ 0.3) discarded real tubular-bell
  partials (E(1) is already 0.3–0.6 in long high bands full of strike noise);
  the untrimmed order handed the amplitude fit clusters of cancelling poles.
  A budget of 40 candidates overfit (100+ modes, erratic glide readings) for
  little recall.
- **Pole hygiene**: frequency inside the band's CORE (overlap never
  double-counts), α > 0, SNR ≥ 0 dB, and α·(window start − onset) ≤ 40 dB — a
  mode that had already fallen 40 dB before the window opened is strike
  transient, not a mode.
- **Amplitudes: one joint fit.** Per-band amplitudes are only valid inside the
  band window, which in low bands starts a long filter half-length after the
  onset; extrapolated back, they blew up (marimba's phantom 32 Hz mode: +48 dB;
  cowbell's and the tom's pole clusters stop cancelling outside the window).
  So all modes' amplitudes and phases come from ONE least-squares fit of the
  full-band recording from the onset — every sample of the first 0.2 s (a
  sparse sample there cannot pin high modes between rows), then 4096 jittered
  seeded rows to the window end — solved as the TRUNCATED minimum-norm
  solution (normal-matrix eigen-directions below 1e-8 of the largest dropped:
  exactly the cancelling combinations). Up to 128 modes (the table capacity).
- Modes below −60 dB of the strongest are dropped. Doublets (close pairs with
  distinct α) are kept — they are the "wavy" beating the tubular-bell verdict
  asked for.
- Cross-check against `AnalyzeModes`: a strong STFT partial with no mode within
  max(3 cents, one 4096-point STFT bin) is listed in the report as "missed".
- Measured (2026-10-12, 8-core laptop): residual marimba 0 %, vibraphone 0 %,
  glockenspiel 5 %, xylophone 5 %, kalimba 7 %, tubular bells 16 %, cowbell
  23 %; tom / conga 6 / 21 % via mode clusters, with a reported 15–20 cent
  glide. ~1.5–2.5 s per instrument, `EstimateModes` dominating — above the
  ≪ 1 s first guess; the 24 bands are independent and go on a pool in plan 3.

### 2c. Strike

Fit a raised-cosine contact force of duration T_c to the onset-region spectral
roll-off of the mode amplitudes (spectral null at 2/T_c), plus a hardness
slope (dB/oct) for the residual tilt. Output: `Strike{Tc, Hardness, ClickLevel}`.

### 2d. Residual

Reference minus the modal resynthesis, measured in ~24 third-octave bands: per
band a level, an attack time, and a two-rate decay (fast rate, fast mix, tail
rate). This is a measurement of the residual's envelope, not the residual.

### 2e. Decay law

Fit α(f) = a·(f / 1 kHz)^p + b·(f / 10 kHz)^6 across the modes (weighted by
energy; a ≥ 0, b ≥ 0). Both reference frequencies are FIXED: the review found
that a free corner f_c is not identifiable — b and f_c only ever appear as
b / f_c⁶ — so the law has exactly three parameters (a, p, b). Used for the
decay macros and for transposition (§3b, §5).

### 2f. Output

`clone.Analysis` (modes, strike, noise bands, decay law, pitch info, window,
diagnostics, timings) with a stable JSON dump. Cost measured and reported
(§2b: ~1.5–2.5 s per instrument).

## 3. Engine modules

**Implemented 2026-10-13 (plan 2,
`docs/superpowers/plans/2026-10-13-wav-clone-2-modal-engine-modules.md`) with
three changes:** (1) `modal`, `strike` and `noiseband` are STAGES, not gen
sources 11/12 — layer values are per gen slot (`gen<k>_*`) while a table is
document-wide, and two capacity-1 layers would both need slot 1; as stages they
are global fields with an enable switch, rendered by `src/c/modal_bank.c` right
after the gen bank. (2) The phasor kernel is NOT shared with
`gen_cymbal_modal` (six lines; the cymbal accumulates the sine part in float
with its own stop rule — sharing would touch its goldens for no reuse). (3)
"Modes: keep N" keeps the FIRST N rows; rows are in priority order (the writer
sorts by energy) — Validate cannot sort-check (energy depends on the law and
the pitch). Measured: 128 modes × 3 s render in 8.6 ms native; the ABI grew
439 → 1321 doubles and a melodic trigger's cache key (`BlockHashWith` over the
block) costs ~11 µs (was ~4 µs) — left as is. The `noiseband` level offset is
`noiseband_level` (dB); there is no modal gain knob (rows are in reference
units, the voice is peak-normalised).

### 3a. Tables in documents

A module may carry an optional `table`:

```json
"table": { "cols": ["freq","decay","amp","phase","attack"],
           "rows": [[1.0, 1.9, 0.82, 0.0, 0.001], ...] }
```

- Declared per module type by a `TableDecl{Block, Columns, Capacity}` in the
  registry.
- `Flatten` writes row *i*, column *c* into ABI field `<block>_<col>` slot *i*,
  and the row count into `<block>_count`; slots ≥ count are zero.
- `Validate`: known columns in declared order, row count ≤ capacity, every
  value finite and within the column's bounds.
- `Clone`, `Diff`, `ContentHash` / `BlockHash` cover the table; a new
  `instrument.SetTable(doc, moduleRef, table)` is the only writer.
- Schema stays `0.0.1`; the field is additive and absent on every existing
  document.
- **Data-rule guard:** `TestTablesAreModelParametersOnly` — the sum of all
  table capacities in the registry is bounded (≤ 1,024 values per document)
  and every table column has declared physical bounds, so a document cannot
  carry a waveform even by accident.

### 3b. `modal` layer

- New gen source (next free id; 11 at the time of writing — 1–10 are taken),
  table capacity **128 rows**.
- **Slot ↔ table mapping.** Layers are gen slots and their scalar fields are
  per slot (`gen<k>_*`), but the table flattens to ONE block-wide array
  (`modal_<col>[128]` + `modal_count`). So at most one slot may carry
  `source = modal`: `Validate` rejects a second one, and the table lives on
  that layer's module (`Layers[k].Table`). The slot's own `gen<k>_gain` /
  `freq_mode` apply to the whole bank. Plan 2 pins this in the registry
  (`TableDecl.MaxPerDoc = 1`).
- Columns: `freq` (ratio to the nominal f₀, or Hz — the slot's `freq_mode`),
  `decay` (a dimensionless ROW FACTOR r_i, see below), `amp` (signed,
  strike-normalised), `phase` (rad), `attack` (s, per-mode raised-cosine
  onset). Doublets are just two rows. `Validate` requires rows sorted by
  descending energy amp²/(2·α), so "keep the N strongest" is a prefix.
- **Normalised storage — no "fitted" state anywhere.** The review found that
  macros defined "relative to the fitted value" need the fitted values in the
  block at render time, which as knobs breaks identity and as hidden fields
  breaks `TestDeclaredModuleValuesAreLive`. Instead every row is stored
  relative to the models the C code evaluates live:
  - α_i (Np/s) at the ACTUAL frequency f of the mode at this render =
    `r_i · DecayX · α_law(f)`, with α_law(f) = a·(f/1 kHz)^p + b·(f/10 kHz)^6
    and r_i = α_measured_i / α_law_fit(f_i). At the recorded pitch with the
    fitted a/p/b this is exactly α_measured_i; at another pitch the law moves
    every mode (higher notes ring shorter) — transposition needs no reference
    pitch in the block.
  - the strike force spectrum is evaluated at the actual f too (§3c).
  - a/p/b are ordinary absolute knobs whose shipped values ARE the fit.
- DSP: Mathews–Smith phasor resonators — the kernel already in
  `gen_cymbal_modal` (`modular_stages.c`) is factored into a shared
  `modal_bank_*` helper both use (the cymbal output must stay bit-identical,
  pinned by the existing goldens). Each mode stops once it is 100 dB below
  the bank's ANALYTIC peak bound Σ|amp_i| (computed before rendering, so
  deterministic); modes above min(20 kHz, 0.45·sr) are skipped.
- Scalar macros (absolute, identity at shipped values): Tune (cents, 0),
  Stretch (f′ = f·(f/f₀)^s, s = 0), Decay × (1), Brightness (dB/oct, 0),
  Modes (keep the N strongest, N = count), law a / p / b.

### 3c. `strike` stage

**Implemented:** S is the raised-cosine spectrum's smooth ENVELOPE
1/√(1 + (π (f·T_c)³)²) — the same corner and −18 dB/oct asymptote without the
nulls. With the nulls the inverse was ill-posed: a mode next to one needed
×1000 storage, and an 8-mode round trip fitted T_c = 25 ms at +44 dB with
every mode on a sidelobe peak. A real mallet has no exact zeros.

Absolute T_c (s), Hardness (dB/oct) and Click level. The amplitude of mode i
at render = `amp_i · S(f; T_c) · 2^(Hardness · log2(f / 1 kHz) / 6.02)` at the
mode's actual frequency, with S the raised-cosine contact force spectrum.
Because rows are stored divided by the FULL fitted strike (S · hardness tilt)
(§5a), shipping the fitted T_c and Hardness reproduces the measurement, and
both knobs mean "a different mallet". Click = an optional short noise burst of
length T_c. Gated on the `modal` layer being present. (The review caught that
dividing by S alone left the tilt in the rows and applied it twice.)

### 3d. `noiseband` layer

- New gen source (12 at the time of writing), table capacity **32 rows**, at
  most one per document (same slot ↔ table rule as §3b).
- Columns: `center` (Hz), `q`, `level` (dB), `attack` (s), `fast_rate`,
  `fast_mix`, `tail_rate` (Np/s).
- This is the existing gen-bank noise tap (`gen<k>_source = 2` with
  `filt_type = rbjBP`, `env_fast_rate`, `env_fast_mix`, `env_tail_rate`)
  generalised to 32 rows plus an attack — the C reuses that tap's RBJ
  band-pass and two-rate envelope code (factored into a shared helper, the
  existing tap bit-identical), it does not grow a second copy. One seeded
  noise draw per sample, seeded per render (never a static RNG — refine
  renders in parallel). Reusable later for wires, breath and bow noise.

### 3e. ABI, cost, platforms

- ABI grows from 439 to ≈ 1,320 doubles (≈ 10.5 KB per block) for EVERY
  instrument. Rows land in `modularFields`; `make gen-synth-abi` regenerates
  the C header and JS ABI. Pitched dispatch hashes the whole block per
  trigger (`BlockHashWith` in `liveVoiceKey`); plan 2 caches the
  pitch-less hash so the hot path stays O(1) in the block size, and
  measures `setInstrumentBlock` push cost before/after.
- Render target: 128 modes × 3 s in 20–40 ms native, pinned by a budget test.
- WASM uses the same C in the render pool; `xplat_audio_compare` gains a
  modal + noiseband voice.
- The full add-knob checklist applies (layout pin re-frozen, bounds in
  `modular_recipe.go`, `modularStageKnobs`, i18n + plain-English, visibility
  gates, liveness, `make build/libdrums.a`, `make src/js/drums.single.js`,
  goldens, `make gen-fundamentals`, `make measure-loudness`).

### 3f. Synth tab

**Implemented:** sections MODAL / STRIKE / NOISE (after OSC), root categories
**Ring** (modal + noise level) and **Mallet** (strike), the stem graph
(`synth_modes_graph.go`, drawn from `audio.ModalModes`, the Go mirror of the
bank). The stages are TABLE-GATED: a document without rows shows none of
them. No macro dial was added. Scenes `crop_synth_modal_root` /
`crop_synth_modal_graph` use a generated clone-shaped document.

- MODAL / STRIKE / NOISE stage cards exposing the macros (rows are never
  knobs).
- A mode-spectrum stem graph (frequency × level, decay as stem fade) drawn in
  the Visualizers palette, morphing live with the macros.
- Plain-English glosses and i18n keys for every new label.
- Every new visible non-gain knob is claimed by exactly one category card
  (`TestMacroGroups_EveryInstrumentHasCategories`); any macro dial added in
  `synth_dials.json` must pass `TestEveryDialDirectionIsAudibleWhenPlayed`
  (or enter the weak-direction ratchet with a reason).
- Scenes `crop_synth_modal_root`, `crop_synth_modal_graph`; captured and
  inspected on desktop and `MOBILE=1`.

## 4. Harness — loss and optimizer

### 4a. Alignment and level

Candidate renders are onset-aligned to the reference and trimmed to the
reference clip length. Level matching: **equal peak** for percussion,
**momentary loudness** for sustained sounds (the A/B convention).

### 4b. Loss (`clone.Loss`, four reported terms)

1. **MR-STFT:** FFT 256 / 1024 / 4096, hop ¼, log-magnitude L1 floored at
   −80 dB plus spectral convergence; the first 100 ms weighted ×2.
2. **Envelope:** 2 ms RMS in dB, L1 down to −60 dB.
3. **Trusted descriptors:** inharmonicity, tristimulus, crest median via the
   registry's `Compare` (the only descriptors the docs trust).
4. **Prior:** the candidate's parametric modes vs the ESPRIT estimates (keeps
   the optimizer from trading modes for noise).

Weights start EQUAL and validation is advisory until it has data that can
rank: `ear_verdicts.jsonl` has 44 rows — one per family, 8 `y` — and a loss
value is not comparable across families (a cymbal's MR-STFT error and a
bassoon's are on different scales), so a cross-family AUC is confounded. Only
rankings WITHIN a family mean something. `refs validate -loss` therefore
reports per-term within-family agreement on the `refs session` rounds
(`ear_sessions.jsonl`, several rated candidates per family — the clone bench
adds more), and an ANTI-correlated term (agreement < 0.5 over ≥ 10 within-family
pairs) gets weight 0. `fingerprint.Distance` is scored the same way for
comparison. Scores are always computed through `RenderAsShipped`, the same
render the listener heard.

### 4c. Optimizer (`clone.Refine`)

**Implemented, with measured changes:** frequencies are fully locked (the
±10-cent group tune made a struck-bar fixture refine ~4× slower — search noise
on sharp partials swamps the 4096-point term); σ₀ = 0.05 in the sin-boxed
space (0.3 made the average marimba candidate 6× worse than Doc₀ and 30
generations found nothing); λ = 4 + ⌊3 ln D⌋; ≤ 150 generations, stop after 30
without a 1e-4 relative gain; the descriptor term is report-only (≈ 0.4 s per
score). Measured: 24–48 s per instrument on 8 cores (the C render ~33 ms and
the loss ~22 ms per candidate at 3 s / 44.1 kHz).

- CMA-ES in Go (~300 lines; the covariance eigendecomposition reuses `clone/linalg`'s Jacobi), box constraints by transform,
  λ ≈ 16, ≈ 60 generations, early stop on stalled improvement.
- ≈ 40 dimensions of smooth corrections over `Doc₀` — never raw rows: per
  octave group (~10) an amplitude gain and a decay scale; strike T_c and
  hardness; per octave (~8) a noise level and a noise decay scale; ±10 cents
  tune per group. **Frequencies are locked** (Hayes 2023: frequency is the
  non-convex dimension; analysis owns it).
- Parallel: a private `async.Pool` of NumCPU − 1 workers after one warm-up
  render; results collected by index; seeded RNG ⇒ deterministic regardless
  of worker count.
- Budget ≈ 10–30 s per instrument; measured and reported.
- The search space is derived generically from the document, so later
  families reuse it.

## 5. Inversion and transposition

### 5a. Building the candidate (`clone.Invert`)

- A fresh document: `modal` layer + `strike` stage + `noiseband` layer
  (residual rows), the existing amp / FINISH stages at neutral values, **no
  FX** (the reference already contains its room; a long diffuse residual tail
  is flagged in the report).
- `-into <id>` keeps the identity fields (id, name, category, `render.beats`,
  notes) and replaces the sound modules. The shipped file is written only by
  `clone adopt` after a `y`.
- Mode budget: rank by energy amp²/(2α); keep ≤ 128; drop below −60 dB of the
  strongest; doublet pairs stay together.
- Amplitudes are stored **strike-normalised** by the FULL fitted strike
  (measured / (S(f; T_c,fit) · hardness tilt_fit)), and the fitted T_c /
  Hardness are written as the strike stage's absolute values — so the shipped
  document reproduces the measurement and either knob behaves like a
  different mallet (§3c).
- Decays are stored as row factors r_i = α_i / α_law_fit(f_i) and the fitted
  a / p / b as the law's absolute values (§3b) — identity at the shipped
  values with no fitted state in the block.
- CMA-ES corrections are **baked into the rows**; the shipped document carries
  rows + identity macros only.
- `render.seconds` = time to the noise floor, and `render.beats` =
  `render.seconds × 120 / 60`, so desktop (beats) and browser (seconds) agree
  at the default 120 BPM (see the platform divergence in §1).
  `render.amplitude` is computed by the same code as `make measure-loudness`.

### 5b. Pitch

- **The fit and every listen happen at the RECORDED pitch.** Melodic voices
  render from 220·2^(p/12) Hz, so a C4 reference is matched at node pitch
  `semis_from_a3` = +3, never at 0. Refine, Loss, the report WAVs and the A/B
  all render at that pitch; at pitch 0 the voice is a transposition (§3b's law
  and §3c's strike move the modes) and is not what the reference recorded.
- **Pitched (melodic):** `freq_mode` = ratio to the measured nominal,
  `render.melodic` = true. The melodic convention holds — node pitch 0 sounds
  A3 (a C4 recording plays 3 semitones lower at pitch 0), and
  `gen-fundamentals` treats it as declared, never measured. Tubular bells use
  the pinned virtual pitch (lowest ratio may be < 1).
- **Transposition by one reference note:** at transposition k, frequencies
  scale by k, each decay becomes decay_i · α(k·f_i)/α(f_i) (higher notes ring
  shorter, as real bar and bell families do), and the strike spectrum is
  re-evaluated at the new frequencies with T_c fixed (same mallet — modes past
  the 2/T_c null fade). Modes above min(20 kHz, 0.45·sr) are skipped. Both
  fall out of the normalised storage (§3b/§3c): the C evaluates the law and
  the strike at each mode's actual frequency, so no transposition factor or
  reference pitch is stored.
- **Unpitched / percussion** (tom, conga; cowbell if it fails the pitch gate):
  `freq_mode` = Hz, not melodic; node pitch uses the engine's existing
  resampling like every drum.
- `Analysis` records the reference note so a later spec can pool modes from
  several notes into one α(f) fit.

## 6. CLI and listening workflow

### 6a. `cmd/clone` (and `make clone INST=<id>`)

```
clone -inst <id> [-wav <path>] [-f0 <Hz>] [-as <newid> | -into <id>]
      [-no-refine] [-seed N] [-gens N] [-json]
clone adopt <id>
```

- Default new id: `<id>-clone`.
- Writes `$BEATMO_WAV_REFS/clones/<id>/` (NOT `candidates/`, which already
  holds competing reference recordings for `refs candidates` / `refs-check`): the candidate document,
  `analysis.json`, `report.md` / `report.json`, and level-matched WAVs of
  reference, current shipped instrument and candidate.
- Report: mode table (Hz, ratio, cents from harmonic, T60, dB), "missed"
  modes from the cross-check, loss terms before / after refine, and a timing
  breakdown (Prepare, ESPRIT, Invert, each CMA generation, total).
- Listening: `make ab-session FAMILIES=<id> CLONE=1` adds the candidate to
  the existing `refs session` round (reference + current + candidates, all
  level-matched against the one shared reference clip, rendered at
  `semis_from_a3`). The session row (`ear_sessions.jsonl`) records the clone
  as a candidate with `doc_sha` = the candidate document's content hash
  computed with `render.amplitude` zeroed (measure-loudness rewrites that
  field after adopt and must not invalidate the verdict), and the clone flow
  adds a `y / n / p` accept prompt to its candidate.
- `clone adopt <id>` writes `internal/assets/instruments/<id>.json` only if
  the newest session row for that exact `doc_sha` has accept = `y`, then
  prints the follow-up steps (`make asset-pack`, `go generate
  ./internal/audio/`, `make measure-loudness`, `make gen-fundamentals`).

### 6b. Benchmark (`make clone-bench`, local only)

| Group | Instruments | Expectation |
|---|---|---|
| fail → fix | tubular-bells, kalimba | `y` |
| partial | glockenspiel, cowbell | `y` |
| controls | marimba, xylophone, vibraphone | clone also `y` (no regression vs shipped) |
| informative | tom, conga | reported, NOT gated: drum heads glide (non-goals); the report's glide diagnostic says by how much |

Vibraphone references may carry motor tremolo; the fit sees it as close
doublets / sidebands. If the control fails on that, the report says so and the
fix belongs to a later spec (an AM term), not to slice 1.

Writes a summary table (loss, mode count, time per instrument). The table
ranks; verdicts decide.

## 7. Error handling

Refuse with a reason, never guess silently:

| Condition | Result |
|---|---|
| No onset found | refuse |
| Decay window < 150 ms | refuse |
| Clipped reference (peak ≥ 0.999 on ≥ 3 samples) | refuse |
| Residual > 50% of energy after the modal fit | refuse: "not a struck/modal sound" (catches sustained / bowed input) |
| Pitch unusable and no `-f0` | proceed unpitched (Hz mode), stated in the report |
| Refine fails or raises the loss | keep the analysis-only candidate, stated in the report |

Partial results (analysis, report) are always written.

## 8. Testing

No test reads reference audio (`TestNoTestReadsReferenceAudio`); all test
audio is generated by `wavgen`. Corpus runs are `make clone-bench` only.

- **Known answer (`-tags test`, pure Go):** synthetic modal sums with known
  f / α / a plus noise at 40 dB SNR — ESPRIT recovers f within 0.5 cent, α
  within 5 %, amplitude within 0.5 dB; resolves a doublet 1 Hz apart from a
  2 s clip; `EstimateStrike` recovers T_c from a synthetic raised-cosine
  strike; `FitDecayLaw` recovers a / p / b; residual bands recover known
  rates STATISTICALLY (mean over 10 seeds unbiased within 10 %): a
  band-limited noise envelope carries only ~B·τ independent samples, so a
  single draw of a 60/s decay in a 2 kHz third-octave cannot be measured
  within 25 % by any estimator.
- **Refusals:** each row of §7 has a generated input that triggers it.
- **Determinism:** same seed ⇒ byte-identical document; parallel and serial
  refine agree.
- **Tables:** `Flatten` / `Validate` / `Clone` / `Diff` / hash cover tables;
  `SetTable` rejects bad columns, over-capacity and out-of-bounds values;
  `TestTablesAreModelParametersOnly`.
- **Engine (real C, `!test`):** round trip (known `modal` document → render →
  clone → render: parameters recovered, MR-STFT under threshold); macros at
  identity are bit-identical; `TestDeclaredModuleValuesAreLive` and
  every-param-mutates cover the new modules; layout pin and
  `TestGeneratedCHeaderIsCurrent`; render budget test; `xplat_audio_compare`
  modal voice.
- **Render parity (`!test`):** `RenderAsShipped(doc)` byte-identical to
  `RenderAsPlayed(id)` for every shipped instrument at pitch 0 and +7.
- **Imports:** `internal/audio/clone` unreachable from production mains
  (`TestCloneIsDevOnly`).
- **Linear algebra:** Hermitian Jacobi reconstructs A = VΛVᴴ; complex QR
  eigenvalues match a matrix built from known eigenvalues; least squares
  matches a known solution.
- **UI:** the MODAL / STRIKE / NOISE scenes captured and inspected (desktop +
  `MOBILE=1`).

## 9. Documentation

- `AGENTS.synth.instruments.md`: a "Cloning a struck instrument" section (the
  pipeline, the data rule, the module semantics, the adopt gate).
- `CLAUDE.md` Quick Start: `make clone`, `make clone-bench`, `clone adopt`;
  a gotcha line for `RenderAsShipped` being the only analysis render.
- `AGENTS.synth.md`: the `modal` / `strike` / `noiseband` modules and tables in
  documents.

## Risks and things to measure first

- **Loss validity:** 44 verdict rows is a small sample for AUC; terms are
  reported separately so a bad term is visible, and the ear stays the gate.
- **ESPRIT on short or noisy clips:** the 150 ms floor and SVD-gap fallback
  are first guesses; the known-answer tests pin behaviour, the bench reports
  real-clip mode counts.
- **ABI growth** (≈ 3× block size): affects every `setInstrumentBlock` push
  and the voice-cache key cost; measured during implementation.
- **Transposition realism** rests on a single-note decay law; acceptable for
  slice 1 by decision, revisited with multi-note fitting.

## Load-bearing references

- Laroche 1993, "The use of the matrix pencil method for the spectrum
  analysis of musical signals" (JASA).
- Badeau, David, Richard 2002/2006: ESTER order selection; HR methods for
  audio (IEEE TSP / TASLP).
- Aramaki, Kronland-Martinet et al. 2006: analysis–synthesis of impact
  sounds; sub-band high-resolution modal estimation.
- Mathews & Smith 2003: phasor-filter modal resonators.
- Serra & Smith 1990 (SMS); Levine & Smith 1998 (sines + transients + noise).
- Chaigne & Lambourg 2001 / plate damping laws (α(f) form used in the cymbal
  model).
- Hansen 2016, "The CMA Evolution Strategy: a tutorial".
- Hayes et al. 2023, sinusoidal frequency estimation by gradient descent
  (non-convexity of frequency parameters).
- Yamamoto et al. 2020 / Engel et al. 2020: multi-resolution STFT loss.
