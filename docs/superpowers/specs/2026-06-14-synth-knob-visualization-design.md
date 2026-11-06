# Synth Tab — Knob-Effect Visualization Design

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** Approved design, pending implementation plan

## Goal

Make it immediately obvious to a user — kid-readable, no DSP background — how each
synth knob changes the sound. Two coordinated visualizations inside the existing
Synth tab:

- **A — Live final-output mirror:** the right pane shows the *actual note you will
  hear* (real rendered PCM), so every edit has a visible whole-sound consequence.
- **B — Per-knob concept pictures:** each knob carries a small live drawing of the
  *idea it controls* (the abstract DSP role of that parameter), so the user learns
  what each individual knob does.

Both were chosen together; the per-knob pictures teach each knob in isolation, the
mirror shows the combined result.

## Decisions (locked during brainstorming)

| Decision | Choice |
|---|---|
| Core direction | A (final-output mirror) **and** B (per-knob concept pictures) |
| What a mini-visual draws | The **concept** the knob controls (abstract DSP picture), not a small copy of the output |
| Mirror refresh | **Snap on knob release**, debounced. Mini concept-curves update live during the drag (cheap math). |
| Platform scope | **Both, desktop-first.** Mobile gets the same mini-visuals; the mirror may degrade to on-release-only / shorter window to protect perf. |
| "Whole chain" reference strip | **Dropped for v1.** Per-knob pictures + the mirror carry the teaching. Revisit later if missed. |
| Ghost (before/after) | Yes — both the mirror and the per-knob picture show a faint "before you grabbed it" trace, captured on press, fading after release. |

## Existing infrastructure this builds on

(From the synth-pipeline exploration — file refs are starting points, not exhaustive.)

- **Synth tab render:** `internal/ui/synth_panel_zone.go` (`drawSynthTab`, pipeline
  chip strip `drawSynthChipStrip`, detail pane `drawSynthDetailPane`, knob grid via
  `ControlGrid`).
- **Right-half preview pane:** `internal/ui/synth_preview_layer.go`
  (`drawSynthPreviewPane`, `drawSynthOscPlot` + `synthOscSample`,
  `drawSynthADSRPlot` + `drawSynthADSRSegment`, `drawSynthFilterPlot`). These already
  draw concept pictures for OSC / ADSR / Filter and are the seed for the per-knob
  renderers.
- **Waveform helpers:** `internal/ui/render_waveform.go` (`drawWaveTrace`,
  `overdrawWaveClips`).
- **Knob + param model:** `internal/ui/knob.go` (`Knob`, `KnobScale`),
  `internal/audio/synth_recipe.go` (`ParamDef` with `Group`,
  `SetInstrumentParam`, `GetInstrumentParams`, `MergeRecipeDefaults`).
- **Filter response (audio layer):** `audio.ModularFilterResponse`,
  `audio.GetChannelEQResponse`.
- **Offline render harness (test-only today):** `internal/audio/offline_render.go`
  (block-by-block mixer drive + `StopOutputCapture`) — promoted to a public preview
  render path.
- **Async pools:** `internal/async` (`DefaultRegistry().Get`, the
  `recording.lifecycle` recipe at `recording_lifecycle.go:23-36`).
- **Scroll:** the shared `MenuScroll`/scroll component used by overflow lists.

## Components

### 1. Concept-renderer registry (per-knob mini-visuals)

A table mapping a `ParamDef.Group` (`osc`, `filter`, `env`, `fm`, motion
[`pitch`/`lfo`/`burst`], `post`) to a **pure-math** draw function
`func(dst, rect, instID, params, ghostParams)`:

| Group | Picture | Source |
|---|---|---|
| `osc` | one cycle of the raw wave; detune = drifting copies | `synthOscSample` (exists) |
| `filter` | frequency-response tone curve | `audio.ModularFilterResponse` (exists) |
| `env` | ADSR loudness-over-time ramp | ADSR reader from `drawSynthADSRPlot` (exists) |
| `fm` | harmonic bars (more depth = more partials) | **new**, cheap math |
| motion | pitch-drop / LFO wobble / burst repeats over time | **new**, cheap math |
| `post` | drive waveshaping + gain amplitude, with ghost | **new**, cheap math |

