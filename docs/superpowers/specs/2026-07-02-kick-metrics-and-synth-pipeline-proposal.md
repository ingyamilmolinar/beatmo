# Proposal: richer (windowed) metrics + synth-pipeline real-time improvements

Status: **proposal for review** (not yet implemented). Context: the kick-tuning
work built a time-resolved fingerprint (`internal/audio/fingerprint/kick_metrics.go`)
and 8 kick DSP variants. Two questions from here: (1) which metrics should be
compared **per time-interval** vs whole-wave, and (2) new DSP metrics + synth
pipeline refactors/optimizations, especially for **real-time** param updates.

---

## Part 1 — Whole-wave vs per-interval: the per-metric audit

**Principle.** A single whole-wave number tells you *that* two kicks differ; a
**per-segment** number tells you *where in time* (→ *which knob*). Perceptual and
DSP structure is segment-based — **ATTACK** ↔ click/beater, **BODY** ↔
modes/harmonics, **TAIL** ↔ decay + reverb — so a per-segment distance grid maps
divergences straight to parameters. Several of our metrics are *already* windowed
traces (good for the distance), but the traces are **hard to read** ("window 7
diverges" isn't actionable) — so the proposal is a **segment layer on top of the
traces**, not replacing them.

### Canonical segments (aligned between synth & ref by envelope landmarks)
- **ATTACK**: 0 → attack-peak + ~10 ms (~0–35 ms) — the transient/click.
- **BODY**: attack-peak → −20 dB (~35–150 ms) — the tonal modes/harmonics.
- **TAIL**: −20 dB → end (~150–500 ms) — decay + reverb.

(Landmarks already exist: `AttackPeakSec`, the −12/−20 dB crossings, tail region.)

### Per-metric verdict

| Metric | Today | Verdict | Why per-segment helps |
|---|---|---|---|
| Envelope (`EnvTrace`) | trace | keep trace | shape is the distance; landmarks are the scalars |
| Pitch (`PitchTrace`) | trace | **segment medians** | per-window pitch on a 55 Hz kick is noisy; attack/body/tail *median* pitch is robust and maps to glide vs settle |
| Bands (`BandTrace`+avg) | trace + whole avg | **per-segment avg** | "the BODY is too bright" is actionable; a per-window compare is noisy and un-mappable |
| Crest | **whole scalar** | **segment** | attack-crest vs body-crest isolates *where* the transient character differs; whole crest hid the reverb-vs-click tradeoff |
| HF-ratio | trace + maxjump | keep | already the spit detector |
| Centroid | trace + attack scalar | **add body + tail centroid** | brightness per region → target click (attack) vs modes (body) vs reverb (tail) |
| Flatness | **whole voiced avg** | **segment** | tail flatness (reverb diffuseness) ≠ body flatness (tonal); the creak lived in body flatness |
| MFCC | **whole (sustain win)** | **segment (attack/body/tail) or frame-wise DTW** | *highest-value* windowing — tells you which region's timbre is wrong; whole-MFCC can match while the attack is totally off |
| LowMid | trace + attack scalar | keep | `AttackLowMidRatio` is already loud-onset-targeted (the kick-vs-tom win) |
| Reverb (`TailRatio`/`Dur`) | tail scalars | **split early vs late tail** | early tail = first reflections, late = diffuse wash; different knobs |

### Deliverable: `KickSegmentDistance`
A `[]Segment × metric` grid + a `SegmentBreakdown` returned alongside the existing
`KickDistanceBreakdown`. The tuner (and a future "match meter") reads it as: *"ATTACK
centroid +1.2 oct → darken the click; TAIL flatness −0.1 → reverb not diffuse
enough; BODY MFCC Δ high → mode balance."* This is the concrete form of "target
specific changes at specific times." Cheap: reuse the existing per-window
spectra, just aggregate them into 3 segment bins instead of (or in addition to)
the whole-signal average.

---

## Part 2 — New DSP metrics (ranked by leverage, given what we've hit)

1. **Roughness / sensory dissonance** (Plomp–Levelt / Sethares). *Would have
   auto-caught the "creak"* — it measures beating/intermodulation between
   partials (dissonant products of inharmonic modes + saturation), which was
   invisible to every current metric. Compute from peak-picked partials: Σ over
   partial pairs of `d(f_i,f_j)·min(a_i,a_j)` with the Plomp-Levelt dissonance
   curve. **Add as a body-segment metric.** Highest value.
2. **Per-band decay time (T60 per band)** — a "decay spectrogram": sub rings long,
   click dies in ms. Directly targets `env0`/`env1`/`mode_decay`/reverb per region.
3. **Spectral flux / warble rate** — rate + periodicity of spectral change;
   catches sweeps, warbles, and creaks (temporal modulation) that a static
   spectrum misses. Complements roughness.
4. **Inharmonicity** (Σ partial deviation from `n·f0`, amplitude-weighted) — the
   metallic/wooden vs pure axis; formalizes what `mode_detune` controls.
5. **Log-attack-time + temporal centroid** (MPEG-7) — attack sharpness and "where
   the energy sits in time" beyond crest.
6. **Perceptual loudness trajectory** (K-weighted / momentary-LUFS) — matches
   *perceived* level over time, not raw RMS (a bright quiet tail can read louder).
7. **Onset/transient count** — detects double-hits/flams and "is the tail-bloom a
   second onset?" (relevant to the variant-6 bloom).
8. **Formant / fixed-resonance peaks** — the body's cabinet resonances that mark a
   real recording (peaks that *don't* move with pitch).

Each new metric should be added **segment-aware** from day one (Part 1) rather
than as another whole-wave scalar.

---

## Part 3 — Synth pipeline: real-time updates + refactoring

### 3a. Real-time param-update path (the biggest win)
Known issue (`[[project_synth_param_rerender_storm]]`): a live param edit evicts the
per-pitch voice cache → **every note does a synchronous ~40 ms `render_modular`
ccall on the main thread** → jitter / audio dropouts, worst on melodic
per-pitch instruments. Proposed, in order of leverage:
- **Coalesce/debounce edits** — a slider drag fires many `setInstrumentParam`s;
  render once on settle (trailing debounce ~30–50 ms), not per event.
- **Render off the main thread** — the render is pure CPU; move it to a Web
  Worker (browser) / the `async` pool (desktop) and swap the buffer in when
  ready. The audio thread keeps playing the previous buffer meanwhile (no glitch).
- **Base-pitch render + resample** — render ONE buffer at a base pitch and
  pitch-shift for other notes, instead of N per-pitch renders (kills the melodic
  storm). Trade a little fidelity for a huge cache-miss reduction.
- **Stage-boundary caching / incremental render** — the modular pipeline is
  osc/FM/env/filter/**post**/gen-bank; if an edited param only affects a late
  stage (e.g. POST gain, or `kick_reverb`), re-run only that stage over a cached
  pre-stage buffer. Needs the pipeline to expose stage buffers (a real refactor,
  highest fidelity payoff).
- **Param smoothing** — ramp live param changes over a few ms so an edit mid-note
  doesn't click.

### 3b. DSP refactoring
- **Split `modular_gen_slot_kick`** — it's now 8 variants (~500 lines) in one
  function. Extract per-variant `render_kick_variantN(...)` and, more importantly,
  the **reusable DSP units** that have emerged: `beater_click`, `lowmid_texture`,
  `modal_bank`, `feedback_reverb`, `pitch_glide`. A small composable DSP-unit
  library lets new instruments (and non-kick voices) reuse them, and makes each
  unit unit-testable in isolation.
- **Shared windowing/segmentation utility** in `fingerprint` — every metric
  re-implements window/hop loops; one `forEachWindow`/`segments()` helper (Part 1
  needs it anyway) removes duplication and guarantees consistent framing.
- **Metric perf for real-time feedback** — if we ever want a live "match meter",
  the segment framework enables **incremental** metric updates (recompute only the
  changed segment), which is what makes real-time metric display affordable.

### 3c. Measurement first
Before optimizing 3a, add a **render-latency counter** (the `perfStats()` path
already exists) that records `render_modular` call duration + cache hit/miss rate
per param edit, so we optimize against numbers, not guesses (the runtime-profile
discipline: every divergent knob carries a bench citation).

---

## Recommended phasing
1. ✅ **Segment framework** (Part 1) — **DONE**: `internal/audio/fingerprint/kick_segments.go`
   — `KickSegmentAnalyze` splits a kick into fixed-time ATTACK/BODY/TAIL segments
   (fixed, not envelope-derived, so boundaries align across signals — envelope
   bounds shift when the signals differ and fabricate divergence), each with
   energyFrac / centroid / flatness / lowMid / bands / MFCC; `KickSegmentDistance`
   returns the per-segment grid + a ranked "top divergences" list. On the shipped
   acoustic vs its ref it immediately surfaced: attack energyFrac 0.92 vs 0.52
   (ref far more transient-front-loaded), body lowMid 0.98 vs 2.80 (ref body is
   mid-dominant), attack flatness 0.28 vs 0.41 (synth attack noisier). Per-segment
   CREST was deliberately dropped (confounded by segment length/decay). Demo:
   `go test -run TestKickSegmentReport -v ./internal/audio/`.
2. ✅ **Roughness + per-band-T60 metrics** (Part 2 #1–2) — **DONE**:
   `fingerprint/kick_roughness.go` (`Roughness` — Plomp–Levelt/Sethares sensory
   dissonance on the body partials; the metric that would have auto-caught the
   "creak") + `fingerprint/kick_decay.go` (`BandT60`/`BandT60Floored` — per-band
   −60 dB ring-out, the "decay spectrogram"). Both folded into `KickWeights` +
   `KickDistanceBreakdown` (weights 1.0 / 0.75, below the dominant trace/timbre
   terms; identity distance stays 0). Measured on the sandyrb ref: Roughness
   0.024, BandT60 ≈ 0.4 s across bands.
3. ✅ **DSP-unit extraction** (3b) — **DONE**: `src/c/synth_dsp_primitives.h`
   (`sp_onepole` / `sp_osc` / `sp_pitch_glide` / `sp_reverb3` as `static inline`
   composable units). The acoustic kick (variant 7) was rewritten to COMPOSE them;
   `TestVariant7RefactorByteIdentity` pins a bit-checksum proving the render is
   **byte-identical** (zero sound change). The one-pole LP that was open-coded 3×
   (click, texture, reverb feedback) is now one primitive.
4. ◑ **Real-time pipeline** (3a) — measure-first foundation **DONE**: value-edit
   coalescing already existed (`SetInstrumentParam` no-op skip) + dispatch latency
   (`param_latency.go`); ADDED `render_latency.go` (`GetRenderLatencyMetrics`:
   cache hit/miss + per-render duration — the actual storm cost, instrumented at
   the `tryRecipeVoice*` render sites). **Remaining** (deliberate follow-up, needs
   Playwright browser validation): off-thread render, base-pitch render+resample,
   stage-boundary incremental render, param smoothing.
   **Update 2026-07-02 (later session):** off-thread render shipped
   (`synth_render_worker.js`, commit b7664860a) but the pipeline is STILL not
   real-time — the cache-miss defer path drops the intended `when` and eviction
   leaves no playable buffer, so 100% of storm notes play 13–56ms early (dev) /
   up to 104ms late (4× throttle), invisible to the schedule metrics. Measured
   + ranked fixes in
   `2026-07-02-wasm-realtime-synth-latency-bench-and-fix-proposal.md`
   (bench: `make bench-render`; JS metrics: `getRenderLatencyMetrics()`).
5. ✅ **Remaining metrics** (Part 2 #3–5) — **DONE**: `fingerprint/kick_flux.go`
   (`SpectralFluxAvg` — level-invariant half-wave-rectified spectral flux over the
   sustained region; `WarbleDepth` — 2–30 Hz amplitude modulation via a log-domain
   least-squares detrend, so an exponential decay is removed exactly and only the
   oscillation/creak survives), `fingerprint/kick_inharmonicity.go` (`Inharmonicity`
   — amplitude-weighted partial mistuning vs `n·f0` on the body; metallic-vs-pure),
   `fingerprint/kick_attack.go` (`LogAttackTime` + `TemporalCentroidSec`, MPEG-7).
   Folded into `KickWeights`/breakdown (`Flux`/`Inharmonicity`/`Attack`, all 0.75,
   below the dominant terms; identity distance stays 0). sandyrb ref: flux 0.032,
   warble 0.842, inharmonicity 0.178, LAT −4.0 (near-instant attack), temporal
   centroid 0.090 s.
6. **Still open** (future): perceptual-loudness trajectory, onset/transient count,
   formant peaks (Part 2 #6–8); stage-boundary caching + off-thread render (Part 3a
   remainder); wiring the param/render latency counters into `perfStats()`/UI
   (param-latency isn't surfaced there yet either).

Each phase is independently shippable and testable. Nothing here changes the ABI
or the shipped kick sounds; it's additive analysis + internal refactor.
