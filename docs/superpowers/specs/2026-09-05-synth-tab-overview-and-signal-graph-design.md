# Synth tab — one overview for every instrument, pipeline strip, drilled signal graph

Date: 2026-09-05
Status: implemented 2026-09-05 (working tree, uncommitted)
Supersedes the UI half of `2026-07-21-synth-macro-controls-proposal.md`
(the "macro layer" it describes is the starting point here, not a parallel
path).

## Problem

The Synth tab has two different user flows depending on which instrument is
selected.

- 85 melodic recipes and the 8 kick-stage recipes get the **category layer**
  (`synth_macro_*.go`): a root of plain-English concept cards (Punch / Boom /
  Tone / Length / Source / Grit for a kick; Sound / Brightness / Attack /
  Length / Movement / Body / Warmth for a violin) that drill into that
  concept's raw params.
- The other 30 recipes with a schema — the 19 bespoke drums (snare, hihat,
  clap, toms, cymbals, shaker, cowbell, sub-bass), the 5 FM instruments, the
  base `synth-modular`, the 3 congas, the pad and the whisper voice — fall
  back to the **legacy stage UI**: a 13-chip pipeline strip (VOICE · OSC · ENS
  · FM · PITCH · LFO · BURST · ENV · FILTER · F.ENV · FRM · RES · POST) with
  a stage detail pane of every raw knob.

So the DnB Kick on row 1 shows six friendly cards and the Snare on row 2
shows thirteen chips of jargon. On top of that:

- the category root never shows the signal pipeline, and the legacy view
  never shows categories;
- a root card gives no hint of which controls live inside it;
- the header is a waveform thumbnail that mostly shows a placeholder, a
  truncated `dnb-kick — synth-modular-…` caption, and four buttons — while
  the Sampler tab next door already has the cleaner "title left, actions
  right" header;
- the right pane stacks three small cards ("Your sound", "Up close", focus
  graph) whose combined message is unclear, and nothing on the tab shows
  how a control acts on the sound *over time*.

## Goal

One flow for every synth instrument. A toddler with an iPad should be able
to pick a card, watch a picture, and drag a control; a synth nerd should be
able to reach every raw param one "More" away. Concretely:

1. **Overview**: categories with a sneak peek of their essentials, plus the
   live pipeline (active stages only), for every instrument.
2. **Drill**: tap a category or a pipeline stage → its essential controls,
   the rest behind "More", and one large animated signal graph.
3. **Animations**: a smooth morph when a control changes and a play-through
   sweep when the note fires. Minimal, clean, and free on steady frames.
4. **Header**: Sampler-style, no thumbnail, no recipe id.
5. **Legacy stage UI deleted.** No instrument keeps a different flow.

Non-goals (explicitly out of scope, unchanged): the audio engine, the recipe
schema, JSON import/export, persisted prefs, the armed-control barrels, the
mobile sticky-bar action cluster, macro "drag the whole concept" adjustment
(cards navigate; they do not fuse params into one knob).

## Decisions taken with the user (2026-09-05)

| Question | Decision |
|---|---|
| Pipeline strip role | Tapping a stage **drills into it**; same drilled screen as a category. Enable/disable lives inside the drilled view. |
| Drilled content | **Curated essentials + "More"** expander holding the remaining raw params. Same shape for every instrument. |
| Animations | **Both**: morph on edit (~200 ms) and play-through sweep when the note fires. |
| Architecture | **Grow the existing category layer into the only Synth tab; delete the legacy chip strip + stage detail pane** (approach 1 of 3). |

## 1. Overview (root)

Inside the Synth tab content rect, top to bottom. Right pane keeps its
current width rule (`synthPreviewWidth`: a third of the content, capped at
`SynthPreviewTargetW`, hidden on mobile).

### 1.1 Header band
See §3.

### 1.2 Pipeline strip

One row of stage pills in signal order, joined by the existing accent
chevron connector. Only stages that are **on and relevant** appear:

- a stage with an enable param whose value is off is skipped;
- a value-gated stage (ENSEMBLE at `unison_voices == 1`, RESONATOR at
  `body_model == 0`) is skipped when at identity;
- a stage pruned by `synthSectionOrderForSchema` (no knob, no pill) is
  skipped;
- VOICE (locked, always on when populated) and POST (always present) always
  show.

Examples: a plain modular voice reads `OSC → ENV → FILTER → POST`; the DnB
kick reads `VOICE → PITCH → ENV → POST`; a violin with LFO vibrato and a body
model reads `OSC → LFO → ENV → FILTER → F.ENV → RES → POST`.

