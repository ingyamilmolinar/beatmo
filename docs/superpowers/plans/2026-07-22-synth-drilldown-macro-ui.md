# Synth Drill-Down Macro UI — Implementation Plan

> **STATUS (2026-07-22): Tasks 1–4 SHIPPED (melodic). The per-card graphs (Task 5), guardrails (Task 6), the PERCUSSION concept set, and mobile scenes were completed in the follow-up plan `2026-07-22-synth-macro-percussion-and-graphs.md`.** ⚠ NAMING: the shipped code uses `macro*` symbols, NOT the `concept*` names written throughout this doc (`concept*` collided with the existing `conceptOsc`/`conceptRenderer` graph renderers). Read the shipped files as the source of truth: `synth_macro_groups.go` (`macroGroup`, `buildMacroGroups`, `macroGroupDefsMelodic`/`…Percussion`), `synth_macro_nav.go` (`synthMacroOpen`/`setSynthMacroOpen`/`synthAtMacroRoot`, field `instEditorMacroOpen`), `synth_macro_render.go` (`drawMacroCard`, `layoutSynthMacroLayer`, `synthMacroHitAreas`, `macroGraphRenderer`).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) to implement task-by-task. Steps use checkbox (`- [ ]`) syntax. This is a central-UI feature — every task that changes rendering ends with a **screenshot verification** step (green Go tests are NOT sufficient for UI per CLAUDE.md).

**Goal:** Add a beginner-first drill-down layer to the Synth tab: a small set of plain-English "concept" cards (Sound, Brightness, Attack, Length, Width, Human Feel, Movement, Body, Warmth …); tapping a concept card drills into its real params (shown as the same cards); a leaf param card opens the existing wheel; a "Basic" button climbs back up.

**Architecture:** A **UI-only lens** over existing ParamDefs — no engine/ABI/golden/export change. A new `synth_concept_groups.go` maps each concept → an ordered list of existing param names (derived from the schema via the existing `sectionForParamDefIn` routing + a curated concept table). The synth panel gains one nav-state field (`instEditorConceptOpen map[string]string`, "" = root). `buildSynthTab`/`drawSynthTab`/`synthTabHitAreas` branch on root-vs-drilled: at root they render concept cards (new `drawConceptCard`, reusing the ControlGrid layout + card TraceSprite + concept graphs); drilled, they render only the open concept's param cards through the **existing** card/wheel/adapter path. A "Basic" pill in the detail header pops to root.

**Tech Stack:** Go 1.26, Ebitengine 2.9, the existing `internal/ui` synth panel (`synthPanel`), `internal/audio` recipe schema. Screenshot harness: `scene_catalog.go` + `make screenshots-all`.

## Global Constraints

