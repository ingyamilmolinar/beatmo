# Configurable Kick Synthesis Stage + DnB Kick — Design

**Date:** 2026-06-27
**Branch:** add-core-node-types
**Status:** Approved (design), pending implementation

## Goal

Turn kick-drum synthesis into a **first-class, user-configurable stage** of the
modular synth, then ship a `dnb-kick` instrument tuned to the reference
recording `36010__sandyrb__dnb-kick-003.wav`. This expands the flexibility,
modularity, and robustness of the config-based synth rather than hard-coding a
bespoke `render_kick_dnb`.

## Reference characterization (measured)

`36010__sandyrb__dnb-kick-003.wav` (mono, 44.1 kHz, 16-bit, 0.48 s):

- Fundamental ≈ **90 Hz**, strong.
- Pitch drop ~92 Hz → ~60 Hz over the first ~30 ms.
- Fast punch: −20 dB by ~19 ms after peak; sub tail to −40 dB by ~98 ms.
- Band energy: **51 % < 80 Hz, 46 % 80–160 Hz**, 2.3 % 160–400 Hz, ~0 above.
- Essentially **no beater click** (1.4 % HF in first 5 ms). Dark, sub-heavy.

This is a deep, dark DnB sub-kick: fundamental-dominated with a short pitch
drop, fast initial decay, ~100 ms sub tail, no click.

## What already exists (engine survey)

- **Dedicated kick voice** `modular_gen_slot_kick` (`src/c/modular_stages.c`,
  `gen_source==5`): verbatim transplant of the legacy kick renderers — harmonic
  stack (f0+2f0+3f0+4f0), per-sample pitch-drop, beater click, noise thud, 5
  variants (base/deep/punchy/lofi/tight). Its knobs already exist in the C
  struct (`gen_kick_*[k]`) and the Go schema, but **hidden** (`SynthHiddenGroup`),
  read via `kp_get(value, variant_literal)` where `NaN → variant_literal`.
- **User-facing general stages** on the osc voice: `pitchenv_*`, `burst_*`, amp
  ADSR, filter, filter-env, drive (Phase-8C). The osc path is single-oscillator
  (no native harmonic stack), so it is *not* the faithful kick path.

**The gap is exposure + a few baked constants**, not new DSP.

## Design

### 1. Top-level KICK stage (mirrors PITCH/BURST)

Surface the kick voice as a **top-level modular stage** reading top-level
`kick_*` params, the same pattern as the Phase-8C `pitchenv`/`burst` stages
(rather than leaking gen-slot indexing into the UI). When `kick_enabled ≥ 0.5`,
`render_modular_p` synthesizes the kick body via the existing
`modular_gen_slot_kick` DSP (refactored to read top-level params); when off,
render is **byte-identical** to today.

Existing drum-recipe kicks (`kick`, `kick-deep`, `kick-1`, `kick-2`,
`kick-tight`) are **untouched** — they keep rendering through their
`drum-kick*` recipes. Only modular kick instruments (initially `dnb-kick`) use
the new stage.

### 2. KICK stage params

Promoted from hidden (already in C struct + schema):

| Param | Meaning |
|---|---|
| `kick_variant` | base / deep / punchy / lofi / tight (Enum) |
| `kick_h2` `kick_h3` `kick_h4` | harmonic gains (body knock) |
| `kick_env0` `kick_env1` | body decay rates (tail length) |
| `kick_pe_amt` `kick_pe_rate` | pitch-drop depth + speed |
| `kick_click` | beater click amount |
| `kick_noise` | noise thud amount |

`kick_env0` max bumped (20 → ~40) so short DnB tails are reachable.

New fidelity knobs (append to C struct **flat tail**, `kp_get(NaN →
per-variant-literal)` so every existing variant stays byte-identical):

| Param | Replaces baked constant | Why |
|---|---|---|
| `kick_attack` | `0.3·e^(-40t)` front boost | punch |
| `kick_fade` | `e^(-4·tNorm)` global fade | tail tightness |
| `kick_sat` | `0.55×` pre-saturation gain | weight / drive |

`kick_click_tone` (click LP/HP alpha) added **only if** A/B shows the click
needs it — out of scope by default.

`kick_enabled` gate, default **off** (byte-identity); audible numeric defaults
when flipped on (same polarity rule as Phase-8C: enable off-by-default, numeric
defaults differ from identity and are gated by the enable).