Each pill = short stage label (`sectionShortLabelKey`) + a faint stage
thumbnail (the existing `synthStageThumbs` watermark: the signal rendered up
to and including that stage). Tapping a pill drills into that stage (§2).

A trailing **`+` pill** opens the shared action menu listing every off stage
by its localized label; picking one turns it on (`applySynthStageToggleByID`)
and drills into it. This is the only place a bypassed stage is visible at the
root. When no stage is off, the `+` pill is hidden.

The strip is drawn through the existing chip `TraceSprite` (key: active-stage
set + thumb content version + rect), so a steady frame is one blit.

### 1.3 Category cards

The concept cards from `buildMacroGroups`, laid out by the existing
`ControlGrid` (3–4 columns desktop, 1 column mobile). Each card:

```
┌──────────────────────────────────────────┬─────────┐
│ Brightness                            ›  │  ╭───╮  │
│ Dull and dark, or bright and sharp?      │ ─╯   ╰─ │  ← signature graph
│ Cutoff ▮▮▮▯   Resonance ▮▯▯▯   Type LP   │         │  ← essentials gauges
└──────────────────────────────────────────┴─────────┘
```

- **Title + one-liner** as today.
- **Signature graph** on the right band as today (`macroGraphRenderer`).
- **Essentials gauges row** (new): up to three of the category's essential
  params (§2.2), each as `short name + 4-segment level bar` (enum params show
  the enum label instead of a bar). Read-only at the root. Rendered from
  `MergedInstrumentParamsRO`, so they track live values without a render.
- Card height grows by one caption row to host the gauges; the grid scrolls
  vertically when the panel is short (same `ControlGrid` scroll + edge fades
  the drilled grid uses today). No card is ever hidden.

Tapping a card drills into that category.

### 1.4 Right pane at the root

One card, "Your sound": the full rendered note (`drawSynthFullNote`, the
existing off-thread `synthMirror` full-note slot). The "Up close" steady-wave
card and the focus-graph card are removed from the root. On a note trigger
(§2.4) a playhead sweeps this card too.

## 2. Drilled view (category or stage)

Navigation state stays the per-instrument map (`instEditorMacroOpen`), but a
drilled target becomes a small struct `{kind: category|stage, id}` rather than
a bare category id.

### 2.1 Header row

`‹ Back` pill · target name · one-liner · (right) the stage **enable pill**
when the target is a stage, or when the category maps to exactly one stage
(Attack → ENVELOPE, Warmth → POST, Movement → LFO, Voice → FORMANT, Body →
RESONATOR, Flam → BURST, Pitch Drop → PITCH). Categories spanning several
stages (Sound, Brightness, Tone, Punch, Boom, Source) show no pill. The pill
reuses `synthStageEnableState` / `applySynthStageToggle`, including the
locked and sole-source (`StageDisableSilences`) states.

### 2.2 Essentials + More

- **Essentials** = the target's params whose `ParamDef.Tier` is
  `TierEssential`, in the category table's declared order (a stage uses the
  schema order). No new classification is introduced; `advancedTierParams`
  remains the single source of truth.
- **More** = the remaining `TierAdvanced` params, collapsed behind the
  existing Advanced divider row (`synthAdvDividerRect` /
  `drawSynthAdvDivider`, split by `synthSectionTierSplit`), relabelled
  "More" through its i18n key. Expanded state is per instrument + target,
  session-only. When a target has no advanced params the divider is omitted.
- Every param renders as the existing `ParamCard`; tap arms it; barrels,
  numeric editor, undo, `SetInstrumentParam` write-through — all unchanged.
  `growArmedCard` keeps working because the drilled grid is the same
  `ControlGrid`.
- Discrete pickers (`osc_type`, `filter_type`, `body_model`, `lfo_target`,
  `formant_vowel`, `formant_voice_type`, `fm_algorithm`, kick variant) stay
  enum pickers.

### 2.3 Signal graph (right pane, drilled)

One large card replacing the three stacked cards. It shows the **selected
control** (the armed one, else the first essential) in its own domain using
the existing property-native renderers (`synthFocusRendererForControl`:
filter response, envelope, pitch sweep, FM harmonics, waveshaper transfer,
LFO, burst, output wave). Caption = control label + `PlainEnglish` gloss;
axis cues via the existing `drawConceptTimeRuler` / cutoff marker. The
"stage off" honesty scrim stays.

