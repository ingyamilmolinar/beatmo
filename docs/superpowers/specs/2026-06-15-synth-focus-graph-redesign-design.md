# Synth Tab Focus-Graph Redesign

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design — approved, pending spec review

## Problem

The Synth tab tries to teach "what does each knob do" by drawing a tiny concept
graph under *every* knob, plus a "Your sound" output wave in the right pane. After
several iterations it still fails on the user's three core complaints:

1. **Per-knob graphs are tiny and hard to see.** The viz band is chrome-limited to
   ~22px on the default panel (`SynthConceptVizH` 38/50/58 by density, but squeezed
   by header + chip-strip). You cannot make 8–12 graphs-per-stage big — there is no
   room.
2. **It's hard to tell how each knob affects the end signal, all else equal.** Every
   knob currently renders the *output waveform* (`conceptKnobWave`). A sine vs a
   slightly-brighter sine look almost identical, so subtle knobs (Attack, Reso, Pan,
   Level) read as "nothing happened / broken."
3. **"Your sound" looks like a block/garbage — too crammed.** It packs more cycles
   than it needs into ~280px, collapsing into a solid fill for some instruments.

## Goal

Make the per-knob effect **faithful but easy to read**, and give the graphs room,
by replacing twelve tiny graphs with **one big focus graph** driven by an explicitly
*selected* knob, and by stacking it under a de-crammed "Your sound" anchor.

## Decisions (locked with user)

- **Structure:** one big **focus graph** for the selected knob — not a graph under
  every knob. (Brainstorm option B.)
- **Representation:** the right pane **splits into two stacked graphs** (option C):
  - **Top — "Your sound":** the real output wave, the stable end-signal anchor.
  - **Bottom — focus graph:** a **property-native** render of the *selected* knob
    (filter → frequency curve, envelope → ADSR, drive → transfer curve, etc.), with
    a dashed "before" ghost, updating live while that knob is dragged.
- **Knob cells are decluttered:** the per-knob concept-viz band is **removed**. Each
  cell is dial + label + value only. Reclaimed height makes dials breathe.
- **Selection is by click, sticky, never hover.** Clicking a dial selects it (pink
  highlight ring + pink label) and drives the focus graph. Hover does *not* change
  selection (would thrash the graph as the pointer crosses dials). Dragging the
  selected knob updates the focus graph live.
- **Default state:** on opening a stage, the stage's **main knob auto-selects** (e.g.
  Cutoff for Filter) so the focus graph is never empty.

## Architecture

All work is in `src/go/internal/ui/` plus the preview-render data layer in
`src/go/internal/audio/`. We reuse the renderers that already exist; the change is
*where* they're drawn (one big pane, not per-knob) and *which* one is drawn (the
selected knob's natural domain).

### 1. Selection state

New per-stage selected-knob state on `DrumView` (the synth panel zone). Keyed so it
survives within a stage and resets sensibly on stage switch:

