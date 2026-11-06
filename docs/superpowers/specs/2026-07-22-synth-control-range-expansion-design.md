# Synth Control Range Expansion — Design

**Date**: 2026-07-22
**Status**: Approved (pending spec review)

## Problem

Several user-facing synth controls max out at conservative values that don't
reach the full range of practical, musically-useful settings. Examples: the
oscillator octave switch only spans ±2, amp attack tops out at 2s (too short for
pad swells), FM operator ratios cap at 8, transposition is limited to ±2
octaves, and the LFO can't reach audio rate. Users hit the end of a knob before
the sound stops changing usefully.

## Goal

Widen the `Min`/`Max` bounds of the affected controls to encompass the full
range of practical musical values, at the **"full practical musical range"**
level (not engine-absolute limits that produce broken output).

## Scope

**In scope:**
- User-facing **modular synth** controls (`ModularSynthParamDefs()` in
  `internal/audio/modular_recipe.go`).
- **Send FX** delay time (requires enlarging the C delay buffer).

**Out of scope (explicitly excluded):**
- Drum-family bespoke recipe knobs (`synth_recipe_wired.go`) — bass/kick/snare/
  tom/cymbal. Left at current ranges.
- Hidden gen-bank / ABI-completeness bounds (`SynthHiddenGroup`) — not shown in
  the UI; changing them risks the `gen_source` clamp-bug and ABI invariants.
- Controls already at full/engine-limited range: `filter_cutoff` (20–20kHz),
  `filter_resonance` (0.5–16), `unison_voices` (**hard C cap of 7** — fixed
  side-voice array in `modular.c`), and all normalized 0–1 / enum controls.

## Key architectural facts (verified)

1. **Single source of truth**: `ParamDef.Min/Max` in `ModularSynthParamDefs()`
   is *both* the UI slider bound *and* the write-time clamp
   (`sanitizeParamValue` in `setparam.go`). Widening the range widens what
   reaches the C engine — it is a real sonic change, not cosmetic.
2. **The C engine reads the affected knobs straight through** (attack/decay/
   release seconds, `pow(2, semis/12)` for pitch, FM ratio, `sin(2π·rate·t)` for
   LFO) with **no hidden upper cap**, so wider maxes reach new territory.
   Verified: `modular.c:697-700` (ADSR), `modular.c:528` (pitch), `fmsynth.c`
   (ratio), `modular.c:526` (LFO).
3. **Defaults are NOT changed.** Only `Min`/`Max` move. Every existing preset,
   saved instrument, and golden renders byte-identically. This preserves the
   "defaults must equal `ModularParamSchemaIdentity()`" invariant.
4. **No test pins the ranges.** `modular_recipe_test.go` only asserts
   `Min ≤ Default ≤ Max` (widening can never violate this). No schema hash /
   golden serializes `Min`/`Max`. `validateParamDef` (registration) also only
   enforces `Min ≤ Default ≤ Max`.
5. **Hard engine caps that must NOT be exceeded**: `unison_voices` = 7
   (`modular.c:558`); send `delay.time_ms` = `maxSendDelayMs` (2000ms C buffer
   allocation in `send_effects.go:122`).

## DSP-grounded per-control evaluation

Every candidate control was evaluated against how the C engine actually consumes
it (`modular.c`, `modular_stages.c`, `fmsynth.c`, `adsr.h`, `effects.c`). Two
findings reshaped the change set from the first draft:

1. **Every value is bounded — no UI setting can produce NaN/Inf.** Output passes
   a per-voice hard clamp at ±1.5 (`modular.c:1191`), FM/drive pass a `tanh`
   softsat, and the filter biquad clamps cutoff to `[20Hz, 0.99·Nyquist]`. The
   only failure modes are *aliasing noise*, *wasted knob throw*, or *degenerate
   silence* — never a crash.
2. **Voices are pre-rendered one-shots** of `samples = Beats × (60/bpm) × sr`.
   Max `Beats` in the shipped defs is 3.0, so the longest voice buffer is ~3s at
   60 BPM (~4.5s at 40 BPM). Any time-based envelope beyond that buffer is
   truncated and inaudible — so envelope maxima are bounded by the buffer, not by
   taste.

Engine sample rate: 44.1kHz desktop / 48kHz browser (Nyquist ~22–24kHz).

### A. Controls to widen — `internal/audio/modular_recipe.go`

Each row edits one `ParamDef` literal's `Min`/`Max` (Default unchanged):

