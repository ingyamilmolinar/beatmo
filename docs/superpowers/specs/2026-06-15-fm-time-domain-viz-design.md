# FM Time-Domain Param Visualization

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design — approved, pending spec review
**Builds on:** `2026-06-15-synth-focus-graph-redesign-design.md` (the focus-graph redesign + phase-2 gap closure)

## Problem

The synth focus graph now visibly responds to ~1187 of the selectable knobs, with **73 exempted** because the steady-timbre, cycle-normalized FM preview (`conceptFM`) cannot show them. The exemptions are FM params whose effect is **time-domain** (it evolves over the note) or otherwise unshown:

| Exempted param(s) | Why unshown today | Category |
|---|---|---|
| `fm_op1_decay`, `fm_op2_decay`, `fm_op3_decay` (bespoke: `fm_dec1..4`) | operator amplitude fades over the note; the steady cycle has no time axis | **time-domain** |
| `fm_pitch_env_amount`, `fm_pitch_env_decay` (bespoke: `fm_pe_amt`, `fm_pe_decay`) | FM pitch sweeps over the note; normalized away by per-cycle auto-zoom | **time-domain** |
| `fm_base_freq` (bespoke: `fm_base`) | base pitch normalized away by auto-zoom | **pitch** (already solvable) |
| `fm_op1_level` | carrier level = output amplitude; `conceptFM` normalizes amplitude | **amplitude** |
| `fm-epiano / fm_op3_ratio`, `fm_op3_depth` | op3 has zero modulation depth in that patch — genuinely inert | **inert** (no fix; faithful) |

`RenderInstrumentPreview` does **not** model FM operator decay or FM pitch-env, so no rendered-audio view could show them without DSP work either.

## Goal

Make every FM time-domain knob visibly move the focus graph, **without** DSP changes, by drawing a **schematic time-curve** of the param's meaning — consistent with how `conceptEnvelope` (ADSR) and `conceptMotion` (LFO / pitch-env / burst) already draw schematic time pictures. Fold in the trivial pitch/amplitude routings. Drive the strict sweep exemption set down to the single genuinely-inert case.

## Decisions (locked with user)

- **Schematic, not rendered.** Draw the curve from the param values (pure math over a normalized note window), not from rendered FM audio. Faithful to what the knob *does*; consistent with the existing time pictures; no `preview_render.go` changes. (User chose this over teaching the preview synth to model FM time evolution.)
- The schematic shows the **selected** operator's curve prominently (the renderer receives the selected `ParamDef`).

## Architecture

All work is in `src/go/internal/ui/`. No `internal/audio/` changes.

### 1. New renderer `conceptFMEnvelope` (`synth_concept_viz.go`)

A `conceptRenderer` (`func(dst, rect, instID, def, ghost)`) that, based on `def.Name`, draws one of:

- **Operator amplitude decay** — for `fm_opN_decay` / `fm_decN`: a curve `a(t) = exp(-t / tau)` over a normalized window `t ∈ [0, 1]`, where `tau` derives from the operator's decay param value (read from merged params; longer decay ⇒ slower fall). Filled curve to baseline (reuse `conceptDrawFilledCurve`). The picture changes as the selected decay knob changes. Optionally draw the other operators' decay curves faint for context (YAGNI — only if cheap; the selected one is required).
- **FM pitch envelope** — for `fm_pitch_env_amount` / `fm_pitch_env_decay` / `fm_pe_amt` / `fm_pe_decay`: a pitch-vs-time curve `p(t) = amt · exp(-t / dec)` (bipolar, fill to centerline), the same shape `motionCurveFromParams` uses for the modular pitch-env. Reuse that math where practical.
- **Ghost** — when `ghost` carries a different value for `def.Name`, draw the faint "before" curve (same pattern as the other concept renderers).

Helpers: a small `fmDecayTau(merged, opParamName) float64` and `fmPitchEnvParams(merged) (amt, dec float64)` that read both the modular (`fm_opN_decay`, `fm_pitch_env_*`) and bespoke (`fm_decN`, `fm_pe_*`) naming. Read ranges from the recipe's `ParamDef` (`audio.RecipeRegistrations()`) so the normalized window maps the param's real Min/Max.