- **UI-only lens:** writes go through the existing `audio.SetInstrumentParam` per leaf; NO new persisted state, NO change to RecipeParams / recipe hash / synth goldens / JSON export / plugin ABI. (verbatim from proposal spec)
- **Bundled Go:** `.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/…`.
- **Screenshots mandatory for UI:** `CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` then `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; open and inspect the PNG.
- **Discrete pickers stay pickers** (never fused into a continuous macro): `osc_type, body_model, fm_algorithm, filter_type, lfo_target, formant_vowel, formant_voice_type, kick_variant`.
- **More clean buckets > fewer overloaded ones** (user directive): keep each concept one idea.
- **Keep the current card UI**: reuse `drawParamCard`, the wheel popup, the graphs, the sticky bar. Only card *contents* + tap-target (drill vs dial) + a Basic button change.

---

## Concept grouping (the buckets)

Static table `conceptGroupDefs` (in `synth_concept_groups.go`). Each entry: `{ID, LabelKey, Gloss, Graph, Params []string, Gate}`. `Params` are the leaf param names the concept owns; `Gate` optionally hides the concept unless a predicate holds (hide-unused). Melodic set:

| Concept | Gloss | Params (leaves, in order) | Graph |
|---|---|---|---|
| `sound` | "The basic character of the sound." | osc_type, osc_octave, osc_detune, fm_algorithm, fm_op1_ratio…fm_op4_ratio, fm_op*_depth, fm_op*_level, osc_sax_*, osc_bow_* | conceptOsc |
| `brightness` | "Dull and dark, or bright and sharp?" | filter_cutoff, filter_resonance, filter_type, filtenv_amt, filtenv_decay, filtenv_attack | conceptFilter |
| `attack` | "Does it start instantly, or fade in?" | amp_attack | conceptAdsr(attack) |
| `length` | "A short pluck, or a long held note?" | amp_decay, amp_sustain, amp_release, amp_curve | conceptAdsr(decay) |
| `width` | "One thin voice, or a big wide group?" | unison_voices, unison_detune, unison_mix | conceptSpread |
| `humanfeel` | "Alive and human, or perfectly still?" | unison_drift_rate, unison_drift_depth, ens_scatter, ens_humanize, ens_jitter | conceptWavy · Gate: voices>1 |
| `movement` | "A steady note, or a moving one?" | lfo_rate, lfo_depth, lfo_target, lfo_delay | conceptWavy |
| `body` | "How real and acoustic it sounds." | body_model, body_mix, bow_dynamics | conceptBumps · Gate: body_model present |
| `warmth` | "Clean, or warm and gritty?" | drive | conceptSoftclip |
| `pitchdrop` | "A downward pitch punch (like an 808)." | pitchenv_amt, pitchenv_decay | conceptPitchWave · Gate: pitchenv present |
| `voice` | "The vowel and voice character." | formant_vowel, formant_voice_type, formant_mix, formant_breath, formant_sing, formant_shift, formant_morph_rate, formant_morph_to, formant_dry | conceptOsc · Gate: formant present |
| `flam` | "One hit, or a quick roll?" | burst_sharp, burst1_off…burst4_amp | conceptWavy · Gate: burst present |

Percussion/kick set (recipe uses the kick stage — `audio.schemaUsesKickStage`): `punch`(kick_attack), `boom`(kick_env0, voice_freq_hz), `pitchdrop`(kick_pe_amt, kick_pe_rate), `harmonics`(kick_h2/h3/h4), `click`(kick_click), `thud`(kick_noise), `tail`(kick_fade, kick_env1), `drive`(kick_sat), `character`(kick_variant, kick_mode_*, kick_reverb). Ordering by musical priority.

**Builder:** `conceptGroupsForSchema(recipeID string, schema []audio.ParamDef) []conceptGroup` — pick melodic or kick table by `audio.schemaUsesKickStage(schema)`; for each concept keep only `Params` that (a) exist in `schema`, (b) are not `synthParamHiddenInContext`; drop a concept with 0 surviving params; drop a concept whose `Gate` fails. Any schema param not claimed by a concept falls into a trailing `more` concept ("Extra controls.") so nothing is unreachable.

---

## Task 1: Concept-group model + builder (pure logic, TDD)

**Files:**
- Create: `src/go/internal/ui/synth_concept_groups.go`
- Test: `src/go/internal/ui/synth_concept_groups_test.go`

**Interfaces:**
- Produces: `type conceptGraphKind int`; `type conceptGroup struct { ID string; LabelKey i18n.Key; Gloss string; Graph conceptGraphKind; Params []string }`; `func conceptGroupsForSchema(recipeID string, schema []audio.ParamDef) []conceptGroup`.

- [ ] **Step 1: Write failing tests** in `synth_concept_groups_test.go`:
  - `TestConceptGroups_Melodic_ViolinHasCoreConcepts`: build schema via `audio.RecipeRegistrations()["synth-modular-violin"].Params`; assert `conceptGroupsForSchema` returns groups whose IDs include `sound, brightness, attack, length, width, movement, body` and that every returned group has ≥1 Param and every Param exists in the schema.
  - `TestConceptGroups_EveryParamReachable`: for violin, the union of all groups' Params (incl. `more`) ⊇ every non-hidden schema param name → nothing unreachable.
  - `TestConceptGroups_Kick_UsesPercussionSet`: for `synth-modular-kick-808`, IDs include `boom, pitchdrop, punch` and NOT `brightness`.
  - `TestConceptGroups_HideUnused`: a schema without formant params yields no `voice` concept.
- [ ] **Step 2: Run — expect FAIL** (`conceptGroupsForSchema` undefined).
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConceptGroups`
- [ ] **Step 3: Implement** `synth_concept_groups.go`: the two static tables + `conceptGroupsForSchema` (filter by presence + `synthParamHiddenInContext` + gates + trailing `more`). Reuse `audio.schemaUsesKickStage`.
- [ ] **Step 4: Run — expect PASS.** Same command.
- [ ] **Step 5: Also run** the fast UI suite subset to ensure no build break: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Concept|Synth' -count=1`.
- [ ] **Step 6: Commit? NO** — repo convention is never stage/commit; leave in working tree.

## Task 2: Nav state + root/drilled predicate (pure logic, TDD)

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (add state accessors near `selectedSynthSection` ~`:835`) or a small new `src/go/internal/ui/synth_concept_nav.go`
- Test: `src/go/internal/ui/synth_concept_nav_test.go`

**Interfaces:**
- Produces on `*DrumView`: field `instEditorConceptOpen map[string]string`; `func (dv *DrumView) synthConceptOpen(instID string) string`; `func (dv *DrumView) setSynthConceptOpen(instID, id string)`; `func (dv *DrumView) synthAtConceptRoot(instID string) bool`.