| Stage | Control | Current | New | DSP rationale |
|---|---|---|---|---|
| OSC | `osc_octave` | ±2 | **±3** (Step 1) | Pure `2^oct`; +3=1760Hz base, −3=27.5Hz. Beyond hits the 1Hz floor / heavy aliasing. |
| OSC | `osc_detune` | ±100¢ | **±200¢** | `2^(cents/1200)`, fully stable; user-chosen fine-tune widening. |
| FM | `fm_op1..4_ratio` | 0.5–8 | **0.25–16** | Min 0.25 matches legacy `fmFamilyParamDefs`; single-sine table (no per-note band-limit) aliases above ~16 at high play pitches. |
| FM | `fm_op1..4_depth` | 0–8 | **0–12** | Modulation index; Carson bandwidth overruns Nyquist into aliased noise past ~12 (softsat still bounds it). |
| ENV | `amp_attack` | 0–2s | **0–4s** | Attack must complete within the voice buffer (~3–4.5s for the long melodic/pad voices); 4s enables slow swells there. |
| POST | `pitch` | ±24st | **±48st** | `2^(st/12)`, no Nyquist ceiling; sine clean to ~+87st, bright waves alias gradually above ~+13st (creative, softsat-bounded). |
| PITCH ENV | `pitchenv_amt` | ±24st | **±48st** | Same pitch law; ±48 keeps the onset transient below the Nyquist-garbage point (~±60st). |
| BURST | `burst_sharp` | 1–200 | **1–500** | Decay rate τ=1/sharp; at 500 τ=2ms (tight click with body); past ~1000 the click is sub-ms/inaudible. |
| BURST | `burst1..4_off` | 0–0.25s | **0–0.5s** | Hit fires only if offset < voice length; 0.5s allows wider flam spread on the longer voices. |
| FILTER ENV | `filtenv_amt` | 0–4oct | **0–6oct** | `cutoff·2^oct`, biquad hard-clamps at Nyquist; 6 lets a very low base cutoff still reach the top. |
| FILTER ENV | `filtenv_decay` | 0.01–2s | **0.01–3s** | Purely perceptual (exp time-constant); longer filter sweeps. |

### B. Send FX delay — `internal/audio/setparam.go` + `send_effects.go`

- `send_effects.go`: bump `const maxSendDelayMs = 2000` → **4000** (`:122`). This
  enlarges the C `delayBuf` malloc (`sr·ms/1000` floats) at init — the only
  memory impact: ~+384 KB (768 KB total @48k, one global mono buffer).
- `setparam.go`: `sendFXParamDefs["delay.time_ms"]` Max `2000` → **4000** (`:72`).
- The two MUST move together — `ConfigureSendDelay` clamps `timeMs > maxSendDelayMs`
  (`send_effects.go:337`); the ParamDef comment already says it must track the
  const. No C change needed (`effects.c` is buffer-size-agnostic).

### C. Controls evaluated and deliberately LEFT UNCHANGED

These were candidates but the DSP shows they are already at/beyond their
practical ceiling, or hard-capped by the engine. Widening would add wasted throw
or degrade the sound; **narrowing** the already-too-wide ones is rejected because
it would clamp existing showcase seeds (e.g. seeds use `unison_detune` up to 40,
`ens_vib_depth` up to 30) and break byte-identical rendering.