Mobile: the same card at a fixed height (`SynthRightCardMinH × 3`) placed
above the essentials grid.

### 2.4 Animations

Both animations live in one small state struct on the graph and cost nothing
when idle: the card is wrapped in the existing `TraceSprite`, and the sprite
key includes the animation progress, so a settled frame is a texture blit.

**Morph on edit.** Each renderer already samples a curve into a slice. The
graph keeps `prev []float64` and `next []float64` (same length) plus a
progress `t ∈ [0,1]`. On any param change for the selected control (or a
different control being selected), `prev ← current blend`, `next ← fresh
sample`, `t ← 0`. Each frame `t` relaxes toward 1 with the existing
`ExpDecayAnim` helper (`DecayStep` on `1−t`; rate tuned so the settle is
~200 ms at 60 fps; added to DESIGN.md `animations:` as `synth-graph-morph`
and regenerated). The drawn curve is `prev + (next−prev)·ease(t)` per sample.
Renderers gain a single seam: a `sample(...) []float64` function separated
from `draw(samples)`, so the morph is a blend of two sample slices and not a
second rendering path. Renderers without a natural curve (the output-wave
fallback) morph via the same blend on their wave samples.

**Play-through.** When the instrument fires, the graph records `playStart`
(frame) and `playDur` (the note's render duration from the recipe's
`DurationSec`, capped at 2 s). While active, a 1 px vertical playhead in
`primary` sweeps the time axis left→right and a small filled dot rides the
curve at the playhead's x; the sweep progress is linear in wall time. Graphs
without a time axis (filter response, waveshaper) instead brighten their
stroke for 150 ms with the `sin-pulse` helper. The root "Your sound" card
sweeps the same way. When `playStart + playDur` passes, the state clears and
the sprite key stops changing.

**Trigger source** (no audio taps, no goroutines): a new sync method
`(*DrumView).onInstrumentTriggered(instID)` — called from the highlight
consumer in `game_highlight_state.go` when a highlight event for a row
arrives (the row already knows every hit; resolve `Rows[row].Instrument`),
and from `previewActiveSynth` for the Preview button. It sets the play state
on the graph only when the Synth tab is visible and `instID` is the tab's
resolved instrument; otherwise it is a no-op. Runs under `seqMu` like the
other sync handoffs and must not call back into `Game`.

**Budget.** Only the signal graph animates; cards and the pipeline strip are
static. The existing `synth_drawcall_budget_test.go` and
`eq_panel_draw_alloc_discipline_test.go` caps stay as they are; the graph's
per-frame work while animating is one blend over ≤256 samples plus one
sprite re-render, and zero allocations on steady frames.

## 3. Header

Mirrors the Sampler header (`layoutSamplerHeaderButtons`):

```
SYNTH · Violin                              [Preview] [Save] [Save As] [Reset]
```

- Left: `i18n(KeyTabSynth)` in `RoleSectionHeader`, a middle dot, then
  `audio.InstrumentDisplayName(resolved)`. No recipe id, no waveform
  thumbnail (`drawSynthHeaderWaveform`, `synthHeaderThumbW`, the placeholder
  and `testSynthHeaderSampleOverride` are deleted).
- Right: the four existing buttons with their existing handlers and specs;
  Save renders in the primary (gold) style when the recipe is dirty, exactly
  like the Sampler's Save.
- Height: `SynthHeaderH` unchanged — it is already the shared synth/sampler
  header token; only the content inside the band changes.
- Mobile: unchanged — no in-panel header; Preview / Save / ⋯ stay on the
  sticky bar (`MobileActionButtons`).

## 4. Category tables — every instrument

`buildMacroGroups` is called for **every recipe with a non-empty schema**;
only WAV-sample instruments keep the "does not use the synth" banner. The
`synthMacroEnabled` flag and the `IsMelodicRecipe || schemaUsesKickStage`
gate are removed.

Table selection:

| Recipes | Table |
|---|---|
| melodic (85), FM (5), base modular, pad, whisper | melodic |
| kick-stage (8), bespoke drums (19), congas (3) | percussion |

Table changes:

- Percussion table gains the bespoke drum families' voice knobs, mapped by
  group: snare noise/tone and hihat/cymbal metal knobs → **Tone**; tom / conga
  / snare body pitch → **Boom**; snap, click and transient knobs → **Punch**;
  family decay/tail knobs → **Length**; `<family>_wave` generator picker →
  **Source**; family drive → **Grit**. The exact name lists are written from
  the drum ParamDefs during implementation and pinned by the discipline test
  "every visible non-gain param is claimed by exactly one category".
