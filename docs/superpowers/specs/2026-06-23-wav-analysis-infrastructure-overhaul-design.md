# WAV Analysis Infrastructure Overhaul (Phase A)

**Date:** 2026-06-23
**Status:** Approved (design); implementation pending
**Scope:** `src/go/internal/audio/fingerprint`, `src/go/internal/audio` (PCM decode), `src/go/cmd/synth-analyze`, `src/go/internal/audio/synthmatch` (edge fixes only)

This is **Phase A** of a two-phase effort. Phase A makes the WAV-analysis
infrastructure comprehensive, robust, and bug-free, validated against the real
violin reference. **Phase B** (separate spec) tunes the synthesized violin to the
D5 reference, with deep engine work allowed and WAV-render checkpoints — it
consumes the foundation built here.

The end goal of the whole effort: **fully and fundamentally understand every part
of the reference WAV (`356181__mtg__violin-d5.wav`) and replicate it.**

---

## Motivation — what's broken and what's missing

### Confirmed bugs (each corrupts violin analysis today)

1. **F0 octave error — sub-harmonic /3.** `fingerprint/pitch.go` `DetectF0` has an
   octave guard that only tests `{f/2, f, 2f}`. On the real D5 file YIN locks to a
   sub-harmonic and the guard cannot rescue it: it reports **195 Hz = 591/3**. The
   stored `tmp/fp/violin_ref.json` confirms it — `Partials[1]=10.5`, `Partials[2]=12.2`
   (H2/H3 ten times the claimed "fundamental"), i.e. the true F0 is the 3rd reported
   partial. Direct numpy ground-truth: **F0 = 591.3 Hz** (D5 ≈ 587, A=442 tuning).
   This single bug poisons every harmonic-relative metric (partials, even/odd,
   inharmonicity, tristimulus, per-partial slopes).

2. **24-bit references unreadable on the fast path.** `sample_pcm_teststub.go` (the
   `-tags test` pure-Go decoder) accepts only 16-bit PCM. The reference is
   **24-bit / 48 kHz / mono**, so no `-tags test` code can decode the real file —
   which is exactly where the golden/characterization tests need to run.

3. **Vibrato mis-detected.** `fingerprint/temporal.go` reported `VibratoDetected:false`
   on a violin that does vibrato (~4 Hz, ±~5 cents). Causes: it runs on the
   octave-wrong F0 trajectory; uses an outlier-sensitive min/max depth; has no
   regularity/jitter or amplitude-modulation measure; no onset-delay measure.

4. **Optimizer duration mismatch** (`synthmatch`). The reference is auto-segmented to
   1.3 s but the candidate is rendered at the instrument's *default* duration, so
   envelope metrics (attack/decay/release) are compared across different note lengths.

5. **Smaller edges:** `synthmatch/seedpatch.go` value-replace regex breaks on a `/`
   inside an inline comment; `seedVarName` mishandles empty hyphen parts; no
   silent/zero-energy guard in `FromWave`/`Distance` (risk of NaN); the fixed
   `±1.5 Hz` partial-search window is too tight at high harmonics under vibrato,
   dropping real upper partials toward zero.

### DSP metric gaps (what "comprehensive" requires)

The current fingerprint is a strong **static** timbre snapshot but misses the
**time-varying morphology** that defines a bowed string. Ground truth measured from
the D5 reference:

- **Brightness ↔ loudness coupling:** `corr(spectral_centroid, RMS) = +0.73`. As the
  bow gets louder the tone brightens; in the loud middle of the note **H2 rises to
  equal the fundamental (+0.9 dB)** with upper harmonics filling in, then everything
  collapses in the decay tail. This coupling is the single biggest "alive vs. organ"
  signal — **and it is not computed today.** The current synth does the *opposite*
  (centroid slope −38 Hz/s, static).
- **Per-harmonic amplitude trajectories over the whole note** — current code has only
  `PartialDecaySlopes[6]` over the sustain region.
- **Robust vibrato** — rate, extent, regularity/jitter, AM depth, onset delay.
- **Spectral-envelope / formants** — the ~2.3–2.4 kHz "bridge hill" is a defining
  violin marker (confirmed in the reference at 2355–2391 Hz). Not computed.
- **Harmonic/residual noise profile** — bow-noise spectral shape and level *over time*,
  beyond the scalar `NoiseRatio`. Reference spectral flatness ≈ 0.094 (tonal w/ noise).

---

## Design

### Section 1 — Bug fixes (foundation; do first)

- **`pitch.go` octave robustness.** Replace the 3-candidate guard with a **Harmonic
  Product Spectrum (HPS)** + harmonic-coverage score evaluated over
  `{f/3, f/2, f, 2f, 3f}`, choosing the candidate whose harmonic comb explains the
  most spectral energy; tie-break toward the YIN estimate. Regression test seeded with
  the D5 file asserts F0 ≈ 591 Hz (±5 Hz).
- **`sample_pcm_teststub.go`.** Add 24-bit signed and 32-bit IEEE-float PCM decode
  (keep 16-bit). Mono/stereo as today. Unblocks real-reference golden tests.
