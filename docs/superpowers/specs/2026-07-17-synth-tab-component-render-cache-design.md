# Synth-tab component render cache (TraceSprite adoption) — design

Date: 2026-07-17
Branch: add-core-node-types
Status: design (pending implementation plan)

## Problem

The `synth_stage_toggle_ui_bench` attribution (`bench-results/synth-toggle-attribution.md`)
proved the audio crackle/lag when interacting with the Synth tab is **dominated
by Synth-tab per-frame draw cost on the single WASM thread**, not the DSP
re-render. The Synth tab is the only heavy audio tab that re-rasterizes all of
its content every frame: its *data* is hash-gated (`synthParamsHash`, the
merged-params RO cache), but its *pixels* are redrawn column-by-column each
frame — ~15 chip-strip pills, the detail-pane ParamCards (multi-layer keycaps +
per-frame caption formatting), and three per-column traces (full-note,
up-close mirror, focus-graph concept curve). EQ, Spectrum, Wave, and Chain
already cache their heavy surfaces; the Synth tab never adopted the caching
infra that was built for exactly this.

## Approach (chosen: surgical TraceSprite adoption on the Synth tab, per-card sprites)

Reuse the existing generic `TraceSprite` (`internal/ui/trace_sprite.go`) verbatim
— no changes to the primitive. Wrap each expensive Synth component's draw in a
`TraceSprite` keyed on the state that actually changes its pixels, so an idle
Synth tab re-renders nothing and a knob drag re-renders at the throttled 30 Hz
(the EQ-curve precedent). This mirrors the EQ/Spectrum/Wave adopters and is the
explicitly-deferred "synth right-pane" item on the TraceSprite design doc's own
rollout checklist.

`TraceSprite` API (unchanged):
`Draw(dst, rect, key any, render func(img *ebiten.Image, r image.Rectangle))` —
re-renders via `render` (at rect-relative `(0,0)` origin) only on key change or
size change, else one blit; `Invalidate()`; `Release()` (immediate
`Deallocate`); 33 ms drag throttle for churny keys; `RENDER_SAFE` bypass.

### The five adopters

Owned by `synthPanel` (mirroring how `EQPanelZone` owns `curveSprite` /
`spectrumCurveSprite`).

| # | Component | Draw site | Sprite | Key | minInterval |
|---|---|---|---|---|---|
| 1 | ParamCards (detail pane) | `drawSynthDetailPane`→`drawParamCard` (`synth_panel_zone.go:2587`, `param_card.go:64`) | `map[string]*TraceSprite` keyed by paramID (per-card) | `{paramID, quantizedValue, cardState, size}` | 33 ms |
| 2 | Chip strip | `drawSynthChipStrip` (`synth_panel_zone.go:2346`) | one `TraceSprite` (band) | `{selectedSection, enableStateHash, thumbHash, size}` | 0 |
| 3 | Focus-graph concept curve | `drawSynthFocusGraph`→`conceptDrawFilledCurve` (`synth_focus_graph.go:188`, `synth_concept_viz.go:159`) | one `TraceSprite` | `{conceptKey (synthParamsHash+selectedKnob), size}` | 33 ms |
| 4 | Full-note "Your sound" trace | `drawSynthFullNote` (`synth_full_note.go:81`) | one `TraceSprite` | `{fullRenderHash, ghost/live flag, size}` | 33 ms |
| 5 | Up-close mirror trace | `drawSynthMirror` (`synth_preview_layer.go:132`) | one `TraceSprite` | `{synthParamsHash, size}` | 33 ms |

Rationale:
- **Per-card sprites (chosen).** Keyed on `paramID` (not slot index), so scroll /
  relayout doesn't thrash and a single-knob edit re-renders only that one card.
  The per-frame caption formatting (`synthParamDisplayName` + `formatParamValue`)
  moves *inside* the render callback → it runs only on re-render, killing the
  alloc the `TabSynth` budget's `FOLLOW-UP` note flags.
- **Chip strip = one sprite** (not per-chip): it's a single horizontal band that
  changes only on selection / enable-toggle / thumb-hash; whole-band is simpler
  and re-renders rarely. `minInterval=0` (discrete changes, no drag churn).
- **The three traces** animate only *during* a knob drag (hash churns per frame);
  at rest they're static. The 33 ms throttle gives 30 Hz during drags and 0
  re-renders idle — the exact case TraceSprite's throttle was built for.

### Split live elements OUT of the cache (critical, and where the Sampler note lands)

Anything that animates independently of params must draw **live on top** of the
cached sprite, never baked into it — the "split around live elements" pattern the
EQ backdrop/labels/curve already use.

- **Hover glow is already external.** ParamCards and chips implement
  `glowTarget`/`glowRect` (`synth_panel_zone.go:3384,3436`); the hover glow is
  composited by the shared tree hover overlay (one-spring hover doctrine), NOT
  inside `drawParamCard`. So caching the card's own draw output does not touch
  hover, and **hover state never enters a cache key** (keying on it would thrash
  the cache on every cursor move).
- **Selection** is a discrete state in `paramCardState`, so it goes in the key
  (baked into the sprite) — it changes rarely, unlike hover.
- The **numeric-editor overlay** (`paramEditor.Draw`) and any trigger cursor stay
  drawn live after the cached content, as today.

### Sampler tab — be mindful, but no changes needed

