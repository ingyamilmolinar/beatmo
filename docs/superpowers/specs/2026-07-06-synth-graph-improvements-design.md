# Synth-tab graphs — accurate, kid-readable (2026-07-06)

## Goal

Make the Synth tab's graphs (right-pane "global" wave views, per-knob focus graph,
and the pipeline chip strip) **accurate** — every graph truthfully reflects what the
knobs and stages do to the sound — and **kid-readable**: an inexperienced user can
get the gist without knowing any audio jargon.

User decisions (2026-07-06 brainstorm):
1. Right pane top band → **both views stacked**: a full-note "Your sound" view plus
   the current zoomed steady-tone view renamed "Up close".
2. Focus graphs → **kid-level story cues** (words + simple markers, no numeric axes).
3. Pipeline chip strip → **living flow strip**: arrows + tiny per-stage live wave
   thumbnails showing the sound after each stage. Flagged as the most expensive
   piece; may ship as a second wave after Sections 1–2.

## Current state (evidence)

All paths under `src/go/internal/ui/` unless noted.

- **"Your sound" mirror** (`synth_mirror.go`, `synth_preview_layer.go`): renders
  `audio.RenderInstrumentPreviewWave(inst, nil, 4, 128)` — a **cycle-normalized
  steady timbre** wave (osc+FM+filter+drive only). Envelope, LFO, burst, and pitch
  knobs change **nothing** in it. Auto-zoom caps display at 2 cycles
  (`mirrorDisplayWindow`, `synthMirrorDisplayCyclesMax=2`); `mirrorTraceGain`
  normalizes peak to ~80% band height. Hash-gated live render each frame
  (`renderLive` + `updateSynthMirrorLive`); prior render lingers as ghost.
- **Focus graph** (`synth_focus_graph.go`): `splitSynthRightPane` = mirror ~42% /
  focus ~58%. Routing: `synthFocusRenderers` (group→renderer) +
  `synthFocusRendererForKnob` per-knob overrides (pitch knobs → `conceptPitchWave`,
  FM time knobs → `conceptFMEnvelope`). Caption = `def.Label — synthKnobPurpose(def)`.
- **Concept renderers** (`synth_concept_viz.go`): bare curves, per-column fill +
  2px stroke, no in-graph text/markers/axes. Known problems observed in the
  2026-07-06 screenshot pass (`screenshots/all/desktop/crop_synth_*`):
  - `conceptMotion` serves pitch/pitchenv/lfo/burst groups → selecting a **burst**
    knob shows an **LFO wobble** curve (wrong story); burst tick markers are
    nearly invisible.
  - `conceptPost` draws an abstract tanh transfer-function diagonal — meaningless
    to a novice; at Drive 0% it is just a straight line.
  - FM waves render **blocky/stair-stepped** (64 pts/cycle stretched over a wide
    focus rect).
  - Filter response is a near-flat low-ink band with no cutoff marker.
  - Bypassed stages (LFO/BURST show "BYPASSED" in the detail pane) still draw
    full-effect curves with no hint that the stage is off.
  - `conceptPitchWave` has **no ghost** (RenderInstrumentPreview lacks an
    override-map form).
- **Pipeline strip** (`drawSynthChipStrip`, `synth_panel_zone.go:2079-2154`): pills
  joined by a 1px `genColorBorder@AlphaFaint` wire, no arrowheads, no depiction of
  the wave changing between stages. 7px enable state dot per stage.
- **Full-note preview**: `audio.RenderInstrumentPreview(instID, durationMs)`
  (`src/go/internal/audio/preview_render.go`) is tag-free pure Go and already
  responds to every stage param (guarded by
  `TestRenderInstrumentPreview_RespondsToEveryStageParam`). It is currently used
  only by the checksum path and `conceptPitchWave`.
- **Responsiveness gate**: `synth_focus_sweep_test.go`
  `TestFocusGraph_RespondsToEveryKnob` — bidirectional, currently 1210 responsive /
  50 exempted; named exemptions include `fm_op1_level`, `burst4_off`, `lfo_target`,
  `lfo_delay`, pair `fm-epiano/fm_op3_ratio`.
- **Mobile**: no side preview pane; a right column beside the knob grid
  (`instEditorMobileFocusRect`) is split by the same `splitSynthRightPane`.

## Section 1 — Right pane: three stacked cards

