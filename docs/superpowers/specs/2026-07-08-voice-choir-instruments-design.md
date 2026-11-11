# Voice / Choir Instruments + Synth Stage Modularization — Design

**Date:** 2026-07-08
**Status:** Approved (architecture + instrument set + verification approved in brainstorming session)

## Goal

Add a new **Voice** instrument category with six voice/choir instruments, all rendering
through the existing modular synth pipeline (config-first, one `modularInstrumentDefs`
row each). Extend the pipeline with the machinery voices need — a formant stage, an
ensemble stage, a glottal source — as first-class composable stages any instrument can
use. Simultaneously decompose stages that do too much, both at the user-visible
stage-card level and inside the C DSP.

## Research basis (summary)

- **Source–filter model** is the workhorse: a harmonically rich source with vocal
  spectral tilt (~−12 dB/oct, Rosenberg glottal model) fed through a **parallel
  bandpass formant bank**. Parallel biquads (one per formant) are O(formants)/sample,
  trivially real-time, and morphable — the right choice over FOF grain synthesis for
  this engine (FOF needs a grain scheduler; deferred).
- **Formant tables**: the canonical Csound Appendix D set — F1–F5 frequency (Hz),
  amplitude (dB rel. F1), bandwidth (Hz) for vowels a/e/i/o/u × soprano/alto/tenor/bass
  (countertenor available but unused). Full tables reproduced in § Formant tables.
  Q = Fc/BW per band. F1–F3 are the perceptual minimum; F4/F5 cluster into the
  singer's-formant region (~2.4–3.1 kHz depending on voice type).
- **Choir vs solo** is an ensemble property: N voices with **pitch scatter 5–14 cents**
  (SD between voices), **per-voice vibrato ~5.5 Hz with randomized phase/rate**
  (synchronized vibrato = one singer), flutter ≤20 cents above ~5 Hz, soft attacks
  (100–150 ms), onset pitch scoop (start 30–80 cents flat, glide up over 50–120 ms).
- **Breath**: filtered noise routed through the *same* formant bank; dominant noise =
  whisper.
- Sources: Csound Manual Appendix D (MiscFormants), Rosenberg JASA 1971, Klatt 1980,
  SOS "Formant Synthesis" (Gordon Reid), Ternström choir-scatter perception studies,
  IRCAM CHANT/FOF literature.

## Architecture decision

**Chosen: first-class FORMANT stage** (parallel 5-band bank with baked vowel tables)
over (a) extending `body_model` with vowel entries (no morph/breath, opaque, buries
voice character in an enum) and (b) FORMANT + FOF oscillator (best solo-soprano
ceiling but adds a grain scheduler; can be a later `osc_type` if the soprano
disappoints by ear).

## Stage-card layout (user-visible, Synth tab)

Before:
`VOICE · OSC · FM · PITCH · LFO · BURST · ENVELOPE · FILTER · FILTER ENV · POST`

After:
`VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE · FILTER · FILTER ENV · FORMANT · RESONATOR · POST`

1. **ENSEMBLE (split from OSC)** — the existing unison/ensemble knobs move out of the
   OSC card into their own card, joined by new choir knobs (see param list). Regrouping
   existing knobs is UI metadata only (ParamDef `Group` + `modularStageGroupSection`
   mapping + section order + i18n); the ABI order and DSP are untouched.
2. **FORMANT (new)** — vowel bank stage, runs after FILTER ENV, before RESONATOR.
3. **RESONATOR (surfaced)** — the existing body-resonator bank (`body_model`,
   `body_mix`, `bow_dynamics`) gets its own visible card instead of hiding inside
   another group. FORMANT ≠ RESONATOR: one is a morphable vowel machine, the other is
   fixed instrument-body modes; they compose.

Enable pills follow the existing convention: each new stage gets its own
`*_enabled` param; **new stages default OFF (0.0)** so a zero-filled param block
renders byte-identically to today.

## C-level signal flow (render_modular_p)

```
SOURCE (osc_type dispatch; unison/ensemble path; NEW osc_type 12 = glottal pulse)
  → gen bank (12 slots, unchanged)
  → LFO → burst → amplitude ADSR → biquad filter (+ filter env)
  → NEW: formant bank (5 parallel biquad BPFs + breath-noise input + singer's formant)
  → body resonator (unchanged DSP, now user-visible as RESONATOR)
  → bow dynamics → drive/gain → POST → clamp
```

### FORMANT stage DSP

