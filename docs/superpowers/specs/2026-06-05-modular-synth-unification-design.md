# Modular Synth Unification — Design

**Date:** 2026-06-05
**Status:** Approved (pending spec review)
**Origin:** User request — per-stage enable/disable toggles on every pipeline stage for every synth instrument; remove "not used by this sound"; standardize/unify the Synth tab across all instruments; TDD throughout.

## Decision log (user-confirmed)

1. **Full migration**: all 25 legacy instruments (20 drums + 5 FM presets) migrate to the modular pipeline. No UI-only or partially-cosmetic toggles.
2. **Fidelity bar**: expose every hardcoded legacy-C capability as modular *config*, and prove **byte-for-byte equality** between the legacy render and its modular-preset equivalent in automated tests.
3. **Legacy fate**: delete legacy C render paths (`drums.c` renderers, `fmsynth.c` presets) once end-to-end audio identity is proven. Capture oracle fixtures first so the bytes stay locked after deletion.
4. **Landing**: incremental, family-by-family phases. Claude does not `git add` or commit — the user controls git.
5. **Endgame**: only discrete, fully parameterized modular stages remain. No baked multi-step C recipes; no instrument diverges from the unified pipeline. Transplanted family-shaped code is scaffolding that must dissolve into stages (Approach C: transplant verbatim → lock with byte goldens → consolidate under the goldens).

## Goals

- Every synth instrument is pure config (a preset) over one engine (`modular.c`).
- Every pipeline stage has an `*_enabled` toggle, functional on every instrument.
- The Synth tab shows one standardized section layout for all instruments; disabled stages show dimmed knobs instead of the "not used by this sound" hint.
- All existing recipe IDs, project JSON, and export semantics survive unchanged.

## Non-goals

- Sampler/WAV instruments (they keep the existing "does not use the synth" banner).
- Changing how voice pre-render/cache works (next-trigger param model stays — see [[feedback_next_trigger_param_deferral]]).
- Multi-preset dropdown / preview-pane work deferred from the original modular project.

---

## 1. Target architecture — the unified stage pipeline

One fixed pipeline shape for every instrument. Per-instrument differences are param values only.

```
GEN BANK (12 slots)
  each slot: source = off | osc(sine/saw/square/tri) | noise-tap | FM | Karplus-Strong
             freq mode = ratio-of-fundamental | absolute Hz; detune; phase config
             gain; dual-rate amp decay (fast rate + tail rate, weights)
             branch filter = none | 1-pole LP | 1-pole HP | RBJ LP/HP/BP (cutoff, Q)
MODULATORS (global stages, per-slot participation/scale)
  PITCH ENV : exponential sweep (amount, rate); per-slot linear drift rate
  LFO       : amp wobble (rate, per-slot rate-step, depth)
  BURST     : up to 4 bursts (offset, amp) × 4, sharpness
MIX         : sum in slot-index order
AMP ENV     : ADSR (existing modular) | legacy exp-decay mode;
              attack boost (amount, rate); global fade (linear|exp, amount)
FILTER      : RBJ LP/HP/BP + 1-pole LP/HP modes (cutoff, Q)
DRIVE       : algorithm = softsat | tanh-normalized | hard tanh×4; amount;
              bit-reduction (levels)
POST        : pitch resample, decay-mult env, body, tone, brightness, gain
```

Sizing is inventory-derived: 12 slots = ride's 10 partials + bell + noise branch (the maximum); kick needs 6, snare 6, crash 10. The full legacy DSP capability inventory (per-renderer signal flow, stage interleaving, hardcoded-vs-parameterized split, state) was produced from `src/c/drums.c`, `src/c/fmsynth.c`, `src/c/modular.c`, `src/c/noise.c`, `src/c/adsr.c` and is the authority for stage requirements.

**Noise bus**: noise is generated once per sample into a shared bus; slots with `source = noise-tap` consume the bus through their own branch filter. This preserves byte-identity where legacy code feeds a single white-noise draw into two branches (e.g., kick thud + beater click). If a legacy renderer draws more than one noise value per sample, additional buses reproduce the documented draw order.

**Stage enables**: every stage (and each gen slot) carries an `*_enabled` param, `Enum: [Off, On]`, default chosen per preset to match legacy behavior. `enabled=0` is an **exact bypass** — byte-identical to the stage not existing. This is what makes toggles functional and honest on every instrument.

### Param / ABI model

