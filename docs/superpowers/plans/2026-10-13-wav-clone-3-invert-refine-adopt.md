# WAV clone 3 — invert, loss, refine, adopt, Synth tab — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** turn an `Analysis` (plan 1) into an instrument document on the
plan-2 stages, score it with a four-term loss, refine it with seeded parallel
CMA-ES, put it in front of the ear (`make ab-session CLONE=1`), ship it only
on a recorded `y` (`clone adopt`), and expose the MODAL / STRIKE / NOISE
stages on the Synth tab.

**Architecture:** everything numeric stays in `internal/audio/clone` (pure,
`-tags test`-testable; the real render is INJECTED as a `RenderFunc`, bound to
`audio.RenderAsShipped` only in `!test` files of `cmd/clone` / `cmd/refs`).
A pure-Go mirror of `modal_bank.c`'s per-mode math (`DocModes`) gives Invert
an exact identity test without C, and gives the loss its prior term.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), the plan-2 C stages,
`internal/async.Pool`, the fingerprint registry, `cmd/refs` session plumbing,
Ebiten UI (Synth tab).

**Spec:** `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md`
(§3f, §4, §5, §6, §7, §8, §9). Plans 1–2:
`docs/superpowers/plans/2026-10-12-wav-clone-1-render-parity-and-analysis.md`,
`docs/superpowers/plans/2026-10-13-wav-clone-2-modal-engine-modules.md`.

## Global Constraints

- Never `git add` / `git commit` (the user commits); never `git stash`.
- No test reads reference audio; generated audio only (`wavgen`, `clone.Synthesize`).
- A document carries model numbers only — no PCM, no wavetable (`TestTablesAreModelParametersOnly`).
- `internal/audio/clone` stays dev-only (`TestCloneIsDevOnly`).
- Use `.tools/go/bin/go`; fast path `-tags test -modfile=go.test.mod`, real C via `make test-real`-style `!test` runs.
- UI changes are done only after a captured, opened screenshot (desktop + `MOBILE=1`).
- Fit and every listen happen at the RECORDED pitch (spec §5b).
- Rows are model parameters, never knobs; the shipped document carries identity macros (tune 0, stretch 0, decay × 1, bright 0, keep = count).

## Review Focus

1. **A mode next to a strike null** — dividing by S(f; Tc) ≈ 0 makes a row amp explode past the column bound (±1000); Invert must clamp, count it, and the report must say so (test in Task 1).
2. **Percussion references at a non-zero `semis_from_a3`** — a Hz-mode clone played at pitch s is RESAMPLED by 2^(s/12); Invert must pre-scale the model so the render at the session pitch equals the measurement (test in Task 1).
3. **Refine making it worse** — a refined loss ≥ the analysis-only loss keeps Doc₀ and says so (test in Task 4).
4. **Adopt without a matching verdict** — a stale session row (different `doc_sha`), an `n`/`p`, or no row must refuse; `render.amplitude` edits must not invalidate the verdict (test in Task 5).
5. **Determinism under parallelism** — the same seed must give the byte-identical document for 1 and N workers (test in Task 4).

---

### Task 1: `clone.Invert` + `clone.DocModes` (pure)

**Files:**
- Create: `src/go/internal/audio/clone/invert.go`, `docmodel.go`, `loudness.go`
- Test: `src/go/internal/audio/clone/invert_test.go`

**Interfaces:**
- Produces:
  - `type InvertOptions struct { ID, Name, Category string; Into *instrument.Doc; MaxModes int; FloorDB float64; FitPitch float64 }`
  - `type InvertReport struct { Kept, Dropped, Clamped, NoiseRows int; FitPitch float64; Melodic bool; Notes []string }`
  - `func Invert(a Analysis, opt InvertOptions) (instrument.Doc, InvertReport, error)`
  - `func (p PitchInfo) RenderPitch() float64` — `12·log2(F0/220)` when pitched, else `SemisFromA3` when known, else 0.
  - `func DocModes(d instrument.Doc, pitch float64, rate int) []Mode` — mirrors `modular_modal_render` (ratio/Hz, stretch, tune, the 0.45·sr limit, law, decay ×, bright, strike S·hardness); `Band` = row index.
  - `func DocNoise(d instrument.Doc) []NoiseBand`
  - `func LoudnessNorm(samples []float64, rate int) (float64, bool)` — the rule `cmd/measure-loudness` applies (loudest-300 ms RMS/peak); measure-loudness calls it.