Recent work added Sampler-tab highlighting: `sampler.hoverHandle` (trim
start/end handle hover, mirroring the EQ-handle hover), `updateSamplerHandleHover`,
plus the own-color highlight tests (`sampler_handle_hover_test.go`,
`sampler_card_test.go`, `highlight_color_test.go`, `eq_handle_hover_highlight_test.go`).

Why this design leaves it intact:
- **`drawParamCard` is shared** by Synth (`:2587`) and Sampler
  (`sampler_panel_zone.go:1002`). This design does **not** change its signature —
  the TraceSprite callback passes `dst=sprite, r=origin-relative` to the same
  function. The Sampler keeps calling `drawParamCard` directly (surgical scope =
  Synth only) and is unaffected.
- The Sampler's `hoverHandle` highlight is on the **sampler waveform**, which this
  design does not cache, and it rides the same external-overlay pattern as the
  Synth hover glow. Nothing here bakes a highlight into a cached surface.
- Acceptance must confirm the Sampler stays green: run `sampler_card_test.go`,
  `sampler_handle_hover_test.go`, `sampler_knob_interaction_test.go`,
  `highlight_color_test.go` unchanged.

## Invalidation & eviction

- **Key-driven** for the common case: value / selection / size live in the key,
  so no manual dirty flags. Explicit `Invalidate()` only for theme reload or a
  forced relayout edge case.
- **Eviction on tab-hide** through the existing `releaseHiddenTabSprites(active)`
  chokepoint (`eq_panel_zone.go:547`) that EQ/Spectrum already use — extend it to
  `Release()` the Synth sprites (and the per-card map) when the Synth tab is not
  active. The per-card map also releases+deletes cards that leave the visible set
  on a section switch, so it stays bounded (~6–12 resident cards).

## Testing / acceptance (evidence-first)

1. **Pixel-identity per component** — cached vs `RENDER_SAFE` byte-identical
   (mirror `TestEQCurveSpriteMatchesDirect`): one test each for a ParamCard, the
   chip strip, the focus-graph, the full-note, and the mirror.
2. **Render-count semantics** — 0 re-renders steady-state; exactly 1 re-render per
   value change past the throttle (mirror `TestEQCurveSpriteRenderCount`). Prove
   the per-card map re-renders ONLY the edited card, not its neighbors.
3. **Byte-budget acceptance gate** — `TestSynthTab_LiveKnobEditFrameByteBudget`
   (`synth_live_param_edit_alloc_test.go`, 128 KB idle / 224 KB drag) is currently
   over budget (351–912 KB per `project_synth_tab_drag_byte_budget_regression`);
   this change should bring it green. Re-baseline the drag budget downward after.
4. **Alloc budget ratchet** — `perTabAllocBudget[TabSynth]` (currently 1198) drops;
   tighten it with a one-line rationale citing the sprite adoption.
5. **The money metric (end-to-end)** — re-run Harness B
   (`synth_stage_toggle_ui_bench.browser.test.js`): the `synth`-run `drawMaxMS`,
   `maxGap`, and `deferAbsMsP90` should fall toward the `eqControl` numbers
   (baseline: synth drawMax 43 ms / maxGap 3354 ms / deferP90 1088 ms vs
   eqControl 7.5 / 393 / 163). This is the direct proof the bottleneck shrank.
6. **Sampler regression** — the Sampler tests above stay green (no signature
   change, no highlight regression).

## Known gotchas carried in

- **ebitenstub fidelity** — reuse the SubImage/DrawImage fixes already in the
  TraceSprite work; a Synth sprite blit must be byte-identical under the stub.
- **Never mutate the RO merged-params map** inside a render callback (the
  `conceptMergedParams`/`MergedInstrumentParamsRO` ghost-clone bite — clone first
  if the callback needs to inject values).
- **Draw bugs are invisible under `play_ui.wasm`** (Update-only, no Draw) —
  acceptance uses real `main.wasm` + real mouse drags
  (`setEQTab`/`selectSynthSection`/`synthKnobRects`), which Harness B already does.
- **`paramCardState` must be fully discrete** in the cache key — if it carries any
  per-frame-varying field, exclude it from the key and draw it live on top.

## Files touched (anticipated)

- `src/go/internal/ui/synth_panel_zone.go` — sprite ownership on `synthPanel`;
  `drawSynthChipStrip`, `drawSynthDetailPane` (per-card sprite map) call sites.
- `src/go/internal/ui/synth_focus_graph.go`, `synth_full_note.go`,
  `synth_preview_layer.go` — wrap the three trace draws.
- `src/go/internal/ui/eq_panel_zone.go` — extend `releaseHiddenTabSprites` to the
  Synth sprites.
- New tests: `synth_component_sprite_test.go` (pixel-identity + render-count),
  plus the ratchet edits to `synth_live_param_edit_alloc_test.go` /
  `eq_panel_draw_alloc_discipline_test.go`.
- No change to `param_card.go`'s signature; no change to Sampler files.

## Out of scope

- Consolidating the hand-inlined `chain_trace_cache.go` / `wave_trace_cache.go`
  onto TraceSprite, and TraceSprite generics.
- The pad-shift-reuse and keyed-map-multi-instance strategies from the
  grid/rows caches.
- Any Sampler-tab caching or highlight change.
- The DSP-side stage-toggle re-render (secondary per the attribution note).