| Control | Current | Why unchanged |
|---|---|---|
| `amp_decay` | 0–4s | Already ≥ max voice buffer (~4.5s); longer is truncated. |
| `amp_release` | 0–4s | Engine hard-caps release effect to `samples/2` (`modular.c:889`); 4s is already beyond useful. |
| `gain` | 0–1.5 | 1.5 IS the per-voice hard-clamp ceiling (`modular.c:1191`); higher = gross square-wave clipping, not loudness. |
| `drive` | 0–1 | Self-normalizing `tanh(x·k)/tanh(k)`; >1 adds no level, only squareness. |
| `body_mix` | 0–1.5 | 1.5 is the hard ceiling; above it high-Q body modes ring into the ±1.5 clamp. |
| `bow_dynamics` | 0–1 | 1.0 already gives a 0–2× loudness swing that hits the clamp on loud notes. |
| `filter_cutoff` | 20–20000 | Biquad clamps to `[20, 0.99·Nyquist≈21829]`; already at the audible ceiling. |
| `filter_resonance` | 0.5–16 | RBJ biquad can't self-oscillate; 16 (~+24dB peak) is the stability/musical ceiling. |
| `lfo_rate` | 0.1–40Hz | 40 is already at/above the "true LFO" range; higher becomes audio-rate FM/AM and the cutoff-LFO aliases above ~375Hz (updates every 64 samples). |
| `lfo_delay` | 0–3s | Beyond the voice length the LFO never fully engages; 3s already spans it. |
| `pitchenv_decay` | 0.01–2s | Used for fast pitch "boom" sweeps; must resolve before note-end. 2s already exceeds the useful range. |
| `filtenv_attack` | 0–2s | Bloom must finish within the voice; 2s already exceeds most voices. |
| `unison_detune` | 0–50¢ | Already at/beyond the "fat unison" edge (~25–30¢); >35¢ reads as out-of-tune. |
| `unison_drift_rate` | 0–8Hz | >2–3Hz is audible warble, not slow drift. |
| `unison_drift_depth` | 0–30¢ | >15¢ stacked with detune pushes past a semitone. |
| `ens_scatter` | 0–20¢ | >12¢ permanently sours the cluster. |
| `ens_vib_rate` | 0–8Hz | Natural vibrato is 4–7Hz; already covers it. |
| `ens_vib_depth` | 0–100¢ | Real vibrato is ±20–50¢; 100¢ is already a wailing extreme. |
| `ens_jitter` | 0–1 | Effective depth = jitter·35¢; 1.0 already sounds gritty. |
| `formant_morph_rate` | 0–8Hz | >4–6Hz blurs vowels into a wah; already covers the musical range. |
| `formant_shift` | 0.5–2 | No DSP break; 0.5–2 already spans giant→chipmunk head sizes. |
| `fm_op*_level` | 0–1 | Natural amplitude range; softsat makes >1 pointless for carriers. |
| `amp_sustain` | 0–1 | Normalized level, correct. |
| `delay.feedback` | 0–1 | Pure recirculation with no limiter; ≥1 = runaway self-oscillation. |
| `reverb.room/damping/wet` | 0–1 | Already full-range; `room`=comb feedback, metastable at exactly 1.0. |

## Risks & mitigations

- **Aliasing at extreme pitch/FM settings**: the single-sine FM table and
  48-harmonic wavetables alias at the top of the widened `pitch`/`fm_op_ratio`/
  `fm_op_depth` ranges. This is a gradual, softsat-bounded creative tradeoff
  (harsh/metallic, never a crash), accepted per the "full practical range"
  decision. Most use stays well inside the clean zone.
- **Wasted throw on short voices**: `amp_attack` up to 4s is fully usable only on
  the long (Beats 2–3) voices; on a 1s voice, 2–4s produces a truncated partial
  swell. Accepted — a shared schema can't be per-instrument, and denying the long
  voices their swell is the worse outcome.
- **Widening a knob the C engine secretly clamps**: every affected knob was
  verified read-through with no hidden upper cap (§A rationale column). The hard
  caps (`unison_voices`=7, per-voice ±1.5, biquad Nyquist, delay buffer) are all
  respected.
- **Defaults stay in range**: every widened bound still contains its existing
  default, so `validateParamDef` and the defaults-match invariant hold, and every
  existing preset/seed/golden renders byte-identically.

## Verification plan

1. **Fast Go tests**: `modular_recipe_test.go` (defaults-in-range),
   `synth_recipe_invariants_test.go`, `synth_recipe_discipline_test.go`,
   the full `internal/audio` + `internal/ui` fast suite.
2. **Round-trip / clamp**: confirm `SetInstrumentParam` accepts the new maxima
   (a value at the new Max is stored, not clamped) via a targeted test or a
   `js_exports` sanity check.
3. **Send delay**: audio test that a 4000ms delay time is accepted and the
   buffer is sized for it (`send_effects` test).
4. **Visual (MANDATORY per CLAUDE.md)**: render the Synth tab
   (`-scene synth_*`) and confirm the widened knobs display/behave correctly —
   labels legible, no layout break, knob sweeps the new range. Desktop +
   `MOBILE=1`.
5. **Parity**: `xplat_audio_compare` / `webaudio_mixer_eq_parity` — the WASM and
   native paths must agree at the new extremes (both read the same ParamDef
   ranges via the shared schema).

## Non-goals

- No new controls, no default changes, no re-tiering of essential/advanced.
- No changes to curve mapping (log/linear) — existing curves still apply across
  the wider range.