Rules (spec §5a):
- Melodic iff `a.Pitch.Pitched`; then `modal_freq_mode` = 0 and rows = f / F0 with `FitPitch` = `RenderPitch()` (voice frequency at the fit pitch = F0 exactly). Otherwise Hz mode, not melodic, and the model is pre-scaled into the pitch-0 domain by k = 2^(s/12) (f/k, α/k, attack·k, noise centre/k, rates/k, Tc·k) so resampling by k at pitch s reproduces it.
- Modes ranked by `Energy()`; drop below `FloorDB` (−60) re the strongest; keep ≤ `MaxModes` (128); a doublet (≤ 0.5 % apart) is never split at the cap.
- Law refit on the kept modes and clamped into the knob bounds; rows `decay` = α / law(f); `amp` = A / (S(f; Tc)·HardnessGain(f, H)), clamped to ±1000 (counted); phase as measured; attack 0.
- Noise rows: active residual bands, loudest first, ≤ 32, values clamped to the column bounds.
- Stages: `core`, `finish` (registry `NewModule` defaults), `modal`, `strike` (click 0 — the residual rows already carry the contact noise), `noiseband`. No FX.
- `render.seconds` = time for the strongest-energy model to reach the measured floor, clamped to [0.25, 10] s; `beats` = seconds × 2; amplitude 1 (the CLI sets it). `Into`: keep ID, Name, Category, Notes, Render.Beats/Seconds.

- [ ] Test `TestInvertReproducesTheMeasuredModes` — a synthetic Analysis (8 modes, a fitted strike, a fitted law) → `DocModes(doc, FitPitch, rate)` equals the analysis modes (f within 1e-9 rel, α 1e-9, amp 1e-9) both pitched and unpitched-with-semis.
- [ ] Test `TestInvertPercussionPreScalesForTheSessionPitch` — unpitched, semis −5: `DocModes(doc, 0)` scaled by k equals the measurement.
- [ ] Test `TestInvertBudget` — 200 modes → 128 kept, energy order, −60 dB floor drops, doublet kept together.
- [ ] Test `TestInvertClampsRowsNearAStrikeNull` — a mode at 2/Tc is clamped and counted.
- [ ] Test `TestInvertValidates` — `instrument.Validate` passes; `Into` keeps identity fields.
- [ ] Implement, run, pass.

### Task 2: real-C round trip (`!test`)

**Files:** Create `src/go/internal/audio/clone/roundtrip_test.go` (`//go:build !test`).

- [ ] `TestCloneRoundTripThroughTheEngine` — a known `modal` document (8 modes + 2 noise rows) → `audio.RenderAsShipped` at 48 kHz → `Prepare` → `Analyze` → `Invert` → render again: the 6 strongest frequencies recovered within 1 cent, α within 10 %, and the loss's MR-STFT term under 1.5 dB.
- [ ] Run with the real C (no `-tags test`).

### Task 3: `clone.Loss`

**Files:** Create `src/go/internal/audio/clone/loss.go`, `stft.go`; test `loss_test.go`.

**Interfaces:**
- `type LossWeights struct{ MRSTFT, Envelope, Descriptors, Prior float64 }` — `DefaultLossWeights` all 1.
- `type LossReport struct { MRSTFT, Envelope, Descriptors, Prior, Total float64; Desc map[string]float64 }`
- `type LossRef struct{…}`; `func NewLossRef(p Prepared, maxSec float64) *LossRef` — reference aligned at its onset, trimmed, spectrograms precomputed.
- `func (r *LossRef) Score(cand []float64, prior float64, w LossWeights, scale LossReport) LossReport` — candidate aligned by the same onset rule, trimmed, equal PEAK; terms: MR-STFT (256/1024/4096, hop ¼, log-mag L1 floored at −80 dB re the reference max + spectral convergence, first 100 ms ×2), envelope (2 ms RMS dB L1 clamped at −60 dB), descriptors (mean Norm of `partial.inharmonicity`, `timbre.tristimulus`, `track.crest.median` via `Descriptor.CompareWith`; NA skipped), prior; `Total` = Σ wₖ·termₖ / scaleₖ (scale 0 ⇒ 1).
- `func PriorTerm(doc instrument.Doc, a Analysis, pitch float64, rate int) float64` — energy-weighted mean |dB| of amp and decay ratios of `DocModes` vs the analysis' kept modes.

- [ ] Tests: identical signals score 0 on every term; a 2 ms shift / 6 dB level change score ~0 (alignment + peak match); a detuned mode, a faster decay and added noise each raise MR-STFT monotonically; scales normalise.

### Task 4: CMA-ES + `clone.Refine`

**Files:** Create `cmaes.go`, `refine.go`; tests `cmaes_test.go`, `refine_test.go`.