- [ ] **Step 1: Failing test** `synth_concept_nav_test.go`: new DrumView (via existing test ctor used by other synth tests — see `synth_panel_*_test.go`); assert default `synthAtConceptRoot`==true and `synthConceptOpen`==""; after `setSynthConceptOpen(id,"brightness")` root==false and open=="brightness"; after `setSynthConceptOpen(id,"")` root==true.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** the field (lazy-init map) + 3 methods.
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit? NO.**

## Task 3: `drawConceptCard` primitive + a static "concepts at root" scene (screenshot)

**Files:**
- Create: `src/go/internal/ui/concept_card.go` (draw a concept card: keycap shell reused from `drawParamCard`, label RoleBody, gloss RoleMicro, a graph area, a "›" drill caret)
- Modify: `src/go/internal/ui/scene_catalog.go` (add `crop_synth_concepts_root` using `synthTabSceneSetup("violin")` + `setSynthConceptOpen(root,"")`)
- Test: `src/go/internal/ui/concept_card_test.go` (a pixel/smoke test that `drawConceptCard` writes non-background pixels in the label+graph rects, mirroring `audio_panel_render_pixels_test.go` style)

**Interfaces:**
- Consumes: `drawParamCard` shell helpers (`param_card.go`), a graph render callback `func(dst *ebiten.Image, r image.Rectangle)`.
- Produces: `func drawConceptCard(dst *ebiten.Image, r image.Rectangle, label, gloss string, graph func(*ebiten.Image, image.Rectangle), st paramCardState)`.

- [ ] **Step 1: Failing pixel test** for `drawConceptCard` (assert label row + graph row have ink).
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** `drawConceptCard` (reuse the cap/bevel from `drawParamCard`; draw label, gloss via `RoleMicro`, call the graph callback in the graph rect, draw `IconChevronRight` caret).
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Wire the root render** minimally: in `drawSynthDetailPane` (`:2664`), when `dv.synthAtConceptRoot(instID)`, iterate `conceptGroupsForSchema(recipeID, schema)` and draw each via `drawConceptCard` into ControlGrid cells (reuse `placeKnobsInSection` grid math or a new `placeConceptCards`). Behind this branch the existing per-knob path is untouched when NOT at root.
- [ ] **Step 6: Add scene** `crop_synth_concepts_root` and **capture**:
  `CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd && xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene crop_synth_concepts_root -screenshot /tmp/concepts_root.png`
- [ ] **Step 7: OPEN /tmp/concepts_root.png and inspect** — concept cards render with labels + glosses + graphs, laid out in the detail pane, no overlaps/blank cards. Fix + re-capture until correct.

## Task 4: Layout + hit-routing (concept card grid, tap-to-drill, Basic pill)