Desktop `instEditorPreviewRect` (and the mobile right column) splits into **three**
cards via an extended `splitSynthRightPane`:

| Card | Share | Content |
|---|---|---|
| **"Your sound"** | ~30% | Full rendered hit (`RenderInstrumentPreview`), DAW-clip style min/max column envelope |
| **"Up close"** | ~26% | Current steady-tone wave (auto-zoom ≤2 cycles) — today's mirror, renamed |
| **Focus graph** | ~44% | Per-selected-knob graph (Section 2) |

**"Your sound" (full note):**
- Source: `audio.RenderInstrumentPreview(instID, windowMs)` — already reads every
  stage param.
- **Adaptive window**: `windowMs` derived from the instrument's effective envelope
  (≈ attack + decay (+ release when present), ×1.2 headroom), clamped to
  [120ms, 1000ms], so a short kick fills the band and a pad isn't truncated to a
  sliver. Pure function + unit test.
- Drawn as a min/max column envelope (bipolar, filled), amplitude-normalized with
  the existing `mirrorTraceGain` clamp behavior.
- Live during drags via the same hash-gated `renderLive` mechanism `synthMirror`
  uses today; the full-note render is cached under the same params-hash with its
  own cache slot (separate from the steady-wave slot). Pre-drag shape lingers as a
  faint ghost trace.
- Rendering stays off the frame's hot path exactly as today: hash-gated (no param
  change → zero work per frame). If profiling shows the full-note render is too
  slow for synchronous in-frame use on WASM, fall back to the existing async
  `synth.preview` pool request path (stale-while-revalidate) — the mechanism
  already exists; the choice is made by measurement during implementation.

**"Up close":**
- Behavior unchanged from today's mirror (steady wave, auto-zoom, ghost).
- Renamed: new i18n keys `KeyCapUpClose` (EN "Up close" / es-419 "De cerca").
  `KeyCapYourSound` moves to the full-note card.

**Mobile fallback rule:** inside the extended `splitSynthRightPane`, if the pane
height cannot give all three cards ≥ their minimum legible height (density token),
**"Up close" collapses first** (most redundant with the focus graph), then the
remaining two split as today. No new layout machinery; same function serves
desktop and mobile.

## Section 2 — Focus graphs: accuracy fixes + story cues

### Accuracy fixes

1. **Split `conceptMotion` by story:**
   - `lfo` group → wobble-only curve (pure LFO, no pitch-env/burst mixed in).
   - `burst` group → new **`conceptBurst`**: a timeline of hit spikes
     (x = `burst*_off` when, height = `burst*_amp` how loud, width/sharpness =
     `burst_sharp`), with the spike belonging to the **selected knob's hit**
     highlighted in accent. Removes the `lfo_delay`-class confusion and should
     let `burst4_off` leave the exemption list (the hit-4 spike position is
     showable even when its amp is 0 — draw a dim placeholder spike).
   - `pitch`/`pitchenv` groups → the decay-sweep curve (reuse/extend the
     `conceptFMEnvelope` pitch-sweep form for modular pitch-env knobs).
2. **`conceptPost` (Drive/Tune/Gain) → before/after wave**: main trace = steady
   wave **with** the knob's current value, ghost = wave with the knob at a neutral
   reference (drive off / unity gain), rendered via
   `RenderInstrumentPreviewWave(inst, override, …)`. A kid sees the wave get
   squashed/edgier instead of an abstract diagonal.
3. **Bypassed-stage honesty**: when the focus knob's stage enable param is off,
   the graph draws dimmed (existing scrim pattern) with a one-line hint
   ("OFF — turn the stage on to hear this", i18n) while still previewing what the
   knob *would* do (force-enable override, as today).
4. **Ghost for pitch knobs**: add an override-map form
   `audio.RenderInstrumentPreviewWithOverrides(instID, overrides, windowMs)`
   (or an extra param on the existing function) so `conceptPitchWave` draws a
   pre-drag ghost like every other renderer. Also unblocks Section 3.
5. **Resolution/rendering quality**: for focus-sized rects, sample ~256 pts/cycle
   (up from 64) and draw a smooth stroked polyline with a lower-alpha fill
   underneath, replacing the chunky per-column fill (`conceptDrawFilledCurve`
   gains a smooth mode or a sibling helper). Kills the FM stair-steps.