**Interfaces:**
- `type CMAES struct{…}`; `NewCMAES(dim int, sigma float64, lambda int, seed uint64)`; `Ask() [][]float64`; `Tell(xs [][]float64, f []float64)` — Hansen 2016 tutorial, covariance eigendecomposition via `linalg.EigHermitian`.
- `type RenderFunc func(d instrument.Doc) []float64`
- `type RefineOptions struct { Render RenderFunc; Pitch float64; Gens, Lambda, Workers int; Seed uint64; Weights LossWeights; StallGens int }`
- `type RefineResult struct { Doc instrument.Doc; Before, After LossReport; Kept bool /* analysis-only kept */; Gens []GenTrace; WarmupMs, TotalMs float64; Dims []string }`
- `func Refine(doc0 instrument.Doc, ref *LossRef, a Analysis, opt RefineOptions) (RefineResult, error)`
- Search space (derived from the document): per populated octave group of mode rows — amp gain (±12 dB), decay scale (log2 ±1), tune (±10 cents); strike Tc (log2 ±1) and hardness (±6 dB/oct); per populated octave of noise rows — level (±12 dB) and decay scale (log2 ±1). Box by `lo + (hi−lo)(1+sin y)/2` (y = 0 ⇒ identity). Corrections are BAKED into rows / strike values.
- Parallel: a private `async.Pool` (`Workers` default NumCPU − 1) evaluates an Ask batch by index; the RNG is consumed only in `Ask` ⇒ deterministic for any worker count.

- [ ] `TestCMAESMinimisesAQuadratic` (rotated ellipsoid, D = 10).
- [ ] `TestRefineIsDeterministicAcrossWorkerCounts` (pure-Go `RenderFunc`: `Synthesize(DocModes)`), byte-identical `MarshalFile`.
- [ ] `TestRefineRecoversAPerturbedModel` — Doc₀ with a +6 dB HF group and ×1.5 decays → refine lowers the loss ≥ 50 %.
- [ ] `TestRefineKeepsDoc0WhenItCannotImprove` — `Gens = 0`/a render that only worsens ⇒ `Kept`.

### Task 5: `cmd/clone` — candidate, WAVs, report, adopt, bench

**Files:** Modify `src/go/cmd/clone/main.go`, `main_test.go`, `internal/audio/clone/report.go`; create `src/go/cmd/clone/render_prod.go` (`!test && !js`), `render_stub.go` (`test`), `adopt.go`, `adopt_test.go`; Makefile (`clone` args, `clone-bench`, `clone-adopt`); `cmd/measure-loudness/main.go` (calls `clone.LoudnessNorm`).

CLI: `clone -inst <id> [-ref] [-wav] [-f0] [-as <newid> | -into <id>] [-no-refine] [-seed N] [-gens N] [-workers N] [-json] [-out] [-cpuprofile]` and `clone adopt -inst <id> [-sessions <jsonl>]`.
Writes `clones/<inst>/`: `analysis.json`, `candidate.json` (the document), `meta.json` (`{ref, source, fit_pitch, doc_id, doc_sha, rate}`), `docs/<sha>.json` (history for `refs validate -loss`), `report.md` / `report.json` (mode table, missed modes, loss before/after, refine trace, timing breakdown), `reference.wav`, `current.wav`, `candidate.wav` (equal peak, recorded pitch).
`doc_sha` = `instrument.ContentHash` with `render.amplitude` zeroed (`clone.DocSHA`).
Amplitude: `L / LoudnessNorm(candidate)` with L = shipped amplitude × `LoudnessNorm(current)` when the instrument ships, else unchanged.
Adopt: newest `ear_sessions.jsonl` row holding a candidate with that `doc_sha` must have `accept = "y"`; writes `internal/assets/instruments/<id>.json` (adds the id to `index.json` if new) and prints the follow-ups.

- [ ] Tests (`-tags test`, stub render): flags parse; `-as`/`-into` exclusive; adopt refuses no-row / `n` / stale sha, accepts `y`, ignores an amplitude-only change; output files exist for a generated WAV (`-no-refine`).

### Task 6: listening — `refs session -clone`, `refs validate -loss`

**Files:** Modify `cmd/refs/session.go`, `session_record.go`, `compare.go`, `validate.go`; `internal/audio/synthmatch/render_prod.go` / `render_stub.go` (`RenderShippedDoc`); Makefile `ab-session` (`CLONE=1` → `-clone`). Tests: `session_clone_test.go`, `validate_loss_test.go`.