- `synthSelectedKnob` — the resolved knob index currently explained by the focus
  graph (default = the stage's main knob).
- Set in the production hit adapter **`synthKnobHitAdapter.OnPress`** (NOT the dead
  `handleSynthTabInput`) — see the knob-overhaul lesson: wire into the real adapter
  and prove it via a `ViaTreeDispatch` test.
- A `mainKnobForStage(section)` helper picks the default (Cutoff/Attack/Generator/…).
- No hover path is added. Confirm via a test that moving the pointer across dials
  without pressing does not change `synthSelectedKnob`.

### 2. Right-pane split geometry

`drawSynthTab` currently gives the whole `instEditorPreviewRect` to
`drawSynthMirror`. Split it:

- **Top band — "Your sound":** `drawSynthMirror`, retuned (see §4).
- **Bottom band — focus graph:** new `drawSynthFocusGraph(dst, rect, instID, selectedDef, ghost)`.
- New density tokens (DESIGN.md → `make gen-design-tokens`): `SynthFocusGraphH`,
  and a split ratio / min-heights so both bands stay legible. Follow the
  dimension-token discipline (kebab-case in `densities:`, read via
  `Profile().DensityValues()`).

**Mobile:** `synthPreviewWidth()` returns 0 on mobile (no horizontal split). On
mobile, stack "Your sound" + focus graph as a horizontal-by-full-width band **above
the knob grid** (so a selected knob's explanation is visible without the side pane).
Drive it from the same selection state.

### 3. Remove the per-knob viz band

- `placeKnobsInSection()` / cell-height formula: drop `SynthConceptVizH` from
  `cellH` so dials get the height back.
- `drawSynthDetailPane()`: delete the per-knob `conceptRendererForGroup(...)(vizR…)`
  draw block.
- Keep the concept renderer *functions* (`conceptFilter`, `conceptEnvelope`,
  `conceptMotion`, `conceptFM`, the transfer/osc renderers) — they are reused by the
  focus graph, just drawn large. `conceptKnobWave` (output-wave-per-knob) is retired
  from the cell path.

### 4. "Your sound" de-cramming

The complaint is **too many cycles → block**. Fix in `drawSynthMirror` /
`synth_preview_layer.go`:

- Show ~1–2 clean cycles (cut `synthMirrorWaveCycles`/auto-zoom target so the trace
  is generously spaced), keep the existing steady-timbre `RenderInstrumentPreviewWave`
  + soft-clamp + normalize so any pitch stays legible.
- A test asserts the displayed trace is **not crammed**: bounded zero-crossings /
  total-variation per width (a legibility ceiling), for every shipped recipe.

### 5. Focus-graph renderer registry (property-native)

`drawSynthFocusGraph` dispatches by the selected knob's **role**, reusing existing
renderers drawn at full focus-pane size. Mapping by `ParamDef.Group`:

| Knob domain | Focus render | Source |
|---|---|---|
| filter (cutoff/reso/type) | frequency-response curve | existing `conceptFilter` |
| env (a/d/s/r) | ADSR envelope | existing `drawSynthADSRPlot` / `conceptEnvelope` |
| pitch / pitchenv / lfo / burst | modulation-over-time | existing `conceptMotion` |
| fm (ratio/depth) | harmonic / sideband view | existing `conceptFM` |
| osc (shape/Generator) | single-cycle waveform | osc plot |
| post / drive (waveshaper) | input→output transfer curve | `postCurveSamples` (tanh) drawn as transfer |
| level / gain / pan / tune (no rich domain) | **output wave with real amplitude/period** so the change is visible (level → height, pan → L/R, tune → period) | fallback renderer |

- Every renderer draws a dashed **before** ghost from `synthConceptGhost` when the
  pre-drag params differ.
- Each is given a clear axis label ("dull → bright", "loudness over time",
  "clean → crunchy") so it's kid-readable (`PlainEnglish` style).

## Testing (TDD — write the failing test first, every step)

Per user: validate with **screenshots** + **automated rendering AND DSP/data tests
for all knobs**.

### DSP / data tests (`internal/audio` + `internal/ui`)
- **All-knobs responsiveness sweep:** iterate `audio.RecipeRegistrations()`; for every
  visible knob of every recipe, assert the focus graph's *underlying data* changes
  when the knob moves Min→Max (extend the existing
  `TestRenderInstrumentPreview_RespondsToEveryStageParam` pattern to cover the focus
  renderer's data path, not just the mirror).
- **Per-domain data correctness:** filter curve gets brighter as cutoff rises (more
  high-freq energy); ADSR attack-time tracks the attack knob; transfer curve gets
  steeper with drive; level fallback's amplitude tracks gain. One assertion per domain.
- Keep DSP guards (clean/bounded/smooth, kick-clean, SVF clamp ≤0.9) green.

### Rendering tests (`internal/ui`, fast `-tags test` path)
- **Focus graph draws ink + morphs:** for each domain, the focus renderer produces
  ink and the rendered pixels change Min→Max (drawRect interceptor pattern).
- **Ghost appears** when pre-drag params differ; fades after release.
- **"Your sound" not crammed:** legibility ceiling test (per §4) for every recipe.
- **Selection drives the graph:** a `ViaTreeDispatch` functional test — real
  `Game.Update()` input loop: click dial A → focus graph shows A; click dial B → shows
  B; drag A → graph updates live. Assert **hover does not change selection**.
- **Default auto-select:** opening a stage selects its main knob; focus graph non-empty.
- **Discipline:** every live `ParamDef.Group` maps to an explicit focus renderer
  (extend `TestEveryRecipeGroupHasExplicitConceptRenderer`).

### Screenshots / scenes (`scene_catalog.go`, `scene_crop_test.go`)
- New `crop_synth_focus_*` scenes per domain (filter / env / fm / motion / osc /
  drive / level) — each selects the relevant knob and crops the right pane.
- A "Your sound" de-crammed scene (clean wave) for a previously-blocky instrument
  (e.g. kick).
- Desktop + mobile passes (mobile stacks the bands above the grid — declare
  `MobileSetup`). Regenerate via `make screenshots-all SCENES=…` and eyeball.
- Update `eq_panel_draw_alloc_discipline_test.go` budget if per-frame draws change,
  with a one-line rationale.

## Out of scope / YAGNI
- No new DSP engine work — reuse `RenderInstrumentPreviewWave` and existing concept
  renderers.
- No spectrum analyzer for the focus graph (the property-native domain views are
  enough).
- No animated per-knob hint icons (cells stay clean).

## Notes
- Per repo policy, the implementer does **not** `git add`/commit; changes are left in
  the working tree for the user to commit. (This overrides the brainstorming skill's
  "commit the design doc" step — the file is written but left unstaged.)