- Flat float params with slot-indexed names (`gen3_ratio`, `gen3_gain`, …) **appended** to the `modular_params` block, exactly like the existing bypass pills were: `mp_get(params, idx, legacy_default)` fallback means absent params elide to legacy literals, so existing modular presets stay byte-identical with zero overlay changes.
- Rough budget: ~120 slot params + ~40 globals ≈ 160 fields. Mechanical via the existing `gen-synth-abi` codegen (`src/go/cmd/gen-synth-abi` → `src/js/synth_param_abi.gen.js`); RecipeDoc defaults, effective-delta export, and the single `updateInstrumentParams` IPC seam are unchanged.
- `ParamDef.Group` maps params to UI sections; hidden params (e.g., seeds) use `Group: "hidden"` per existing convention.

### Noise determinism & round-robin variants

- `noise.c` carries `noise_ma`, a bit-identical transplant of miniaudio's ma_noise white stream (Lehmer LCG; seed 0 → 4321 per `ma_noise_config_init`), proven against real `ma_noise` by `TestNoiseMaParity`. Family transplants consume `noise_ma` instead of `ma_noise`.
- Legacy renders are fully deterministic: `drums.c` re-seeds `ma_noise` with 0 on every call, so consecutive renders — and all 3 `RoundRobinVariants` entries — are byte-identical today (pinned by `TestLegacyRendersAreDeterministicAcrossCalls`). The oracle is therefore ONE render per (instrument × param case); no variant stream gymnastics.
- Per-hit "randomization" (snare LFO-rate jitter, shaker burst offsets, hihat phases) comes from the same deterministic stream and is identical on every hit; it is captured by the oracle fixtures as-is. The modular engine's existing `noise_seed` param remains the seam for true per-variant variation later.

---

## 2. Migration mechanics — byte-identity under refactor

Per family, a five-step loop:

1. **Capture oracle goldens.** Render every legacy instrument with current code: one render per (instrument × param case) — defaults + per-param min/max sweeps — at 48 kHz (renders are deterministic; all round-robin variants are byte-identical, see § Noise determinism). Commit PCM hash fixtures. These outlive the legacy code.
2. **Transplant.** Move the family's render internals into `modular.c`, replacing hardcoded literals with `mp_get(params, idx, legacy_literal)`. NULL-params render is bit-identical by construction (proven NaN-sentinel pattern from the full-knob-parameterization project).
3. **Consolidate into discrete stages under the goldens.** Refactor toward the Section-1 stage functions; byte-equality reruns after each step. Family-shaped scaffolding must be fully dissolved before the family's phase is done.
4. **Extract the preset.** Recipe becomes pure config: RecipeDoc defaults = values reproducing the legacy literals.
5. **Delete the legacy path** — `render_*_p`, CGo wrapper, `ParamRenderFuncs` rebind to the modular renderer. Goldens keep passing against the committed fixtures.

### Byte-identity rules (normative)

- **Op order per value is sacred.** Each sample's dependency chain executes identical float ops in identical order. Mixing sums in slot-index order. Buffer-at-a-time stage processing is permitted only for independent branches.
- **Shared noise draws** go through the noise bus (see above).
- **Precision preserved.** Legacy `double` phase accumulators and double math stay `double` inside stage functions. No float-narrowing during consolidation.
- **Default elision.** Absent/NaN params return the exact legacy literal.
- **Disabled stage = exact bypass.**
- Any consolidation step that cannot preserve bytes is wrong and must be redesigned — the bar does not relax to epsilon.

---

## 3. Presets, import, UI, testing, phases

### Recipes & import migration

- All 27 recipe IDs survive unchanged (`drum-kick`, `fm-bell`, `synth-modular`, …). Old project JSON loads by ID with no migration of identity.
- Generic post knobs (`pitch/decay/tone/drive/body/brightness`) keep their names — they are the POST stage params.
- Legacy family params (`snare_tone2_freq`, `kick_env0_rate`, `fm_op1_ratio`, …) get a **per-family rename table** → slot/stage param names, applied at import (precedent: `MigrateGenType`). Export writes the new names. Each rename table ships **in the same phase as its family's migration** so saved projects never break mid-sequence.
- Byte-identity is verified at **non-default** knob settings via the param-sweep goldens, so the mapping is proven, not assumed.

### UI unification