- **Partial picking.** Widen the per-harmonic search window proportionally to harmonic
  index (track the local spectral peak within ±(k·tol)), so vibrato no longer drops
  high partials toward zero.
- **Guards.** Silent/zero-energy input yields a well-defined empty fingerprint;
  `Distance` returns finite values (no NaN) for empty/zero fingerprints.
- **`synthmatch` edges** (cheap, fixed now): robust value-replace in `seedpatch.go`;
  `seedVarName` empty-part guard; optimizer renders the candidate at the reference's
  segmented duration.

### Section 2 — New DSP metrics (new files in `internal/audio/fingerprint/`)

Each new file owns one concern, with its own struct and a `compute*` entry point
called from `FromWave`:

- **`vibrato.go` → `VibratoFingerprint`** — rate (autocorr/FFT of the F0 track),
  extent (percentile-based cents, outlier-robust), regularity/jitter (cycle-to-cycle
  rate variance), AM depth (amplitude tremolo synchronous with FM), onset delay
  (time from note start to vibrato establishment). Supersedes the inline vibrato code
  in `temporal.go` (which is removed/redirected).
- **`harmonics_temporal.go` → `HarmonicTrajectory`** — per-harmonic amplitude track over
  the whole note: attack-rise rate, peak time, sustain level, decay rate per harmonic
  (k = 1..N), plus time-varying even/odd ratio.
- **`formants.go` → `SpectralEnvelope`** — real-cepstrum-liftered spectral envelope
  (optional LPC cross-check) → formant peak frequencies + bandwidths + an explicit
  bridge-hill marker (strongest broad peak in 1.5–4 kHz).
- **`coupling.go` → `DynamicCoupling`** — `corr(centroid, RMS)`, brightness-modulation
  depth and rate, FM↔AM correlation. The aliveness axis.
- **`noise_residual.go` → `NoiseProfile`** — harmonic/residual split → residual
  (bow-noise) spectral shape and level over time.

### Section 3 — Integration

- Extend `Fingerprint` (additive fields / sub-struct pointers) and `FromWave` to
  populate the new metrics. JSON stays backward-compatible (new fields, `omitempty`
  where pointer-typed).
- Extend `Distance`/`Score` with new weighted terms (vibrato, coupling, formant,
  harmonic-trajectory). Weights documented inline with rationale. Preserve identity
  (`Distance(fp,fp).Total==0`) and symmetry tests.
- Extend `cmd/synth-analyze` human-readable + JSON output to print every new metric, so
  Phase B's WAV checkpoints are legible.
- Preserve native sample rate in the `synth-analyze ref` path (do not silently resample
  48 k → 44.1 k before analyzing a reference).

### Section 4 — Tests / validation

- **Synthetic unit test per metric:** synthesize a signal with known
  vibrato/formant/coupling/noise and assert recovery within tolerance (e.g. inject
  5.5 Hz ±20 c vibrato → assert rate/extent; inject a 2.4 kHz resonance → assert formant).
- **Real-reference characterization (golden) test** against `356181__mtg__violin-d5.wav`
  with measured ground truth as *tolerances* (guardrail, not tripwire):
  F0 ≈ 591 Hz, vibrato ≈ 4 Hz / ±5 c, bridge hill ≈ 2.3–2.4 kHz,
  `corr(centroid,RMS) ≈ +0.73`, spectral flatness ≈ 0.094, attack (10–90%) ≈ 0.5 s.
- Maintain existing discipline: `goleak` where applicable, gofmt-clean, `-tags test`
  fast path green.

---

## Out of scope (→ Phase B)

- Violin synth parameter tuning toward the reference.
- Synth-engine (C/Go) changes — e.g. making brightness track loudness, time-varying
  harmonic morphology, bow model.
- WAV-render audition checkpoints.
- Extending the `synthmatch` objective/optimizer to *use* the new metrics for tuning
  (Phase A makes the metrics exist and be correct; Phase B wires them into the loss
  and tunes).

## Ground-truth reference characterization (measured 2026-06-23)

`356181__mtg__violin-d5.wav` — 24-bit / 48 kHz / mono / 2.675 s, MTG/UPF Good-Sounds,
A=442:

- F0 = 591.3 Hz (D5)
- Harmonic amps (norm, H1..H8 sustain): 1.00, 0.75, 0.22, 0.58, 0.37, 0.03, 0.03, 0.005
- even/odd energy ratio ≈ 0.76
- Vibrato ≈ 4 Hz, extent ±~5 cents (shallow — a fairly straight Good-Sounds note)
- Spectral centroid (sustain) ≈ 2339 Hz; `corr(centroid,RMS) ≈ +0.73`
- Per-harmonic dynamics: H2 rises to ≈ H1 in the loud middle, upper harmonics fill in,
  all collapse in the tail
- Attack 10–90% ≈ 0.5 s (slow bow swell); spectral flatness ≈ 0.094
- Formant / body peaks: ~557 Hz, **bridge hill 2355–2391 Hz**, 3551 Hz, 4184 Hz