### 3. The `dnb-kick` instrument

- Instrument id `dnb-kick`, display "DnB Kick".
- New recipe `synth-modular-kick-dnb` (modular category, `renderModular`).
- Seed: `osc_enabled:0`, `kick_enabled:1`, `voice_freq_hz≈90`, base variant
  `deep`, tuned pitch drop, strong fundamental, fast tail (high `kick_env0` /
  tight `kick_fade`), `kick_click≈0`, low `kick_noise` — converged against the
  reference via A/B.
- Wired through every touchpoint (per
  `reference_add_builtin_modular_instrument`): `instrument_seeds.go`,
  `synth_recipe.go` (descriptor + binding), `synth_recipe_dispatch.go` (NOT in
  `melodicRecipeIDs` — a kick is not pitch-re-rendered),
  `engine_instruments.go`, `config.go`, `audio.js` (×3: render_modular map,
  RENDER_INFO, MELODIC set — kick excluded from MELODIC), regen
  `instrument_ids_gen.go`.
- GOTCHA: never seed `gain:0.5` (`TestPhase2Native` mutates gain→0.5).

### 4. UI — KICK stage card

- New `synthSectionKick`, inserted after OSC in the stage order
  (`VOICE · OSC · KICK · FM · PITCH · …`).
- Enable pill + sliders; `kick_variant` rendered as an Enum selector.
- Touchpoints: section enum, `sectionForParam` / `enableParamForSection`
  mapping, a `crop_*` scene + `scene_crop_test`, alloc-budget entry in
  `eq_panel_draw_alloc_discipline_test.go`.

### 5. Engine / ABI mechanics

- New scalar params append at the **flat struct tail** (ABI-append rule); regen
  `synth_param_abi.gen.js` via `gen-synth-abi`.
- Add new symbols to Makefile `EXPORTED_FUNCTIONS` if any new exported C entry
  is introduced (the kick DSP is already exported via `render_modular_p`; new
  params ride the existing struct, so likely no new export — verify).
- Force-rebuild `build/libdrums.a` (`rm build/modular.o build/libdrums.a`) after
  any `src/c` edit; rebuild WASM (`make wasm`) for the browser path.
- Additive params + identity defaults ⇒ all existing modular/drum renders stay
  byte-identical.

### 6. Testing & fidelity gate

- **Schema identity**: `ModularParamSchemaIdentity()` must list the new params
  at their identity defaults; `ModularSynthParamDefs` order mirrors
  `modularParamSchema` and the ABI index map.
- **Discipline**: `c_header_parity_test`, `synth_recipe_discipline_test`,
  `synth_recipe_invariants_test`, `synth_recipe_completeness_test`,
  `recipe_doc_test`, modular ABI/enum-count tests.
- **Fidelity (new Go test, native CGo lane `!test && !js`)**: render `dnb-kick`
  and assert fundamental ≈ 90 Hz (±~1 semitone), short tail (energy −20 dB
  within ~120 ms), and band energy concentrated < 160 Hz — matching the
  measured reference. Lives alongside `synthmatch`/`cmd/synth-analyze`.
- **Manual A/B loop**: `synth-analyze synth -inst dnb-kick … -out` → `ref`
  both WAVs → compare; iterate the seed. **User ear is the acceptance gate** —
  render a WAV for audition before declaring done.

### 7. Reference handling & commits

- The reference WAV is freesound/sandyrb (non-PD): stays **local, never
  committed**.
- Per standing preference: **no `git add` / commit** — all work left
  uncommitted for review.

## Out of scope

- Migrating the existing drum-recipe kicks to the modular path (parity risk, no
  user benefit now).
- `kick_click_tone` unless A/B demands it.
- Per-variant UI presets beyond the `kick_variant` selector.

## Implementation order (high level)

1. C: refactor `modular_gen_slot_kick` to read top-level `kick_*` params; append
   `kick_enabled` + new fidelity params at struct tail with `kp_get` fallbacks.
2. Go schema: promote kick params to a `kick` group; add new params; update
   `ModularParamSchemaIdentity` + ABI; regen `synth_param_abi.gen.js`.
3. Wire `dnb-kick` instrument (8 touchpoints + regen ids).
4. UI: `synthSectionKick` stage card + mappings + crop scene + alloc budget.
5. Tune the `dnb-kick` seed to the reference (A/B loop).
6. Tests green (discipline + fidelity); rebuild WASM; bridge smoke.