### Kid-level story cues (1–3 per graph, words not numbers)

- **Envelope**: "starts" / "fades" region words under the curve; the selected
  knob's region (attack vs decay/release) emphasized (slightly stronger fill).
- **Filter**: ▼ marker at the cutoff position on the response curve; "dull" /
  "bright" words at the band's left/right ends (i18n; reversed appropriately for
  high-pass).
- **Wobble/burst/pitch-sweep (time-domain graphs)**: a minimal time ruler —
  baseline ticks + "0" and "1s" (or the actual window) end labels only.
- **Ghost legend**: while a drag is active (ghost differs), a tiny "before / after"
  hint in the card corner explains the faint trace (i18n).
- All markers via `DrawIcon` (no forbidden glyphs); all words via i18n +
  `PlainEnglish` glosses; text uses existing caption font scale and
  `TokenTextSecondary`/`colConceptStroke` tokens.

## Section 3 — Living pipeline strip (second wave; may ship after 1–2)

- **Arrows**: replace the 1px wire between chips with visible arrowhead connectors
  (new `IconArrowRightSmall` via `DrawIcon` if no suitable icon exists), drawn at
  readable weight. Mobile: arrows within each wrapped row only.
- **Per-stage thumbnails**: each enabled stage chip hosts a tiny live sparkline of
  the sound **after** that stage: full-note preview rendered with all **downstream**
  stages force-disabled via the override-map form (stage → enable-param mapping
  already exists in the section model / `conceptStageEnableParam`). Stages without
  an enable param (always-on, e.g. VOICE/OSC) are cumulative points in the chain.
  Disabled stages draw a flat dim pass-through line.
- **Cost control**: thumbnails render at tiny resolution, are cached by
  params-hash + stage index, rendered off-thread on the existing `synth.preview`
  pool; an edit only invalidates the edited stage and downstream.
- **Chip layout**: chips grow taller (label row above, sparkline below) — heights
  from new density tokens; mobile `synthChipWrapRows` compaction preserved.
- Accepted simplification: the preview synth is an approximation of the real C
  DSP (`preview_render.go` header); thumbnails teach the pipeline's *shape*, not
  bit-exact output. Same accepted basis as the existing mirror.

## Section 4 — Guardrails & verification

- **Tokens**: new sizes (card min-heights, thumbnail height, marker sizes) declared
  in DESIGN.md `densities:` (all three densities) → `make gen-design-tokens`;
  colors reuse existing viz tokens (`colWaveTrace`, `colAccent`, `colConceptGhost`,
  `genColorVizScopeBg`); token-discipline ratchet respected (no new inline literals).
- **i18n**: every new string (EN + es-419): `KeyCapUpClose`, bypass hint,
  before/after legend, dull/bright, starts/fades, time-ruler labels.
- **Tests (TDD, fast `-tags test` path):**
  - Extend `TestFocusGraph_RespondsToEveryKnob`; exemption list expected to
    **shrink** (`burst4_off`, `lfo_delay` become showable). Bidirectional gate
    keeps guarding stale exemptions.
  - New: full-note "Your sound" responds to envelope/LFO/burst/pitch knobs
    (fingerprint at Min vs Max differs); adaptive-window unit tests; "Up close"
    unchanged-behavior regression; mobile collapse-order test; bypassed-stage
    hint test; before/after drive test; burst-spike-highlight test.
  - Section 3: thumbnails differ stage-to-stage, respond to edits, cache
    invalidation only downstream; alloc budget: update
    `perTabAllocBudget` in `eq_panel_draw_alloc_discipline_test.go` only if
    needed, with one-line rationale.
- **Scenes/screenshots**: new `crop_` scenes (`crop_synth_yoursound_env`,
  `crop_synth_focus_burst_hit`, `crop_synth_focus_drive`, `crop_synth_pipeline`)
  with Setup+MobileSetup; refresh existing `crop_synth_*`; final visual
  verification desktop **and** mobile.
- **Process**: nothing staged or committed (standing user policy); WASM + native +
  `-tags test` all green before claiming done.

## Non-goals

- No changes to the Chain tab scope, the audio-panel analyzers, or the actual DSP
  output — display-only work plus the additive preview override-map primitive.
- No numeric/technical axes (Hz/dB ticks) on synth graphs.
- No redesign of knob grid, chip selection model, or section ordering.