- Each table entry gets `Stages []synthSectionID` (the stage(s) its params
  live in) so the drilled header can show a pill for single-stage
  categories (§2.1) and the pipeline strip can highlight the stage a
  category maps to.
- Glosses and labels move to `internal/i18n` keys (`synth.cat.<id>.label`,
  `synth.cat.<id>.gloss`) with EN + es-419 strings.
- HIDE-UNUSED predicates stay as they are.

## 4b. Reuse map — nothing new where something exists

The user's standing rule: reuse the component tree and the grid-layout
infrastructure; never hand-roll hit-testing or layout. Every new surface
below is built from a named existing component.

| New surface | Built from | Not built |
|---|---|---|
| Pipeline stage pills, `+` pill, `‹ Back`, stage enable pill, More divider | `Button` via `NewSpecButton` (pill specs already used by the sticky bar), appended to `dv.instEditorBtns` so `synthTabHitAreas` publishes them through the existing `buttonHitAdapter` | no new hit adapters, no custom press logic |
| Pipeline strip layout | `ControlGrid` with `SetMaxCols` for the desktop single row and greedy wrap on mobile; chevron connector = the existing `DrawIcon(IconChevronRight)` gap code | no hand-rolled x/y cursor loop |
| Pipeline pill thumbnails | `synthStageThumbs` (already rendered off-thread, hash-gated) | no new preview renders |
| `+` off-stage menu | `dv.openActionMenu` (the shared action menu, keyboard nav included) | no new popup |
| Category cards + drilled param grid | the existing `ControlGrid` instances (`instEditorMacroGrid`), `drawMacroCard`, `drawParamCard`, `drawControlGridEdgeFades`, `growArmedCard` | no new grid |
| Card essentials gauges | `drawParamCardGauge` at gauge size + `knobPillLabel` for enum labels | no new bar drawing |
| More expander | the existing Advanced divider (`synthAdvDividerRect`, `drawSynthAdvDivider`, `synthSectionTierSplit`); only its label key changes | no new divider |
| Header | `layoutSamplerHeaderButtons` pattern and the same `ComponentButtonPrimary` / `Secondary` specs; title via `DrawTextStyled(RoleSectionHeader)` | no new header component |
| Signal graph card | `TraceSprite` (same holder as `synthSpriteCache.focus`), `synthFocusRendererForControl`, `drawConceptTimeRuler`, `drawWaveTrace`, `DecayStep` / `SinPulse` from `animation_helpers.go` | no new renderer family, no new easing code |
| Root "Your sound" | `drawSynthFullNote` + `synthMirror` full-note slot | unchanged |
| Input routing | the existing `EQPanelZone.HitAreas → synthTabHitAreas` chokepoint and z-tiers (`ZEQPanel+1/+2`), `Touch + ClipRect` for touch expansion | no new zone, no new dispatcher path |
| Scenes / screenshots | `synthTabSceneSetup`, `macroSceneSetup`, `SubjectSynthPanel` | no new harness |

The only genuinely new code is: the active-stage filter for the strip, the
gauges row inside the card, the `{kind,id}` drill target, the graph's
`prev/next/t` + playhead state, and the `onInstrumentTriggered` sink.
Everything else is layout wiring over the table above.

## 5. Files