- Session: with `-clone`, every family with `clones/<inst>/candidate.json` gets candidate `K` (label "clone") rendered from the document at the clone's fit pitch in the SAME level-matched set; families absent from the candidates file run with just `K`. The record stores `doc_sha` and an `accept` (`y`/`n`/`p`) asked right after K's rating.
- Validate: `-loss` scores the four terms (and `fingerprint.Distance`) for the preference pairs through `scorePreferencesFromNorms` — within-family by construction — and flags a term ANTI at agreement < 0.5 over ≥ 10 pairs (weight 0 recommended).

### Task 7: `xplat_audio_compare` modal voice

**Files:** `src/go/cmd/export_audio/main.go` — a `clone-modal` param case (a fixed modal + strike + noiseband document, not a roster id) so the browser renders the same block; check `src/js/xplat_audio_compare.browser.test.js` picks it up.

### Task 8: Synth tab MODAL / STRIKE / NOISE

**Files:** `internal/audio/modular_recipe.go` (labels + groups `modal` / `strike` / `noiseband`, units, tiers), `modular_stage_params.go`, `synthdiag/stages.go`; `internal/ui/synth_panel_zone.go` (sections), `synth_macro_groups.go` (a "Resonance" category for both tables), `plain_english.go`, `synth_focus_graph.go` (mode-spectrum stem renderer), i18n keys, `scene_catalog.go` (`crop_synth_modal_root`, `crop_synth_modal_graph`).

- [ ] Tests: `TestMacroGroups_EveryInstrumentHasCategories` with a modal document; stage enable pills toggle `modal_enabled` / `strike_enabled` / `noiseband_enabled`; the stem graph morphs with Tune / Decay ×.
- [ ] Screenshots desktop + `MOBILE=1`, opened and inspected.

### Task 9: docs, corpus run, memory

- `AGENTS.synth.instruments.md` (invert / refine / adopt), `CLAUDE.md` Quick Start (`make clone-bench`, `clone adopt`), spec status, plan execution notes, memory.
- Run `make clone` on the bench instruments (local corpus): loss before/after, mode counts, timings → report to the user; verdicts are theirs.

---

## Execution notes (2026-10-13, executed inline)

All nine tasks done. Deviations, each forced by a measurement or a test:

- **Task 1.** A mode far above a long contact's corner also exceeds the ±1000
  row bound (the clamp test uses three modes). Rows a percussion pre-scale
  pushes out of a column's range are dropped (conga at +22 st put the 31.5 Hz
  band at 8.8 Hz). `DocSHA` lives beside `Invert`.
- **Task 2 (round trip) found four analysis/engine bugs** — see
  AGENTS.synth.instruments.md "Invert → refine → listen → adopt": the strike
  spectrum's nulls (→ smooth envelope, C and Go; EstimateStrike's −60 dB floor
  removed, ties go to the shortest contact), cancelling fast-pole clusters
  (→ transient hygiene loop), the two-rate residual spike (→ t = 0 cap) and
  rate-0 bursts. The test fixture must ring out inside the render.
- **Task 3.** The loss carries its own two-for-one real FFT (50 → 22 ms per
  score; `wave.MagnitudeSpectrum` allocates three buffers per frame);
  descriptor Norms are taken net of their identity Norm.
- **Task 4.** Tune dimensions dropped; σ₀ 0.05; λ = 4 + ⌊3 ln D⌋; 150 / 30
  stall; descriptor term report-only and normalised by 1. NOISEBAND rows now
  stop 100 dB under their own level (render 46 → 33 ms).
- **Task 5.** `-into` / `-as` exclusive; `clone adopt -repo/-dir/-sessions`
  overrides for tests; measure-loudness calls `clone.LoudnessNorm`; the
  dev-only allowlist gained `cmd/refs` and `cmd/measure-loudness`.
- **Task 6.** `renderFamilyCompareSpecs` generalises the family render (a
  document option at its own fit pitch); `sessionClip.DocSHA`,
  `sessionCandidate.Accept`; `refs validate -loss` reuses
  `scorePreferencesFromNorms`.
- **Task 7.** `audio.RenderDocRaw`; the browser half of
  `xplat_audio_compare` now zeroes the block and fills table cells from
  `MODULAR_PARAM_TABLES` (they were uninitialised `malloc` memory — harmless
  only while every case had 0 rows).
- **Task 8.** The stages are table-gated (`tableGatedStages`,
  `IsTableGatedField`); `audio.ModalModes` is the one Go mirror (UI graph +
  `clone.DocModes`); categories Ring / Mallet; `conceptModes` falls back to the
  level wave when there is nothing to draw; Partials prints as an integer and
  Tune in cents. Screenshots inspected (desktop + mobile, both scenes).