### 2. Per-knob routing (`synth_focus_graph.go`)

Extend `synthFocusRendererForKnob(def)`:
- Add an `isFMTimeDomainKnob(name) bool` predicate (`fm_opN_decay`, `fm_decN`, `fm_pitch_env_amount`, `fm_pitch_env_decay`, `fm_pe_amt`, `fm_pe_decay`) → returns `conceptFMEnvelope`.
- Add `fm_base_freq` and `fm_base` to `isSynthPitchKnob` → routes to `conceptPitchWave` (existing).
- Route `fm_op1_level` to `conceptFocusLevelWave` **iff** op1 is the carrier for the recipe's algorithm. The implementer must verify carrier identity (read `fm_algorithm` handling); if it's algorithm-dependent and not reliably the carrier, leave `fm_op1_level` exempt with a precise reason rather than show a misleading amplitude.

Ordering in `synthFocusRendererForKnob`: pitch knobs first, then FM-time-domain, then group default — so a name that matches one category is unambiguous.

### 3. Strict sweep gate (`synth_focus_sweep_test.go`)

Remove the exemption entries that are now closed: `fm_opN_decay`, `fm_dec*`, `fm_pitch_env_*`, `fm_pe_*`, `fm_base_freq`/`fm_base`, and `fm_op1_level` (if routed). The bidirectional gate then **requires** these to respond (and would fail if any are still flat). Keep only the genuinely-inert `fm-epiano / fm_op3_*` per-recipe exemption. Target exemption count: the single inert case (plus any algorithm-ambiguous `fm_op1_level` the implementer keeps, with reason). Update the summary log line.

## Testing (TDD)

- **Renderer responsiveness** (`synth_fm_envelope_test.go`, `//go:build test`): for a modular FM instrument and at least one bespoke FM recipe (fm-bell/fm-lead/fm-pluck), assert `conceptFMEnvelope` produces a different fingerprint (rect count + summed coords via `collectFilledRects`) at Min vs Max for each of: an operator decay knob, `fm_pitch_env_amount`, `fm_pitch_env_decay` (using the recipe's real param names). Write failing-first.
- **Routing**: assert `synthFocusRendererForKnob` returns `conceptFMEnvelope` for the FM-time names, `conceptPitchWave` for `fm_base_freq`, and the correct renderer for `fm_op1_level`.
- **Ghost**: a ghost with a different value adds ink (mirror the existing `*_GhostAddsInk` tests).
- **Strict sweep**: `TestFocusGraph_RespondsToEveryKnob` stays green with the reduced exemption set; both directions still enforced (unresponsive non-exempt fails; responsive exempt fails). Report the new responsive/exempt counts.
- **Scene + visual**: add `crop_synth_focus_fm_decay` (FM stage, an operator decay knob) and `crop_synth_focus_fm_penv` (FM pitch-env knob) with `Setup` + `MobileSetup`; add guard rows to `TestSynthFocusSceneSelectsIntendedKnob`. Capture desktop + mobile PNGs and visually confirm the schematic decay/pitch curves render in the focus band (and the mobile always-on band shows them).

## Out of scope / YAGNI
- No `preview_render.go` / DSP changes (the schematic is pure-math).
- No spectrogram/waterfall view.
- The `fm-epiano / fm_op3_*` inert case is intentionally left exempt (faithful — the operator does nothing there).

## Notes
- Per repo policy the implementer does **not** `git add`/commit; changes stay in the working tree. (Overrides the brainstorming "commit the design doc" step.)
- Concurrent i18n/Settings WIP intermittently flap-breaks the `-tags test` build in shared files (`synth_panel_zone.go`, `synth_preview_layer.go`); verify own work with `-run` filters and `go vet`, never by a clean full-suite run. Tell dispatched agents to do the work, verify with `-run`, then STOP — do not spawn background log-watch loops.
