# Design: one declarative, composable synth pipeline for every instrument

Status: **approved design** (2026-07-04); **implementation complete, P0–P5** (2026-07-05) — see "Status by phase" below. Goal: every instrument is defined
purely by declarative CONFIG flowing through ONE shared synthesis pipeline
(stages enabled/disabled per instrument) — no bespoke per-instrument C renderers,
no hardcoded literals. Byte-for-byte identical output. Expand the pipeline to be
modular, composable, performant, and — for the exposed knobs — understandable.

## Non-negotiables
- **Byte-for-byte identical** render for every current instrument at every step
  (fall back to a tight tolerance + documented reason only where a physical model
  genuinely can't stay bit-exact — none expected, sentinels preserve op order).
- **No audible change** from the refactor itself. Sound changes, if any, are a
  separate later act with the user's ear.
- **One pipeline.** All instruments run `render_modular_p`; they differ only by
  config (which stages are enabled and their params).

## Current state (what already exists)
`render_modular_p` is already a config-driven staged pipeline:
`OSC → GEN×8 → FILTER(+env) → AMP ADSR(+pitch-env/LFO) → UNISON → POST`. Most
instruments already flow through it. The gaps to "purely config" are concentrated:
1. **Physical models** (`osc_type 7–11`: bowed string / brass / reed / flute /
   sax, in `src/c/modular.c`) pass **hardcoded literal args** into
   `render_*` — not config-driven.
2. **Legacy-drum defaults** (`gen_snare_*`, `gen_tom_*`, `gen_cymbal_*`, …) live
   as **C literals** in the `kp_get(param, LITERAL)` sentinel, not as a single
   declarative source of truth.
3. Renderers open-code DSP state machines; only the kick was refactored onto the
   `sp_*` composable primitives (`src/c/synth_dsp_primitives.h`).

## Target architecture

### The one pipeline (unchanged shape, made universal)
```
OSC stage      wavetable | analytic | physical{bowed,brass,reed,flute,sax} | harmonic-kick
GEN stages ×8  partial | noise | KS | analytic | kick | tom | snare | clap | cymbal | FM
FILTER (+env)  type / cutoff / resonance + filter-envelope
AMP ENVELOPE   A/D/S/R + curve + pitch-env + LFO/vibrato
UNISON         voices / detune / drift
POST           drive / gain / body-resonator / reverb
```
Every stage has an enable gate that already exists (`osc_enabled`,
`filtenv_enabled`, `lfo_enabled`, `unison_voices>0`, per-gen `source`). An
instrument is a config that toggles stages and sets params — nothing else.

### Composable primitive library (the "modular" substrate)
Extend the kick's `sp_*` units into a full `static inline` library:
`sp_onepole`, `sp_osc`, `sp_reverb3` (exist) + `sp_biquad`, `sp_adsr`,
`sp_noise`, `sp_delayline`, `sp_pitch_env`, `sp_waveguide_delay`, `sp_softclip`,
`sp_dcblock`, `sp_body_resonator`. Refactor the stage + voice renderers to
COMPOSE these primitives instead of open-coding — the byte-identical extraction
already proven on the kick (`TestVariant7RefactorByteIdentity`). Result: the C is
a primitive library + a fixed pipeline that wires them per config; instruments
compose stages, stages compose primitives.

### Config as the single source of truth
- **Physical-model params → config.** Add a config field per `render_*` arg
  (sax: blow_pos, reed_offset, reed_slope, reflect, breath, loss, vib_depth;
  bowed: bow_pos, slope, force, loss, refl; reed/brass/flute likewise), read via
  `kp_get(param, <exact current literal>)`. Absent config → the literal →
  **byte-identical**.
- **Drum defaults → config.** One declarative instrument table (extending the
  existing `modularInstrumentDefs` / recipe specs) holds every default; the C
  `kp_get` sentinel value equals it (kept in sync by a guard test, as the snare
  already does between `modular_stages.c` and `synth_recipe_wired.go`).
- **Unified instrument definition.** Every instrument resolves through the same
  declarative descriptor → recipe → merged params → `render_modular_p`. The
  legacy `RenderFunc`/baked path is removed once all instruments are on it.

## Knob & stage exposure (curated + understandable)

**Stages** are the existing Synth-tab card vocabulary (OSC · GEN/VOICES · PITCH ·
LFO · FILTER · FILTER-ENV · ENVELOPE · POST). EVERY instrument shows the SAME
cards; a disabled stage is dimmed with an enable pill (knobs stay interactive).
This makes "one pipeline, stages toggled by config" visible and consistent.

**Knobs are tiered** to avoid overwhelming the user:
- **Essential (always shown):** 3–5 character knobs per stage — the ones that
  actually shape the sound (OSC type + 2–3 model knobs; FILTER cutoff/res;
  ADSR; the defining physical-model knobs like sax blow/breath/brightness).
- **Advanced (per-stage expander, collapsed by default):** the fine-detail knobs
  (reed slope, bow position, loss coefficients, exact decay rates). Power without
  clutter. Which tier each param is in is declared on its `ParamDef`.

**Understanding is first-class** (the user's explicit requirement):
1. **Pipeline overview diagram** — a small signal-flow graph showing the enabled
   stages the sound passes through (OSC → FILTER → ENV → OUT), so the user sees
   the shape of their instrument. New.
2. **Live mini-graph per stage** — each stage card renders what it does: OSC
   waveform/spectrum, ADSR curve, FILTER frequency response, LFO/PITCH modulation
   shape, physical-model spectrum. Extends the existing `synth_preview_layer` /
   preview pane to every stage.
3. **Plain-English on every knob** — extend `PlainEnglish` to cover every exposed
   param; show it as an inline subtitle + a tooltip one-liner ("Cutoff → bright or
   dull"). Every Essential knob must have a gloss; Advanced knobs get a tooltip.

## Byte-identity safety net (built first)
`TestSynthGoldenByteIdentity`: render every instrument one-shot at fixed
pitch/seed, hash the buffer, pin the hashes. Runs before/after every refactor
step; any bit change fails loudly. Mirrors `TestVariant7RefactorByteIdentity`.
This guardrail is what makes the whole refactor safe.

## Performance
Config is read ONCE per render into locals (never per-sample); primitives are
`static inline` (identical codegen to the open-coded loops, zero call overhead);
the unified pipeline adds no per-sample branching beyond the stage gates that
already run. Validated against the existing WASM asset-size budget
(`TestWasmAssetSizeBudget`) and render-latency counter (`render_latency.go`):
byte-identical output AND no latency/size regression.

## Phasing (each phase independently shippable + byte-identical)
- **P0 — Golden-identity net. DONE.** `TestSynthGoldenByteIdentity` snapshots +
  hashes all 83 shipped instruments at fixed pitch/seed; gates every later phase.
- **P1 — Primitive library. DONE**, scoped to the shared idioms that actually had
  duplicated call sites. Final inventory in `src/c/synth_dsp_primitives.h`, all
  byte-identical (golden-gated): `sp_onepole_tick` (7 sites), `sp_onepole_mix`
  (15 sites), `sp_biquad` (tick-only, 10 sites — the coefficient math stays
  per-site deliberately, it's not a duplicated idiom), `sp_osc` (11 new sites +
  4 prior), `sp_delay_write` (6 sites across brass/reed/flute/sax), plus the
  prior `sp_dcblock` / `sp_pitch_glide` / `sp_reverb3`. Documented
  non-migrations: the 2 bare-`sin()` bell sites (crash/china) were not migrated
  to `sp_osc` (structurally different, not worth forcing); per-site biquad
  coefficient derivation stays open-coded (not a shared idiom); `sp_adsr` and
  `sp_noise` were **deliberately not added** — no voice duplicates an ADSR block
  (envelopes are single-expression `exp(-rate*t)` decays) and all noise already
  flows through the shared `noise.h` generators, so both would be primitives
  with zero call sites.
- **P2 — Physical-model parameterization. DONE** for the two models with real
  per-instrument variation (sax, bowed-string) — `osc_sax_*` / `osc_bow_*`
  config fields + `kp_get` sentinels, byte-identical, contextual knobs excluded
  from the Advanced-tier cap. Brass/reed/flute stay dormant-by-design (single
  shipped instrument each today; no variation to parameterize yet — the config
  fields exist and default to the prior literals so the door is open).
- **P3 — Config source-of-truth. DONE.** The legacy `RenderFunc`/baked path is
  removed (`ParamRenderFuncs` and `builtinRecipeRenderers` are now empty maps —
  every family renders through the modular binding). All 34 remaining
  seeded-modular instruments are migrated into the declarative
  `modularInstrumentDefs` table (`modular_instruments.go`); the kick family was
  already there. Drum-gen defaults are declaratively tabled
  (`drumGenDefaults`, `drum_gen_defaults.go`) with a fast-path C-drift guard
  (`TestDrumGenDefaultsMatchCSentinels`) and a documented, ratcheted
  pre-existing drift (`knownStaleSnareSpecLits` — snare push literals are stale
  vs C; fixing is a sound-affecting decision left to the user). The base
  `modular` instrument (no seed) stays hand-written by design.
- **P4 — Curated UI exposure + understanding. DONE**, scoped down from the
  original diagram idea. Tiered knobs (Essential/Advanced) ship via `ParamDef`
  tiers + the tier-cap guard (`TestP4_TierClassification`). The "pipeline
  overview" requirement is met by the existing Synth-tab chip strip
  (`drawSynthChipStrip` — stages in audio order, connector wires, per-stage
  enable dots); a separate new diagram was scoped out as redundant. Per-stage
  live mini-graphs and plain-English glosses were pre-existing and extended,
  not newly built in this plan.
- **P5 — Perf + composability audit. DONE.** `TestP5_RenderRealTimeFactor`
  measures an **aggregate render real-time factor of 99× across all 83
  instruments** (worst case: hihat-1 at 34×), enforced by a floor of 20×
  (env-override `BEATMO_RTF_MIN`) — this is the number to cite, not the earlier
  unmeasured "160×" estimate. `TestP5_EveryStageToggleablePerInstrument` checks
  every stage is toggleable per instrument via rule-based
  `stageEffectivelyActive` predicates mirroring the C gates (zero hand-written
  exemptions). Documented pipeline gap, not fixed in this plan: physical-model
  `osc_type` 7–11 structurally bypass the pitch-envelope/LFO-vibrato branches
  in `modular.c` (the model branches run before the generic wavetable branch
  that reads them) — those voices don't get vibrato/pitch-env even though the
  UI shows the stage as available. A related tuning-only gap: `bass-fm`'s
  `fm_op2_level` is seeded at 0, nulling its modulator, so the seed's intended
  "metallic overtones" never sound — a tuning decision, not a pipeline defect.

## Testing
- P0 golden net (byte-identity) gates P1–P3.
- Each new primitive: an isolated behavior test (where the repo's C-test
  convention allows; else via a byte-identity composition test).
- Physical-model params: the "every wired param mutates output" contract
  (`recipe_param_effect_test.go`) + the C-literal↔spec parity guard.
- UI: `crop_*` screenshot scenes for the new stage graphs + a plain-English
  coverage test (every exposed `ParamDef` has a gloss).

## Out of scope (explicitly, for later)
- Runtime DSP node-graph (arbitrary topologies) — deferred; the primitive library
  is designed so it could be layered on later without a rewrite.
- Any deliberate sound change / re-tuning of instruments.
- Sampler / WAV-playback instruments (not synthesized; unaffected).