- Drawn beneath each knob's dial+caption in the detail pane.
- Pure math, no audio render → updates **live during drag**, allocation-budgeted.
- Resolution is by `Group`, so a stage with several knobs of the same group can
  share the same picture kind (each instance reads the live params).

### 2. Final-output mirror (right pane)

- **New public audio API:** `audio.RenderInstrumentPreview(instID string, durationMs int) []float64`
  — promotes the `offline_render.go` block-mixer infra to render one note of the
  instrument's resolved recipe (`MergeRecipeDefaults` + `GetInstrumentParams`) and
  return PCM. Runs **off the realtime audio path**.
- Drawn with `drawWaveTrace` + `overdrawWaveClips` (free clip/headroom warning); a
  ghost of the previous render sits behind it.
- **Refresh policy:** snap-on-release + debounce; result **cached** by
  `(recipeID, paramsHash)`. The render executes on a dedicated `synth.preview`
  async pool (1 worker / small bounded queue, following the `recording.lifecycle`
  recipe with a private-pool fallback); the worker posts the PCM back and the UI
  draws it on the next frame. No blocking of `Update()`/`Draw()`.
- **Mobile degrade:** on-release-only with a shorter window if perf headroom
  requires; same mini-visuals as desktop.

### 3. Ghost state machine

- On knob **press**: snapshot the affected param value(s) → `ghostParams`.
- During **drag**: per-knob picture draws ghost (faint) under the live curve; mirror
  keeps showing the last completed render as its ghost.
- On **release**: trigger the debounced mirror re-render; fade the ghost out over a
  short animation, then clear.

### 4. Layout / scroll

- Each knob cell grows vertically to fit its mini-visual scope.
- Detail pane shows a few knob rows; overflow scrolls via the existing scroll
  component. The right-pane mirror stays fixed (does not scroll).

## Data flow

```
knob drag ─▶ Knob.Value ─▶ propagateSynthSliderValue ─▶ audio.SetInstrumentParam
   │                                                          │
   │ (live, cheap)                                            │ (on release, debounced)
   ▼                                                          ▼
concept-renderer registry                          synth.preview async pool
reads live params, draws under knob                RenderInstrumentPreview(instID)
                                                            │
                                                            ▼
                                                   PCM cached by (recipe,hash)
                                                            │
                                                            ▼
                                          right-pane mirror via drawWaveTrace (+ghost)
```

## Testing (Go fast-path; honours the JS Test Charter — logic lives in Go)

- **Concept-renderer registry:** table-driven test that every param `Group` resolves
  to a renderer; discipline guard (in the spirit of `synth_recipe_discipline_test.go`)
  fails if a new group is added without a renderer.
- **Per-knob reactivity (pixels):** `drawRect`-interceptor tests (mirroring
  `audio_panel_render_pixels_test.go`) proving each mini-visual draws non-empty data
  and that changing the bound param changes the drawn pixels.
- **Mirror render:** Go test for `RenderInstrumentPreview` (non-empty, deterministic
  for fixed params; differs when a param changes); cache/debounce/coalesce logic test.
- **Ghost state machine:** press→drag→release snapshot/fade/clear transitions.
- **Layout/scroll:** detail pane lays out the new taller knob cells and scrolls
  overflow; right-pane mirror keeps its rect.
- **Alloc budget:** add/justify an entry for the new per-frame draws on the Synth tab.
- **One browser test:** real-WASM mirror render produces a visible trace (per the
  charter — the WASM↔audio boundary, not re-testing DSP logic).

## Non-goals (YAGNI)

- The "staged signal flow" direction (Approach C) and the always-on whole-chain strip.
- Live-during-drag re-rendering of the real PCM mirror.
- Per-node parameter override visualization (the synth tab edits instrument-level
  params only).
- New DSP — visuals reuse existing render/response functions plus cheap concept math.

## Risks

- **Audio starvation on WASM** (the Chain-tab precedent): mitigated by snap-on-release,
  debounce, caching, and off-thread rendering on a bounded pool. Verify with the same
  discipline used for the Chain trace cache.
- **Mirror render cost** for long/complex recipes: cap `durationMs`; cache aggressively.
- **Layout churn** in the detail pane: reuse `ControlGrid` + existing scroll; keep the
  draw path dispatch-only per the DrumView layer discipline.

## Repo conventions

Per project memory, **do not stage or commit** in this repo — all changes (including
this design doc) are left in the working tree for the user to commit.