- All instruments share one schema ⇒ `synthSectionOrderForSchema` collapses to a single order. Sections map 1:1 to stages: **GEN · PITCH · LFO · BURST · ENVELOPE · FILTER · DRIVE · POST** (labels go through `PlainEnglish` glosses; final kid-readable wording at implementation).
- Every section title row gets the existing `FXTogglePill` via the established `_enabled` suffix mechanism (`isSynthEnableParam`, `sectionEnablePillRect`, `toggleSynthStage`).
- `synthCollapsedSectionHint()` ("not used by this sound") is **deleted**; no section is ever empty — a disabled stage shows its knobs at their values, dimmed by the existing scrim (`synth_panel_zone.go:1391-1400`). **Dimmed but interactive** (current modular behavior): users may pre-tweak a disabled stage; the pill stays bright.
- **GEN slot picker**: a pill row (slots 1–12; active slots visually distinguished) selects which slot's knobs fill the GEN section grid. Slots with `source = off` are inactive but selectable (enable by choosing a source).
- Toggling a stage writes `0.0/1.0` via `SetInstrumentParam` and auditions via `auditionInstrumentAfterDrag` (existing behavior).
- WAV-sample instruments keep the existing banner; unchanged.

### Testing strategy (TDD order)

1. **Oracle fixtures** (Phase 0): legacy renders hashed and committed before any engine change.
2. **Stage unit tests**: each discrete stage function — bypass-identity (`enabled=0` ⇒ untouched buffer), param response, state reset.
3. **Per-family byte-equality**: modular preset render == legacy oracle, 3 variants × defaults + sweeps. These are written *before* the family transplant (red → green).
4. **xplat parity**: extend the existing xplat param cases to migrated presets (WASM vs native bit-exact).
5. **UI fast-path tests** (`-tags test`): every synth instrument shows an enable pill on every section; the hint string is gone repo-wide (grep-style test); disabled-stage scrim renders; toggle round-trips persist through overlay + export delta; slot-picker selection drives the knob grid.
6. **Discipline/invariants updated**: `synth_recipe_discipline_test.go`, `synth_recipe_invariants_test.go`, ABI invariants, `native_golden_test.go` re-pointed at modular renders with the *same expected bytes*; `modular_golden_test.go` unchanged throughout Phase 1 (proves appended params don't disturb existing modular sound).

Validation commands per phase: fast Go suite (`-tags test -modfile=go.test.mod ./internal/audio/... ./internal/ui/...`), `make coverage-c` for new C stage code, `xplat_audio_compare.browser.test.js` + `wasm_bridge_smoke.browser.test.js` for bridge/parity.

### Phases

| # | Phase | Done means |
|---|---|---|
| 0 | Oracle capture + byte-compare harness + ma-LCG noise transplant | Fixtures committed; legacy renders proven deterministic across calls |
| 1 | Engine scaffold: gen-bank/modulator/stage params, ABI growth, stage functions with bypass-identity tests | Existing modular goldens byte-unchanged |
| 2 | Bass family (bass_guitar, sub_bass) — incl. Karplus-Strong slot source | Family byte-equality green; legacy bass renderers deleted; rename table + import test in-phase (rule applies to phases 2–7) |
| 3 | Kick family (kick, deep, punchy, lofi, tight) — incl. bit-reduction, drive variants | same |
| 4 | Tom family (tom, high, low) — pitch sweep start→end | same |
| 5 | Snare family (snare, rimshot, sidestick, clap) — noise bands, bursts, per-slot drift | same |
| 6 | Cymbal family (hihat, open_hihat, cowbell, shaker, ride, crash) — partial banks, LFO | same |
| 7 | FM presets (bass, bell, lead, epiano, pluck) — FM slot source absorbs `fmsynth.c` presets | same; `fmsynth.c` preset table deleted |
| 8 | UI unification: single section order, pills everywhere, hint removed, slot picker | UI suite green; original user ask fully delivered |

Each phase lands independently green. Claude stages/commits nothing.

### Phase-2 hand-off notes (from the Phase-1 integration review)

- **Karplus-Strong does NOT fit the rectangular noise bus.** `render_bass_guitar` draws noise in a control-flow-dependent pattern (seed + delay-line init draws + conditional per-sample "stretch" draw + attack-window draws) — not N draws/sample. The KS slot source (`gen<k>_source == 4`) must own its delay line and consume `noise_ma` through a KS-specific path; do not force it onto `noise_draws`/`gen<k>_noise_offset`.
- **sub_bass needs appended slot fields**: per-slot exponential pitch envelope (per-sample freq modulation — the Phase-1 slot oscillator is fixed-freq), phase-init π/2 (covered by `gen<k>_phase_mode=1`), a 2nd-harmonic companion slot, per-slot attack boost, and per-slot tanh drive. All land as NEW appended `gen_<f>[12]` columns (the by-field array layout + the C-struct-parsing ABI drift test make this mechanical).
- **Byte-identity guards are native-only** (`!test && !js`). The fast stubbed suite proves schema shape only — every family phase's "done means" MUST include the xvfb native run of the full golden gate.

## Post-implementation amendments (2026-06-06 gap audit)

The migration shipped (all 25 instruments byte-identical on the modular
engine, legacy C deleted, native gates green). The gap audit reconciled this
document with what was actually built; the following amendments are the
now-normative record.

### A1. Endgame renegotiation — family-shaped sources are the endgame shape

Decision-log #5 ("transplanted family-shaped code is scaffolding that must
dissolve into stages") proved incompatible with §2's own byte-identity rules:
the legacy per-sample float-op order fuses operations that discrete orthogonal
stages would have to reorder. The Phase-2 plan (decision 5) renegotiated the
endgame to **discrete parameterized SOURCES + shared global stages**: gen
sources 3–10 are one-source-per-family voices (variant-branched verbatim
transplants with every curated knob exposed via the kp_get NaN sentinel);
sources 1–2 (osc, noise-tap) are orthogonal primitives. The committed oracle
fixtures make any future consolidation safe; none is planned or required.

### A2. Final stage set & UI

- Shipped section order (every synth instrument):
  **VOICE · OSC · FM · PITCH · LFO · BURST · ENVELOPE · FILTER · POST**.
- §1's PITCH ENV / LFO / BURST modulators were initially cut; the gap audit
  closed them as **Phase-8C global stages** (appended `modular_params` tail:
  `pitchenv_*`, `lfo_*`, `burst_*`). Unlike the pre-existing toggles they
  default **DISABLED** (new DSP ⇒ off is the byte-identity polarity); their
  knob defaults are audible so flipping a pill is never a silent no-op (the
  defaults≠identity carve-out in modular_recipe_test.go documents this).
  PITCH ENV scopes to the OSC-stage oscillator + the FM core — context-
  dependent exactly like fm_enabled (it does not reach inside family voices;
  byte-identity forbids editing their transplanted loops). LFO and BURST are
  post-mix multiplicative stages and shape every voice.
- **DRIVE ⊂ POST**: the modular drive saturator's toggle (`drive_enabled`)
  lives on the POST card; a separate DRIVE section would duplicate the generic
  post `drive` knob's name. §1's "DRIVE" row is satisfied there.
- **GEN slot picker dropped**: the 12-slot bank is engine plumbing
  (Group:hidden); stages are the user surface. Per-slot enables are
  `gen<k>_source = 0`, not a `*_enabled` param.
- **VOICE has no pill** — the voice IS the instrument; "disabling" it is
  silence, not a stage bypass.

### A3. Rename tables dropped (deliberate)

§3's per-family import rename table was not built. Migrated recipes keep their
legacy-named ParamDefs everywhere (UI, project JSON, prefs, oracle fixture
keys); the per-family binding (`<family>RecipeToModular`) is the permanent
translation layer, applied only at the native render seam and the Go→JS push
seam (`ModularPushParams`). Nothing churns on disk; old projects load
unchanged.

### A4. Cross-platform parity is correlation-grade, NOT bit-exact

The native byte-identity contract (legacy bytes == modular render) holds
exactly and is gated by the oracle fixtures. **Browser-vs-native is not
byte-equal by design**: the push seam spells kp_get fallbacks as float32
literals while the native binding passes NaN sentinels that C widens to full
double literals — an inherent ~1 float32-ULP seam (see the header of
family_push_binding_parity_test.go; in-process guard tolerance 1e-5 scaled).
The xplat gate (xplat_audio_compare.browser.test.js param cases) asserts
correlation ≥ 0.999, peak/RMS Δ ≤ 0.01, and a hard per-sample maxDiff bound
calibrated above the ULP-amplification floor. Any earlier "xplat bit-exact"
phrasing is superseded by this paragraph.

### A5. Alloc-discipline harness fix

The per-tab Draw alloc harness now re-Layouts for each active tab; the
TabSynth/TabScope/TabMeters budgets were re-baselined against honest renders
(they previously measured an empty ~79-alloc draw and the caps were fiction).
See perTabAllocBudget in eq_panel_draw_alloc_discipline_test.go for the
measured numbers.

### Risks / known hazards

- **Float op-order traps** during consolidation — mitigated by per-step golden reruns; redesign (not epsilon) on failure.
- **Multi-draw noise samples**: if a renderer draws several noise values per sample, bus layout must replicate draw order (verify per family during transplant, against the inventory).
- **ABI growth (~160 fields)**: heap block size and JS write loop scale linearly; `gen-synth-abi` regeneration + `wasm_bridge_smoke` catalogue keep the bridge honest.
- **UI knob count**: mitigated by the slot picker; per-tab alloc budget (`eq_panel_draw_alloc_discipline_test.go`) may need a cited bump.
- **Long phase tail**: family phases are independent; pausing after any phase leaves main green with a partially-migrated (but fully working) instrument set.