| File | Change |
|---|---|
| `synth_macro_groups.go` → `synth_category.go` | tables gain `Stages`; drum-family params added; i18n keys; `categoryEssentials(group, schema)` helper (tier split). |
| `synth_macro_nav.go` → `synth_category_nav.go` | drilled target becomes `{kind, id}`; More-expanded map. |
| `synth_macro_render.go` → `synth_category_render.go` | root card gauges row; drilled header pill; essentials/More layout; hit areas for Back, More, stage pill. |
| **new** `synth_pipeline_strip.go` | active-stage list, pill layout, thumbs, `+` menu, hit areas, sprite key. |
| **new** `synth_signal_graph.go` | graph state (prev/next/t, playStart/playDur), morph + playhead drawing, sprite wrap, `onInstrumentTriggered` sink. |
| `synth_concept_viz.go`, `synth_focus_graph.go` | split each renderer into `sample` + `draw(samples)`; no visual change. |
| `synth_panel_zone.go` | Sampler-style header; `buildSynthTab` / `drawSynthTab` / `synthTabHitAreas` lose the legacy branch; the chip strip, stage detail pane, `layoutSynthSections`, `placeKnobsInSection`, chip types and their helpers are deleted. The Advanced divider (`synthAdvDividerRect`, `drawSynthAdvDivider`, `synthSectionTierSplit`) is kept and becomes the drilled view's More expander. |
| `synth_preview_layer.go` | `drawSynthPreviewPane` + osc/ADSR/filter plots deleted if no remaining caller; `drawSynthMirror` ("Up close") deleted. |
| `game_highlight_state.go` | call `dv.onInstrumentTriggered` on highlight arrival. |
| `drumview.go` | field cleanup: `instEditorChips`, `instEditorDetailR`, `instEditorSections*`, `instEditorSelectedSection`, `instEditorMobileFocusRect` removed; graph + pipeline fields added. |
| `DESIGN.md` + `make gen-design-tokens` | `synth-graph-morph` animation; gauge bar sizes as density tokens. |
| `scene_catalog.go` | new scenes (§7); legacy stage scenes removed. |
| `CLAUDE.md` | Synth-tab row of the per-tab map rewritten. |

No change to `internal/audio`, schemas, ABI, goldens, export, or prefs.

## 6. Mobile

Same screens, stacked. Header actions stay on the sticky bar. Pipeline strip
wraps greedily into two rows of pills (as the old chip strip did). Category
cards single-column with the gauges row under the one-liner. Drilled view:
signal graph card (fixed height) first, then the essentials grid, then More.
Armed barrels unchanged. Every new scene declares `MobileSetup`.

## 7. Testing and verification

Unit (fast `-tags test` path):
- For every registered recipe: categories non-empty, every root category
  drills to ≥1 bound knob, every visible non-gain param claimed by exactly
  one category, essentials non-empty per category.
- Pipeline strip: off stages skipped, value-gated identity skipped, order is
  `unifiedSynthSectionOrder`, `+` pill hidden when nothing is off.
- Signal graph: morph progress settles to 1 and stops changing the sprite
  key; playhead clears after `playDur`; `onInstrumentTriggered` is a no-op
  for a different instrument or a hidden tab; zero allocations on a settled
  frame.
- Header: EN and es-419 labels fit at every supported width; Save style
  flips with dirty state.

End-to-end through `Game.Update` with real input (per
`feedback_verify_end_to_end_not_just_units`): root card tap drills; pipeline
pill tap drills; Back returns; More expands; drilled control tap arms and
grows; Preview starts a play-through; `+` turns a stage on and drills.

Screenshots (opened and inspected, desktop + `MOBILE=1`): `crop_synth_root_kick`,
`crop_synth_root_violin`, `crop_synth_root_snare`, `crop_synth_root_fm_bell`,
`crop_synth_drilled_category`, `crop_synth_drilled_stage`,
`crop_synth_drilled_more`, `crop_synth_graph_morph_mid`,
`crop_synth_graph_playthrough_mid`, `crop_synth_header`.

Discipline: `synth_drawcall_budget_test.go`,
`eq_panel_draw_alloc_discipline_test.go`, token discipline, forbidden glyphs,
`zone_input_isolation_discipline_test.go` all stay green.

## 8. Legacy removal and migration

Deleted: chip strip, stage detail pane, section layout, "Up close" mirror
card, three-card right pane, header thumbnail, their scenes, and the
`synthMacroEnabled` gate. Kept: stage thumbs, ParamCards, the Advanced
divider (as More), armed controls, barrels, sprite caches, mirror full-note
renderer, concept renderers, section ids and the stage enable-state
resolver.

The ~26 test files that reach into `instEditorChips` /
`synthSelectedSection` / `drawSynthChipStrip` / `instEditorDetailR` are
migrated to the category + stage-drill paths, or removed when the behaviour
no longer exists. Tests asserting stage semantics (enable states, locked
VOICE, sole-source OSC, bypass scrim) are re-targeted at the drilled stage
view, not deleted.

## 9. Rollout order

1. Category tables for all recipes + gate removal + i18n glosses.
2. Header.
3. Pipeline strip.
4. Root essentials gauges.
5. Drilled essentials / More + stage drill + stage pill.
6. Signal graph + morph + play-through + trigger sink.
7. Mobile pass.
8. Legacy deletion + test/scene migration + CLAUDE.md.

Each step ends with a screenshot inspected on desktop and mobile.