- 5 parallel RBJ bandpass biquads; per-band center/gain/Q derived from the baked
  vowel tables (Csound Appendix D), indexed by `formant_voice_type` and interpolated
  by `formant_vowel` (continuous 0..4 ≈ a→e→i→o→u; linear interpolation of Hz,
  dB→linear gain, BW between adjacent vowel columns).
- `formant_shift` scales all center frequencies (±1 octave range) — vocal-tract size.
- `formant_breath`: white noise fed through the same 5 biquads (second filter-state
  set), mixed pre-sum. `formant_breath = 1` with `osc_enabled = 0` ⇒ whisper.
- Singer's formant: one extra narrow BPF (Q ≈ 10) at the voice-type-specific
  frequency (soprano 3092 / alto 3000 / tenor 2750 / bass 2400 Hz), gain knob.
- `formant_mix` wet/dry blend (like `body_mix`).
- Vowel morphing must recompute biquad coefficients when the effective vowel value
  changes; recompute at control-rate blocks (e.g. every 64 samples) with coefficient
  smoothing, not per-sample, to protect the RTF budget.

### ENSEMBLE stage DSP (extension of the existing unison path)

Existing: N detuned `wt_osc` copies + drift LFO. Added, all deterministic (no
`rand()` at render time — per-voice values derived from a fixed LCG seeded by voice
index, preserving byte-reproducibility and xplat parity):