**Files:**
- Modify: `synth_panel_zone.go` — `buildSynthTab` (`:1062`, lay out concept cards at root into the detail grid), `synthTabHitAreas` (`:3310`, publish a new `synthConceptHitAdapter` per concept card at root; publish the existing per-knob adapters only when drilled), detail header (`drawSynthDetailPane` ~`:2696`, draw a **"◀ Basic"** pill when `!synthAtConceptRoot`), and the drilled-knob filter (restrict `sel.knobIdxs` to the open concept's Params).
- Create: `synth_concept_hit.go` — `synthConceptHitAdapter` (`OnRelease` → `setSynthConceptOpen(instID, id)`), and the Basic-pill adapter (`OnRelease` → `setSynthConceptOpen(instID, "")`).
- Modify: `scene_catalog.go` — `crop_synth_concept_drilled` scene (`setSynthConceptOpen(root,"brightness")`).

**Interfaces:**
- Consumes: `conceptGroupsForSchema`, `synthConceptOpen/setSynthConceptOpen/synthAtConceptRoot` (Tasks 1-2), `drawConceptCard` (Task 3), existing `ControlGrid`, `synthControlHitAdapter`, `openSynthControlWheelPopup`.
- Produces: `type synthConceptHitAdapter struct{...}` implementing the HitArea handler interface (mirror `synthChipHitAdapter` `:3565`).

- [ ] **Step 1:** Implement `placeConceptCards(instID)` — lay concept-card rects with the same `ControlGrid` used by `placeKnobsInSection`; store rects for hit-testing (a new `dv.instEditorConceptRects []conceptCardSlot{id string; r image.Rectangle}`).
- [ ] **Step 2:** In `buildSynthTab`, when at root call `placeConceptCards`; when drilled, set the section's knob set to the open concept's Params (filter `sel.knobIdxs` by name∈concept.Params) and reuse `placeKnobsInSection`.
- [ ] **Step 3:** In `synthTabHitAreas`, when at root publish a `synthConceptHitAdapter` for each `instEditorConceptRects` slot (Z=`ZEQPanel+2`, Touch, ClipRect via `clipFor`); when drilled publish the existing per-knob adapters (unchanged) **plus** the Basic-pill adapter.
- [ ] **Step 4:** Draw the "◀ Basic" pill in the detail header when drilled (reuse `drawFXTogglePill` styling or a plain pill); store its rect `dv.instEditorConceptBackRect`.
- [ ] **Step 5:** Go test: extend `synth_concept_nav_test.go` — drive `dv.synthTabHitAreas()` at root and assert a hit at a concept card rect resolves to a drill (open becomes that id); a hit on the Basic pill when drilled resets to root. (Follow the functional-test convention — real `Game.Update`/HitIndex, per [[feedback_functional_tests_over_isolation]].)
- [ ] **Step 6: Run** `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Concept' -count=1` — expect PASS.
- [ ] **Step 7: Capture** `crop_synth_concept_drilled` + re-capture `crop_synth_concepts_root`; **OPEN both PNGs**. Root shows concept cards; drilled shows Brightness's param cards + a "◀ Basic" pill. Tapping is covered by the Go test; the PNGs prove render. Fix + re-capture until correct.
- [ ] **Step 8:** Manual real-input sanity via the desktop binary is optional; the Go functional test + screenshots are the gate.

## Task 5: Concept graphs (visual language) + glosses

**Files:**
- Modify: `concept_card.go` (map `conceptGraphKind` → the matching `synth_concept_viz.go` renderer, drawing a representative curve for the concept using the instrument's live params), `plain_english.go` (add the concept glosses as i18n keys), `synth_concept_groups.go` (reference the gloss keys).

- [ ] **Step 1:** Add a `conceptGraphRenderer(kind conceptGraphKind, instID string) func(*ebiten.Image, image.Rectangle)` that dispatches to existing `conceptOsc/conceptFilter/conceptAdsr/conceptPitchWave/…` (from `synth_concept_viz.go`) with a synthetic `def` representing the concept's headline param.
- [ ] **Step 2:** Add concept glosses to `plainEnglishMap` (or a parallel `conceptGloss` map) — one-liners from the table above.
- [ ] **Step 3: Capture** `crop_synth_concepts_root` again; **OPEN it** — each concept card shows its distinct graph (waveform / filter curve / envelope / spread / wavy / bumps / soft-clip) + gloss. Fix + re-capture.
- [ ] **Step 4:** Run `plain_english_coverage_test.go` + `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Concept|PlainEnglish' -count=1`.

## Task 6: Guardrails + full verification

- [ ] **Step 1:** Discipline test `synth_concept_groups_discipline_test.go`: every `Params` name in both concept tables is a real ParamDef name for at least one shipped recipe (guards against typos/param renames); the pickers listed in Global Constraints each live in exactly one concept and are never split.
- [ ] **Step 2:** Run the full fast UI + audio suites: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ -count=1`. Expect PASS (the lens touches no audio path).
- [ ] **Step 2b:** Confirm **zero** golden/ABI impact: `CGO_ENABLED=1 ../../.tools/go/bin/go test ./internal/audio/ -run 'Golden|Recipe|Wired' -count=1` unchanged.
- [ ] **Step 3: Screenshots** — `make screenshots-all SCENES=crop_synth_concepts_root,crop_synth_concept_drilled` and the mobile pass `MOBILE=1`; **open every PNG**. Verify desktop + mobile both render concept cards + drill + Basic.
- [ ] **Step 4:** Update `AGENTS.synth.md` / memory with the concept-layer entry.

---

## Phase 2 (later — NOT in this plan)
- Macro **drag-adjust** on a concept card (moving it drives all its child params via a curve + reverse-mapping + "Custom" flag).
- **Nested sub-macros** (Length → Feel/FadeTime/Hold/Tail as an intermediate level) → full recursive depth.
- i18n of every gloss; per-locale coverage.
- A "surprise me" randomizer as a second beginner on-ramp.

## Self-review notes
- Spec coverage: grouping (Task 1), drill-down nav (Tasks 2,4), keep-card-UI (Task 3 reuses `drawParamCard` shell + existing wheel), Basic button (Task 4), hide-unused (Task 1 gates), graphs+one-liners (Task 5), UI-only/zero-risk (Task 6 step 2b). ✔
- No placeholders: each task cites exact files/functions from the architecture map. Screenshot gates on every render task.
- Type consistency: `conceptGroup`, `conceptGroupsForSchema`, `synthConceptOpen/setSynthConceptOpen/synthAtConceptRoot`, `drawConceptCard`, `synthConceptHitAdapter`, `instEditorConceptRects/instEditorConceptBackRect` used consistently across tasks.