- `ens_scatter` — per-voice constant pitch offset, cents SD (0–20; choir ≈ 10).
- `ens_vib_rate` / `ens_vib_depth` — vibrato LFO per voice (Hz; cents).
- `ens_humanize` — 0..1: randomizes per-voice vibrato phase and rate (±15 %) and adds
  slow flutter; 0 = all voices identical (today's behavior).
- Onset scoop stays in the existing pitch-env stage (it already models start-flat
  glide); voice seeds use it rather than a new mechanism.

### Glottal source

`osc_type 12` — Rosenberg pulse rendered into a wavetable per note (closed-form:
raised-cosine opening over N1, cosine closing over N2, open quotient fixed ≈ 0.6),
giving the −12 dB/oct vocal slope. No scheduler. Follows the physical-model
finiteness discipline: `osc_type` Max bumps 11→12 and the new branch must produce
finite output under the extreme-max preset (known SIGSEGV trap otherwise). Choir
patches may instead use tilted saws (saw + existing LP) — per-seed choice.

### C decomposition (byte-identical refactor)

- The OSC stage's ~170-line else-if ladder in `render_modular_p` splits into
  composable primitives declared in `src/c/synth_dsp_primitives.h`:
  `modular_source_basic_osc(...)`, `modular_source_unison(...)`,
  `modular_source_physical_model(...)` (dispatch over osc_type 7–11), plus the new
  `modular_source_glottal(...)` and `modular_formant_bank_process(...)`.
- `modular_gen_slot_kick` (~680 lines, 8 variants) splits into per-variant static
  functions (`gen_kick_base`, `gen_kick_deep`, …) sharing extracted noise/envelope
  primitives; same treatment for `modular_gen_slot_cymbal` (~420 lines, 6 variants)
  and `modular_gen_slot_snare` (3 variants).
- The existing golden byte-identity net (83 goldens incl.
  `TestSynthGoldenByteIdentity`, modular/fm/native golden tests) is the proof: the
  decomposition must land with zero golden drift.

## Param additions (ABI append-only, flat tail of `modular_params`)

All floats, appended in this order; discrete values are integer-valued floats.
Defaults chosen so a zero-filled struct is byte-identical to today (mp_get fallback
discipline: new stage toggles default 0.0 = off).

| Param | Group | Range / enum | Default |
|---|---|---|---|
| `formant_enabled` | formant | 0/1 | 0 |
| `formant_vowel` | formant | 0..4 continuous (a,e,i,o,u) | 0 |
| `formant_voice_type` | formant | enum 0 soprano / 1 alto / 2 tenor / 3 bass | 2 |
| `formant_mix` | formant | 0..1 | 0 |
| `formant_shift` | formant | 0.5..2.0 (freq scale) | 1.0 |
| `formant_breath` | formant | 0..1 | 0 |
| `formant_sing` | formant | 0..1 (singer's-formant gain) | 0 |
| `formant_morph_rate` | formant | 0..8 Hz (LFO vowel drift; 0 = static) | 0 |
| `formant_morph_to` | formant | 0..4 (target vowel for morph LFO) | 0 |
| `ens_scatter` | unison | 0..20 cents | 0 |
| `ens_vib_rate` | unison | 0..8 Hz | 0 |
| `ens_vib_depth` | unison | 0..100 cents | 0 |
| `ens_humanize` | unison | 0..1 | 0 |

Wiring for each: `modular_params` field + `modularParamSchema` + identity map +
bounds in `modular_recipe.go` + `ParamDef` (Label/Group/Min/Max/Default/Enum) +
`make gen-synth-abi` (regenerates `synth_param_abi.gen.js`). New Groups `formant`
(→ new `synthSectionFormant`) and the existing `unison` group remapped to a new
`synthSectionEnsemble`; body params' group remapped to `synthSectionResonator`.
Section additions go through `modularStageGroupSection` + `unifiedSynthSectionOrder`
+ enable-pill wiring + i18n keys (EN, es-419) + `PlainEnglish` glosses for new knob
labels (kid-readable, e.g. `formant_vowel` → "which mouth shape").

## New category

`internal/audio/category.go`:
- `CatVoice CategoryID = "voice"` const + `Category{ID: CatVoice, Label: "Voice"}` in
  `builtinCategoryOrder`, placed between Brass and Lead/Synth.
- `categoryByText` substring rules (most-specific-first): `choir`, `vocal`, `voice`,
  `soprano`, `whisper` → `CatVoice`.
- i18n label for the category (EN + es-419) in the categories menu.

## The six instruments

Each = seed in `instrument_seeds.go` + one row in `modularInstrumentDefs` + regen
(`make gen-modular-instruments` + `go generate ./internal/audio/`). No per-instrument
C code. Loudness via `cmd/measure-loudness` regen — never baked into `gain`; no seed
uses `gain: 0.5` (mutation-test trap).

| ID | Display | Seed sketch |
|---|---|---|
| `choir-ahh` | Ahh Choir | 5-voice unison saw + tilt, ens_scatter ≈ 10¢, ens_vib 5.5 Hz/8¢, humanize high, tenor "a" formants, formant_sing moderate (~2.7 kHz), attack ≈ 100 ms, long release, DurationSec ≈ 2.5 |
| `choir-ooh` | Ooh Choir | Same ensemble; vowel "u→o", darker; formant_breath higher; attack ≈ 150 ms; the sustained pad |
| `voice-soprano` | Soprano | Solo (unison 1), osc_type 12 glottal, soprano "a", vibrato ±60¢ @ 5.5 Hz (pitch-env scoop at onset), formant_sing bright (3.1 kHz) |
| `voice-bass` | Bass Singer | Solo glottal, bass "a/o", vibrato ±30¢, formant_sing 2.4 kHz |
| `voice-pad` | Vowel Pad | Ensemble saws, formant_morph_rate ≈ 0.2 Hz a↔o, wide/lush, deliberately synthy |
| `voice-whisper` | Whisper | osc_enabled 0, formant_breath 1 (noise-only through bank), no vibrato, soft attack; texture layer |

**Acceptance gate: the user's ear.** Render audition WAVs (48 kHz, via
`synth-analyze synth`) for all six; the user auditions before goldens are pinned as
final. Metrics guide, the ear decides.

## Verification

- **Byte identity (existing 42 instruments):** full golden net stays green through
  the C decomposition and ABI appends — zero drift tolerated.
- **New goldens:** pin renders for the six voice instruments after ear approval.
- **Schema shape:** `TestModularParamSchemaShape` / `TestSynthParamSchemaShape` /
  JS ABI drift — updated in the same change as the appends.
- **Extreme-max finiteness:** `TestCompleteness_RecipeRenderPresetsHaveFiniteOutput`
  with osc_type Max = 12 and formant params at Max; enum-count tests
  (modular_saveas/modular_recipe) updated.
- **RTF guard:** `TestP5_RenderRealTimeFactor` ≥ 20× aggregate; formant bank adds
  ~6 biquads (+6 for breath) per voice instrument — control-rate coefficient updates
  keep this cheap. Measure before/after.
- **Gen-source ceiling / clamp trap:** verify no new seed value exceeds a ParamDef
  Max (the silent-clamp gotcha); `genBounds` untouched (no new gen_source).
- **UI:** section-order and enable-pill tests for the three new/changed cards; Synth
  tab alloc budget check; i18n key tests; category menu shows Voice with six entries.
- **Regen set:** `gen-synth-abi`, `gen-modular-instruments`,
  `gen_instrument_ids`, `measure-loudness`, design tokens untouched.
- **Xplat:** `wasm_bridge_smoke` catalogue (six new ids in JS RENDER tables via the
  generated file — no new hand-written JS file, so the deploy manifest is unaffected),
  `xplat_audio_compare` for at least one voice instrument.
- **C rebuild discipline:** `make build/libdrums.a` after every C edit (the Makefile
  now forces the cgo relink).

## Risks / known traps

- osc_type Max bump without a finite extreme-max branch ⇒ SIGSEGV (documented trap).
- Vowel morph recomputing biquad coefficients per-sample ⇒ RTF regression; use
  block-rate updates with smoothing.
- Sustained subtractive voices can read as "church organ / radio hum" (tuning memory
  §3/§4); the humanized ensemble + breath noise are the specific antidotes — if 3+
  tuning rounds still sound electronic, escalate to FOF (`osc_type 13`) rather than
  more param tuning.
- Formant bank on a low bass note: harmonics may fall between formant peaks and thin
  out — the known parallel-bank weakness; mitigate with wider F1 bandwidth at low
  pitch if needed.
- Per-voice randomness must come from a deterministic per-voice-index LCG, never
  `rand()`/time — byte-identity and xplat parity depend on it.

## Formant tables (baked into C as static const data)

Csound Appendix D values. Per voice type × vowel: F1–F5 center (Hz), amplitude
(dB rel. F1), bandwidth (Hz). Q = Fc/BW.

### Soprano
| Vowel | Hz | dB | BW |
|---|---|---|---|
| a | 800, 1150, 2900, 3900, 4950 | 0, −6, −32, −20, −50 | 80, 90, 120, 130, 140 |
| e | 350, 2000, 2800, 3600, 4950 | 0, −20, −15, −40, −56 | 60, 100, 120, 150, 200 |
| i | 270, 2140, 2950, 3900, 4950 | 0, −12, −26, −26, −44 | 60, 90, 100, 120, 120 |
| o | 450, 800, 2830, 3800, 4950 | 0, −11, −22, −22, −50 | 40, 80, 100, 120, 120 |
| u | 325, 700, 2700, 3800, 4950 | 0, −16, −35, −40, −60 | 50, 60, 170, 180, 200 |

### Alto
| Vowel | Hz | dB | BW |
|---|---|---|---|
| a | 800, 1150, 2800, 3500, 4950 | 0, −4, −20, −36, −60 | 80, 90, 120, 130, 140 |
| e | 400, 1600, 2700, 3300, 4950 | 0, −24, −30, −35, −60 | 60, 80, 120, 150, 200 |
| i | 350, 1700, 2700, 3700, 4950 | 0, −20, −30, −36, −60 | 50, 100, 120, 150, 200 |
| o | 450, 800, 2830, 3500, 4950 | 0, −9, −16, −28, −55 | 70, 80, 100, 130, 135 |
| u | 325, 700, 2530, 3500, 4950 | 0, −12, −30, −40, −64 | 50, 60, 170, 180, 200 |

### Tenor
| Vowel | Hz | dB | BW |
|---|---|---|---|
| a | 650, 1080, 2650, 2900, 3250 | 0, −6, −7, −8, −22 | 80, 90, 120, 130, 140 |
| e | 400, 1700, 2600, 3200, 3580 | 0, −14, −12, −14, −20 | 70, 80, 100, 120, 120 |
| i | 290, 1870, 2800, 3250, 3540 | 0, −15, −18, −20, −30 | 40, 90, 100, 120, 120 |
| o | 400, 800, 2600, 2800, 3000 | 0, −10, −12, −12, −26 | 70, 80, 100, 130, 135 |
| u | 350, 600, 2700, 2900, 3300 | 0, −20, −17, −14, −26 | 40, 60, 100, 120, 120 |

### Bass
| Vowel | Hz | dB | BW |
|---|---|---|---|
| a | 600, 1040, 2250, 2450, 2750 | 0, −7, −9, −9, −20 | 60, 70, 110, 120, 130 |
| e | 400, 1620, 2400, 2800, 3100 | 0, −12, −9, −12, −18 | 40, 80, 100, 120, 120 |
| i | 250, 1750, 2600, 3050, 3340 | 0, −30, −16, −22, −28 | 60, 90, 100, 120, 120 |
| o | 400, 750, 2400, 2600, 2900 | 0, −11, −21, −20, −40 | 40, 80, 100, 120, 120 |
| u | 350, 600, 2400, 2675, 2950 | 0, −20, −32, −28, −36 | 40, 80, 100, 120, 120 |

Singer's-formant center per voice type: soprano 3092 Hz, alto ≈ 3000 Hz,
tenor ≈ 2750 Hz, bass ≈ 2400 Hz.

## Out of scope

- FOF grain synthesis (documented as the escalation path for the solo soprano).
- Countertenor voice type (table available; add later if wanted).
- Consonants / text-to-singing of any kind.
- Reverb/space: voice seeds use the existing per-row send/FX machinery, no new
  reverb DSP.
