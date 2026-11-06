# Synth Tab Overview + Signal Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One Synth-tab flow for every instrument: a root of category cards (with essentials gauges) plus an active-stage pipeline strip, a drilled view (category OR stage) of essentials + "More" with one animated signal graph, a Sampler-style header, and the legacy chip strip / stage detail pane deleted.

**Architecture:** Grow the existing category ("macro") layer in `internal/ui/synth_macro_*.go` into the only Synth-tab path. The stage data model (`synthSection`, `synthStageEnableState`, `applySynthStageToggle`) is kept as the source of truth for stages; only the chip-strip UI and the stage detail pane go. Every new surface is built from existing components: `Button`/`NewSpecButton` published via `instEditorBtns`→`synthTabHitAreas`, `ControlGrid` for strips and cards, `synthStageThumbs` for pill thumbnails, `openActionMenu` for the off-stage menu, `TraceSprite` + the concept renderers + `animation_helpers.go` for the graph.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten stub test path (`-tags test -modfile=go.test.mod`), screenshot binary (`/tmp/beatmo_screenshot`, xvfb), i18n catalogs, DESIGN.md token generator.

**Spec:** `docs/superpowers/specs/2026-09-05-synth-tab-overview-and-signal-graph-design.md`

## Global Constraints

- NEVER `git add` / `git commit` (user's standing rule). Leave every file in the working tree.
- Go binary: `.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- No change to `internal/audio` schemas, ABI, goldens, export or prefs. Every param write goes through `audio.SetInstrumentParam`.
- Reuse only: no new hit adapters (use `buttonHitAdapter` via `instEditorBtns`, or the existing macro adapters), no new grid, no new zone, no new renderer family, no new easing code.
- Forbidden glyphs in `internal/ui/` source: `▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←` (use `DrawIcon`). Corner radius by ROLE (`RadiusControl` / `RadiusContainer`). No new inline `color.RGBA{}` literals (token discipline ratchet).
- i18n: adding a key = 4 edits (const in `keys.go`, `AllKeys()`, `catalog_en.go`, `catalog_es.go`). `TestCatalogCompleteness` guards it.
- Every UI task ends with a screenshot that is opened and inspected (desktop, and `MOBILE=1` where the surface differs). Command: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd && cd ../.. && xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; mobile via `make screenshots-all SCENES=<name> MOBILE=1` (output `screenshots/all/mobile/<name>.browser.png`).
- Budgets that must stay green at the end: `synth_drawcall_budget_test.go`, `eq_panel_draw_alloc_discipline_test.go` (re-baseline with a one-line rationale only when the new surface genuinely adds draws), `token_discipline_test.go`, `zone_input_isolation_discipline_test.go`, `TestNoPillShapedControls`.

---

## File map

| File | Responsibility |
|---|---|
| `internal/ui/synth_macro_groups.go` | category tables (melodic + percussion), `Stages`, drum-family params, i18n label/gloss keys, `buildMacroGroups` for every recipe, `categoryEssentials` |
| `internal/ui/synth_macro_nav.go` | drill target `{kind, cat, stage}`, More-open state |
| `internal/ui/synth_macro_render.go` | root cards (+gauges row), drilled header (Back, title, gloss, stage pill), essentials + More layout, hit areas |
| `internal/ui/synth_pipeline_strip.go` (new) | active-stage list, pill Buttons, thumbs, `+` menu, layout + draw |
| `internal/ui/synth_signal_graph.go` (new) | graph state, curve hook, morph, play-through, `onInstrumentTriggered` |
| `internal/ui/synth_concept_viz.go` | `synthCurveHook` seam inside `conceptDrawFilledCurve`; `conceptOsc`/`conceptEnvelope` main curves routed through the seam |
| `internal/ui/synth_panel_zone.go` | header rewrite; legacy branches removed |
| `internal/ui/game_highlight_state.go` | trigger sink call |
| `internal/i18n/{keys,catalog_en,catalog_es}.go` | category keys, More, root title, add-stage |
| `DESIGN.md` | `synth-graph-morph` animation; `synthGaugeH` density token |
| `internal/ui/scene_catalog.go` | new scenes; legacy stage scenes removed |
| `CLAUDE.md` | Synth-tab row of the per-tab map |

---

### Task 1: Category tables cover every recipe

**Files:**
- Modify: `src/go/internal/ui/synth_macro_groups.go`
- Modify: `src/go/internal/ui/synth_panel_zone.go:1334-1360` (gate)
- Modify: `src/go/internal/ui/drumview_ctor.go` (drop `synthMacroEnabled` init)
- Modify: `src/go/internal/ui/drumview.go:505-510` (drop the field)
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go`
- Test: `src/go/internal/ui/synth_macro_discipline_test.go`

**Interfaces:**
- Produces: `macroGroupDef.Stages []synthSectionID`, `macroGroup.Stages`, `macroGroup.Essentials []string` (subset of `Params` whose `Tier != audio.TierAdvanced`, in table order), `buildMacroGroups(schema, merged)` returning non-nil for every recipe with a schema, `macroGroupLabel(id) string`, `macroGroupGloss(id) string` (i18n), `synthCategoryTableFor(schema) []macroGroupDef`.

- [x] **Step 1: Write the failing discipline test**

```go
// synth_macro_discipline_test.go (append)
func TestMacroGroups_EveryRecipeHasCategories(t *testing.T) {
	for id, reg := range audio.RecipeRegistrations() {
		if reg == nil || len(reg.Params) == 0 {
			continue
		}
		merged := audio.RecipeDefaultParams(id)
		groups := buildMacroGroups(reg.Params, merged)
		if len(groups) == 0 {
			t.Errorf("recipe %s: no categories", id)
			continue
		}
		claimed := map[string]int{}
		for _, g := range groups {
			if len(g.Essentials) == 0 {
				t.Errorf("recipe %s category %s: no essentials", id, g.ID)
			}
			for _, p := range g.Params {
				claimed[p]++
			}
		}
		kick := schemaUsesKickStage(reg.Params)
		for _, d := range reg.Params {
			if d.Group == audio.SynthHiddenGroup || isSynthEnableParam(d.Name) || d.Name == "gain" || d.Name == "post_enabled" ||
				synthParamHiddenInContext(d, kick, merged["osc_type"]) {
				continue
			}
			if claimed[d.Name] != 1 {
				t.Errorf("recipe %s: param %s claimed %d times (want 1)", id, d.Name, claimed[d.Name])
			}
		}
	}
}
```

- [x] **Step 2: Run it, expect failures for drum-*/fm-*/synth-modular/conga/pad/whisper and unclaimed drum params**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMacroGroups_EveryRecipeHasCategories -v`

- [x] **Step 3: Extend the tables**

In `synth_macro_groups.go`:
- Add `Stages []synthSectionID` to `macroGroupDef` and `macroGroup`; add `Essentials []string` to `macroGroup`.
- Melodic table stage mapping: sound→{Osc, FM, Voice}, brightness→{Filter, FilterEnv}, attack→{Envelope}, length→{Envelope}, width→{Ensemble}, humanfeel→{Ensemble}, movement→{LFO}, body→{Resonator}, warmth→{Post}, pitchdrop→{Pitch}, voice→{Formant}, flam→{Burst}.
- Melodic "sound" gains the bespoke FM family knobs: `fm_wave, fm_base_freq, fm_op1_decay, fm_op2_decay, fm_op3_decay, fm_op4_decay`; "pitchdrop" gains `fm_pitch_env_amount, fm_pitch_env_decay` (active when either pitchenv is on OR `fm_pitch_env_amount != 0`); "brightness" gains generic `brightness, tone`; "warmth" gains generic `drive` (already) ; "length" gains generic `decay`; "sound" gains generic `pitch, body`. Generic-group names on FM/modular recipes: `pitch`, `decay`, `tone`, `drive`, `brightness`, `body`.
- Percussion table stage mapping: punch→{Voice, Burst, Envelope}, boom→{Voice, Resonator}, pitchdrop→{Pitch, Voice}, tone→{Filter, Voice}, length→{Envelope, Voice}, source→{Osc, FM, Voice}, grit→{Post}.
- Percussion table gains the drum-family params:
  - punch: `snare_attack, bass_attack, kick_click, kick_noise, tom_stick, cym_env_fast, gen1_kick_attack, gen1_kick_click`
  - boom: `pitch, body, fundamental, snare_tone2_freq, snare_tone_mix, tom_o1_gain, tom_o2_gain, tom_room, bass_harmonic, kick_h2_gain, kick_h3_gain, kick_h4_gain, cym_tune, cym_tone_mix, gen1_kick_h2, gen1_kick_h3, gen1_kick_h4, gen1_kick_mode_detune, gen1_kick_mode_gain, gen1_kick_noise`
  - pitchdrop: `kick_pitch_env_amount, kick_pitch_env_rate, bass_pitch_env, tom_sweep_rate, gen1_kick_pe_amt, gen1_kick_pe_rate` (activity predicate ORs any of these being non-zero in merged)
  - tone: `tone, brightness, snare_noise_tune, snare_noise_mix, snare_wire_mix, cym_noise_mix`
  - length: `decay, snare_tone_decay, snare_noise_decay, snare_tail_decay, tom_ring_rate, cym_env_tail, cym_noise_decay, bass_env_rate, kick_env0_rate, kick_env1_rate, gen1_kick_env0, gen1_kick_env1, gen1_kick_fade, gen1_kick_mode_decay, gen1_kick_reverb`
  - source: `snare_wave, cym_wave, tom_wave, bass_wave, kick_wave, gen1_kick_variant`
  - grit: `drive, gen1_kick_sat`
- `buildMacroGroups`: table pick = `schemaUsesKickStage(schema) || isPercussionSchema(schema)` where `isPercussionSchema` is true when the schema contains any of `snare_wave, cym_wave, cym_tune, tom_wave, bass_wave, kick_wave` (bespoke drum families). Fill `Essentials` from `present[name].Tier != audio.TierAdvanced`. Drop the `active` check when the group would otherwise be the ONLY group (never return an empty list).
- Labels/glosses: `macroGroupLabel(id)`/`macroGroupGloss(id)` read i18n keys `KeySynthCat<Id>` / `KeySynthCat<Id>Gloss` (17 ids: sound, brightness, attack, length, width, humanfeel, movement, body, warmth, pitchdrop, voice, flam, punch, boom, tone, source, grit). Add EN (existing English literals) + ES strings. `macroGroup.Label/Gloss` are filled from these at build time.
- In `synth_panel_zone.go` replace the gate with `macros = buildMacroGroups(schema, genMerged)` unconditionally (the WAV case already returned earlier). Delete `synthMacroEnabled` everywhere (`drumview.go`, `drumview_ctor.go`, tests set it — remove those lines in Task 11).

- [x] **Step 4: Run the discipline test + existing macro tests**

Run: `... -run 'TestMacroGroups|TestSynthMacro|TestBuildMacroGroups|TestCatalogCompleteness' ./internal/ui/ ./internal/i18n/` — expect PASS.

- [x] **Step 5: Screenshot** `crop_synth_tab_drum` (snare) and `crop_synth_tab_fm` — both must now show category cards. Inspect.

---

### Task 2: Sampler-style header

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`synthHeaderLayout`, `layoutSynthHeader`, `drawSynthHeader`, `synthHeaderCaptionText`, delete `drawSynthHeaderWaveform`, `synthHeaderWaveFloat64`, `drawSynthWaveformPlaceholder`, `testSynthHeaderSampleOverride`, `minSynthHeaderSamples`, `synthHeaderWaveScratch`)
- Modify: `src/go/internal/i18n/*` (add `KeyCapSynthTitle = "cap.synth_title"`: EN "SYNTH", ES "SÍNTESIS")
- Test: `src/go/internal/ui/synth_header_layout_test.go` (rewrite), delete `synth_header_caption_metric_test.go` if it only tests the caption truncation.

**Interfaces:**
- Produces: `synthHeaderLayout{rect, previewRect, saveRect, saveAsRect, resetRect image.Rectangle; instLabel string}`; `synthHeaderTitle(instID) string` = `i18n.T(KeyCapSynthTitle) + " · " + audio.InstrumentDisplayName(instID)`.

- [x] **Step 1: Failing test**

```go
func TestSynthHeader_SamplerStyle(t *testing.T) {
	g := newViolinMacroGame(t) // from synth_macro_render_test.go
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.SetActiveChannel("violin")
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	h := g.drum.instEditorHeader
	if h.rect.Empty() { t.Fatal("header rect empty") }
	// Right-aligned action cluster: Reset is the rightmost, Preview the leftmost.
	if !(h.previewRect.Max.X <= h.saveRect.Min.X && h.saveRect.Max.X <= h.saveAsRect.Min.X && h.saveAsRect.Max.X <= h.resetRect.Min.X) {
		t.Fatalf("buttons not ordered Preview<Save<SaveAs<Reset: %v %v %v %v", h.previewRect, h.saveRect, h.saveAsRect, h.resetRect)
	}
	if h.resetRect.Max.X != h.rect.Max.X-SpaceMD { t.Fatalf("Reset not flush right: %v vs %v", h.resetRect, h.rect) }
	if got, want := synthHeaderTitle("violin"), i18n.T(i18n.KeyCapSynthTitle)+" · Violin"; got != want {
		t.Fatalf("title %q want %q", got, want)
	}
}
```

- [x] **Step 2: Run → FAIL** (no `synthHeaderTitle`).

- [x] **Step 3: Implement** — `layoutSynthHeader(contentR, headerH, instID, mobile)`: mobile → empty (unchanged). Desktop: `btnH := dv2.SynthHeaderButtonH` clamped to the band; `by := r.Min.Y + (r.Dy()-btnH)/2`; walk from `rx := r.Max.X - SpaceMD` leftwards placing Reset, Save As, Save, Preview with `labelButtonWidth(synthActionLabel(...))` and `SpaceSM` gaps (mirror `layoutSamplerHeaderButtons`). `drawSynthHeader`: `drawRoundedRect(dst, h.rect, TokenSurface1(), RadiusSM, true)` then `DrawTextStyled(dst, truncCaptionStyled(synthHeaderTitle(h.instLabel), previewRect.Min.X-SpaceMD-(h.rect.Min.X+samplerTitlePad), RoleSectionHeader), h.rect.Min.X+samplerTitlePad, h.rect.Min.Y+(h.rect.Dy()-StyledTextHeight(RoleSectionHeader))/2, RoleSectionHeader, TokenTextPrimary())`. Remove `recipeID` from the struct and the `layoutSynthHeader` signature (update the caller in `buildSynthTab`).

- [x] **Step 4: Run header tests + `TestSynthHeaderActionsAlwaysVisibleNoOverflow` (fix its expectations for the new layout).**

- [x] **Step 5: Screenshot** `crop_synth_macro_root` desktop. Inspect: "SYNTH · Violin", four buttons flush right, no thumbnail.

---

### Task 3: Pipeline strip

**Files:**
- Create: `src/go/internal/ui/synth_pipeline_strip.go`
- Modify: `src/go/internal/ui/drumview.go` (fields), `synth_macro_render.go` (reserve strip band in `layoutSynthMacroLayer`; draw it in `drawSynthMacroLayer`), `synth_panel_zone.go` (`synthTabHitAreas` publishes strip pills)
- Modify: `src/go/internal/i18n/*` (`KeySynthAddStage = "synth.add_stage"`: EN "Add", ES "Añadir")
- Test: `src/go/internal/ui/synth_pipeline_strip_test.go`

**Interfaces:**
- Consumes: `dv.instEditorSections []synthSection` (kept, built in `buildSynthTab`), `p.synthStageEnableState(instID, &sec)`, `dv.lazyStageThumbs().waveFor(i)`, `sectionLabelChipFitted`, `dv.openActionMenu`.
- Produces:
```go
type synthPipePill struct { id synthSectionID; btn *Button; secIdx int; rect image.Rectangle }
// on DrumView:
instEditorPipePills []synthPipePill
instEditorPipeAddBtn *Button
instEditorPipeRect image.Rectangle
func (p *synthPanel) activePipelineStages(instID string) (on []int, off []int) // indices into instEditorSections
func (p *synthPanel) layoutPipelineStrip(band image.Rectangle, instID string, mobile bool)
func (p *synthPanel) drawPipelineStrip(dst *ebiten.Image, instID string)
func (p *synthPanel) pipelineHitAreas(instID string, z int) []HitArea
func pipelineStripHeight(mobile bool, n int, availW int) int
```

- [x] **Step 1: Failing tests**

```go
func TestPipelineStrip_SkipsOffStages(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, "violin") // helper: SetActiveTab(TabSynth)+SetActiveChannel+Layout
	audio.SetInstrumentParam("violin", "lfo_enabled", 0)
	audio.SetInstrumentParam("violin", "burst_enabled", 0)
	layoutSynthFor(t, g, "violin")
	for _, pill := range g.drum.instEditorPipePills {
		if pill.id == synthSectionLFO || pill.id == synthSectionBurst {
			t.Fatalf("off stage %v shown in strip", pill.id)
		}
	}
	on, off := g.drum.synp().activePipelineStages("violin")
	if len(off) == 0 { t.Fatal("expected off stages") }
	// order == unifiedSynthSectionOrder order
	prev := -1
	for _, i := range on {
		if int(g.drum.instEditorSections[i].id) < prev { /* compare via index in unifiedSynthSectionOrder */ }
	}
	if g.drum.instEditorPipeAddBtn == nil || g.drum.instEditorPipeAddBtn.Rect().Empty() { t.Fatal("+ pill missing while stages are off") }
}

func TestPipelineStrip_TapDrillsIntoStage(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, "violin")
	var pill synthPipePill
	for _, p := range g.drum.instEditorPipePills { if p.id == synthSectionFilter { pill = p } }
	if pill.btn == nil { t.Fatal("no FILTER pill") }
	clickAt(t, g, pill.rect.Min.X+2, pill.rect.Min.Y+2) // existing real-input helper in testutil_test.go
	if tgt := g.drum.synthDrillTarget("violin"); tgt.kind != drillStage || tgt.stage != synthSectionFilter {
		t.Fatalf("drill target %+v", tgt)
	}
}
```
(`synthDrillTarget` is produced in Task 5; write the test now, it compiles after Task 5 — or stub `drillStage` handling in Task 3 via `setSynthStageOpen`.)

- [x] **Step 2: Implement** `synth_pipeline_strip.go`:
  - `activePipelineStages`: for each section i: `st := p.synthStageEnableState(instID, &sec)`; on when `st.kind == enableNone || st.kind == enableLocked || st.enabled`; else off.
  - `layoutPipelineStrip`: reuse a `ControlGrid` (`dv.instEditorPipeGrid`, `NewControlGrid(controlGridScrollbarStyle())`, `SetMaxCols(len(on)+1)` desktop / adaptive on mobile) with `cellIdealW = dv2.SynthChipMinW + 16`, `cellH = dv2.SynthChipH`, `hGap = SpaceSM + 10` (room for the chevron), `vGap = SpaceXS`. For each on-stage: `btn := NewSpecButton(sectionLabelChipFitted(id, cellW-2*SpaceXS), ComponentButtonSecondary, func(){ p.setSynthStageOpen(instID, id) ; dv.eqPanelZone.Invalidate() })`; `btn.SetToggled(drilled target == this stage)`; `btn.SetRect(cell)`. `+` pill last cell when `len(off) > 0`: label `"+ " + i18n.T(KeySynthAddStage)`, `OnClick`: `dv.openActionMenu("synth-add-stage", btn.Rect(), items)` with one `actionMenuItem{Label: sectionLabelLocalized(id), OnSelect: func(){ p.applySynthStageToggleByID(instID, id); p.setSynthStageOpen(instID, id) }}` per off stage.
  - `drawPipelineStrip`: for each pill in order: `btn.Draw(dst)`; then the thumb watermark: `if w := thumbs.waveFor(secIdx); len(w) > 1 { drawWaveTrace(dst, w, pill.rect.Inset(3), midY, pill.rect.Dx()-6, WithAlpha(colWaveTrace, AlphaFaint), 1.0, nil) }`; chevron between same-row neighbours exactly as the old `drawSynthChipStripContent` did (`DrawIcon(dst, IconChevronRight, arrowR, WithAlpha(colAccent, AlphaMedium))`). Then `addBtn.Draw`.
  - `pipelineHitAreas`: `HitArea{Rect: pill.rect, ZIndex: z+1, Handler: &buttonHitAdapter{btn: pill.btn}, Tag: "synth-pipe-" + sectionLabel(pill.id), Touch: true, ClipRect: pill.rect.Inset(-4)}` + the `+` pill with tag `"synth-pipe-add"`.
  - `pipelineStripHeight`: desktop `dv2.SynthChipStripH`; mobile rows = ceil((n+1) / colsThatFit) × (`SynthChipH`+`SpaceXS`) + `SpaceXS`.
  - Stage thumbs: `synthStageThumbs` is indexed by chip index today; re-key it by section index (`dv.instEditorSections[i]`) — update `synthStageDownstreamOverride(chipIdx)` to take a section index and read `dv.instEditorSections` (it read `instEditorChips` before).
- [x] **Step 3: Wire** in `layoutSynthMacroLayer`: after computing `inner`, reserve `stripH := pipelineStripHeight(...)` at the top of `detailR` (below the root title), call `p.layoutPipelineStrip(stripBand, resolved, mobile)`, shrink `inner.Min.Y`. Root only — when drilled the strip is hidden (`instEditorPipePills = [:0]`, `instEditorPipeAddBtn = nil`). Draw in `drawSynthMacroLayer` root branch; publish in `synthTabHitAreas` (inside the macro branch). Keep `ensureStageThumbs(resolved)` call in `drawSynthTab` (move it before the macro draw).
- [x] **Step 4: Run tests; screenshot** `crop_synth_macro_root` (violin), `crop_synth_macro_perc_root` (kick-808) desktop + mobile. Inspect: pills in order with chevrons and faint thumbs; `+` pill present; no overlap with cards.

---

### Task 4: Root card essentials gauges

**Files:**
- Modify: `src/go/internal/ui/synth_macro_render.go` (`macroCardSlot` gains `gauges []macroCardGauge`; `drawMacroCard` gains the gauges row; root `cellH` grows), `DESIGN.md` densities (`synthGaugeH`: compact 4, comfortable 5, spacious 6) + `make gen-design-tokens`
- Modify: `src/go/internal/i18n/*` (`KeySynthBasicControls = "synth.basic_controls"`: EN "Basic controls", ES "Controles básicos"; replaces the literal in `drawSynthMacroLayer`)
- Test: `src/go/internal/ui/synth_macro_render_test.go`

**Interfaces:**
```go
type macroCardGauge struct { name string; gauge paramCardGauge; enum string }
func (dv *DrumView) macroCardGauges(cg macroGroup) []macroCardGauge // ≤3, from cg.Essentials via instEditorBindings/instEditorControls
func drawMacroCard(dst *ebiten.Image, r image.Rectangle, label, gloss string, gauges []macroCardGauge, graph func(*ebiten.Image, image.Rectangle), st paramCardState)
```

- [x] **Step 1: Failing test** — `TestMacroCard_GaugesRowFromEssentials`: build violin root; the Brightness card slot has 3 gauges named after `synthParamDisplayName("filter_cutoff")`, `("filter_resonance")`, `("filter_type")` (the third has `enum != ""`). And `TestMacroCard_DrawsGaugeRow`: intercept `drawCapsule` (func var? if not, count via `drawRect` interceptor `withDrawRectCapture` used by `synth_card_draw_test.go`) and assert ≥ 2 extra capsule/rect draws inside the gauge band rect.
- [x] **Step 2: Implement** — gauges built in `layoutSynthMacroLayer` root branch (`byName` map of binding index → `paramCardGaugeFor(dv.instEditorControls[idx], def)`; enum label via `knobPillLabel` when `len(def.Enum) > 0`). `drawMacroCard`: after the gloss line, one row of up to 3 cells across `textR.Dx()`: micro name (truncated) then a `synthGaugeH`-tall capsule gauge via `drawParamCardGauge(dst, gr, g.gauge, st.Dim)`; for enum, draw the enum label in `RoleMicro` accent instead of a bar. Root `cellH += StyledTextHeight(RoleMicro) + dv2.SynthGaugeH + SpaceXS`. Make the root grid scroll (it already is a `ControlGrid`; publish a scroll-body hit area for `instEditorMacroGrid` when `HasScroll()`, reusing `synthSectionScrollAdapter`'s shape by adding a `macroGridScrollAdapter{dv}` — NO: reuse `synthSectionScrollAdapter` by giving it an optional `grid *ControlGrid` field so one adapter serves both).
- [x] **Step 3: Tests pass; screenshot** root scenes desktop + mobile. Inspect legibility of the gauge names at each density.

---

### Task 5: Drill target = category OR stage; drilled header with Back / title / gloss / stage pill; essentials + More

**Files:**
- Modify: `src/go/internal/ui/synth_macro_nav.go`, `synth_macro_render.go`, `drumview.go`, `synth_panel_zone.go` (`synthTabHitAreas`: publish stage pill via `synthToggleHitAdapter` when drilled target has exactly one stage; publish More divider Button)
- Modify: `src/go/internal/i18n/*` (`KeySynthMore = "synth.more"`: EN "More", ES "Más"; `KeySynthAdvanced` stays for now, unused after Task 11)
- Test: `synth_macro_nav_test.go`, `synth_macro_render_test.go`

**Interfaces:**
```go
type synthDrillKind int
const ( drillNone synthDrillKind = iota; drillCategory; drillStage )
type synthDrillTarget struct { kind synthDrillKind; cat string; stage synthSectionID }
func (dv *DrumView) synthDrillTarget(instID string) synthDrillTarget
func (dv *DrumView) setSynthMacroOpen(instID, cat string)          // "" = root (kept for scenes/tests)
func (dv *DrumView) setSynthStageOpen(instID string, id synthSectionID)
func (dv *DrumView) synthAtMacroRoot(instID string) bool
func (dv *DrumView) synthMoreOpen(instID string) bool / setSynthMoreOpen(instID string, v bool)
func (dv *DrumView) drilledKnobIdxs(target) (essential, advanced []int)
func (dv *DrumView) drilledStageForPill(target) (secIdx int, ok bool) // stage drill, or category with exactly one stage present in instEditorSections
```

- [x] **Step 1: Failing tests** — `TestSynthDrill_StageTargetListsStageKnobs` (violin, `setSynthStageOpen(inst, synthSectionFilter)` → `drilledKnobIdxs` essentials = filter_type/cutoff/resonance in schema order; advanced empty); `TestSynthDrill_CategoryEssentialsThenMore` (violin Brightness: essentials = cutoff, resonance, type, filtenv_amt, filtenv_decay; advanced = filtenv_attack; More closed → `filtenv_attack` knob rect empty; `setSynthMoreOpen(true)` → non-empty); `TestSynthDrill_HeaderShowsStagePillForSingleStageCategory` (violin Attack → pill rect non-empty, tag `synth-toggle-env_enabled` published; Brightness → no pill).
- [x] **Step 2: Implement** — `instEditorMacroOpen map[string]synthDrillTarget`; `instEditorMoreOpen map[string]bool` keyed `instID + "|" + targetKey`. Drilled layout (`layoutSynthMacroLayer` drilled branch): header band = `macroLayerHeaderH()`; Back = `NewSpecButton(i18n.T(KeyMenuBack), ComponentButtonSecondary, back)` appended to `dv.instEditorBtns` with rect at top-left (drop `macroBackHitAdapter` + `instEditorMacroBackRect`; keep a `dv.instEditorMacroBackBtn *Button` so tests can click it); title + gloss text right of it; stage pill rect at the band's right (reuse `FXToggleTrackW/H` sizing from `synthDetailEnablePillRect`, stored as `dv.instEditorMacroPillRect`). Body: essentials then More: port the two-pass divider code from `placeKnobsInSection` (lines 2140-2185) verbatim into a helper `layoutTieredGrid(grid, innerR, essential, advanced []int, moreOpen bool) (visible []int, dividerRect image.Rectangle)`; when `!moreOpen` the advanced list is not laid out (divider row still present as the expander). The divider is drawn by `drawSynthAdvDivider` (label → `KeySynthMore` + chevron down/up by state) and is a `NewSpecButton("", ComponentButtonSecondary, toggleMore)` in `instEditorBtns` sized to `synthAdvDividerRect` (draw the divider chrome over the button? No: draw ONLY the divider via `drawSynthAdvDivider`, and use the Button purely for input — skip its `Draw` by tagging it `synthMoreButtonTag` in `drawSynthFooter`).
- [x] **Step 3: `synthTabHitAreas`** — drop the else-branch (chips + detail pill) entirely once Task 11 lands; now: inside the macro branch add the stage pill hit area (`synthToggleHitAdapter{dv, sec: id, instID}` when `drilledStageForPill` ok and `st.interactive`).
- [x] **Step 4: Tests; screenshot** new scenes `crop_synth_drilled_category` (violin/brightness), `crop_synth_drilled_stage` (violin/filter via `synthStageSceneSetup(inst, id)`), `crop_synth_drilled_more` (kick-808/boom with More open). Desktop + mobile. Inspect Back pill, title, pill, divider label "More".

---

### Task 6: Signal graph card + curve hook (no animation yet)

**Files:**
- Create: `src/go/internal/ui/synth_signal_graph.go`
- Modify: `synth_concept_viz.go` (`synthCurveHook` seam in `conceptDrawFilledCurve`; `conceptOsc` + `conceptEnvelope` main curves drawn via `conceptDrawFilledCurve`), `synth_focus_graph.go` (`drawSynthFocusGraphContent` selects by drill target instead of `synthSelectedSection`), `synth_panel_zone.go` (`drawSynthTab` right pane: root = full-note only; drilled = graph only; mobile: graph band above the grid), `synth_preview_layer.go` (delete `drawSynthPreviewPane`, `drawSynthMirror` method + func, `splitSynthPreviewMirror`, `synthPreviewMinW` stays)
- Test: `synth_signal_graph_test.go`

**Interfaces:**
```go
// package-level seam consulted by conceptDrawFilledCurve for the PRIMARY curve (fillCol != nil):
var synthCurveHook func(samples []float64, yAt func(float64) int) []float64
type synthSignalGraph struct {
	prev, target, blend []float64
	t        float64          // morph progress, 1 = settled
	yAt      func(float64) int
	lastRect image.Rectangle
	playStart time.Time; playDur time.Duration; playing bool; glowStart time.Time
}
func (dv *DrumView) synthGraph() *synthSignalGraph                       // lazy
func (dv *DrumView) synthGraphControlIdx(instID string) int             // armed control, else first essential of the drilled target, else -1
func (p *synthPanel) drawSynthSignalGraph(dst *ebiten.Image, rect image.Rectangle, instID string)
```

- [x] **Step 1: Failing tests** — `TestCurveHook_SeesPrimaryCurveOnly` (install a hook that records calls; call `conceptFilter` with a ghost differing → exactly one hook call, samples len == rect.Dx()); `TestSignalGraph_DrawsSelectedControl` (violin drilled Brightness, arm cutoff → `synthGraphControlIdx` == cutoff idx; draw through `drawRect` capture → non-zero draws inside the graph rect).
- [x] **Step 2: Implement** — `conceptDrawFilledCurve`: `if fillCol != nil && synthCurveHook != nil { samples = synthCurveHook(samples, yAt) }` right after the length guard. `conceptOsc`: replace `drawSynthOscPlot` with: bg fill via `conceptBgFill`, samples from `synthOscSampleModel` over `w` points, `conceptDrawFilledCurve(dst, r, samples, yAt, midY, colSynthOscFill, WithAlpha(colWaveTrace, AlphaStrong))`, centre line; keep ghost. `conceptEnvelope`: samples = `adsrGhostSamples(a, d, s, rel, w)` with live params (fallback a=0.05,d=0.3,s=0.6,r=0.3 as `drawSynthADSRPlot` did), curve via `conceptDrawFilledCurve(dst, r, samples, yAt, yBot, WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))` — note `amp_curve` bending: apply `adsrCurveShape` inside a local sampler that mirrors `adsrGhostSamples` segment math (write `adsrSamplesCurved(a,d,s,rel,curve float64, w int) []float64`, and make `adsrGhostSamples` call it with curve 0). Keep the "starts/fades" cues. `drawSynthADSRPlot`/`drawSynthOscPlot` are then deleted along with `drawSynthPreviewPane`.
  - `drawSynthSignalGraph`: chrome as `drawSynthFocusGraphContent` (caption = label + `synthControlPurpose`), then `synthCurveHook = g.hook; renderer(img, graphR, instID, def, ghost); synthCurveHook = nil`. In this task `g.hook` just stores `target`+`yAt` and returns samples unchanged. Sprite: reuse `p.sprites.focus` with key `{hash, sel}`.
  - `drawSynthTab`: `if r := dv.instEditorPreviewRect; !r.Empty() { if root { p.drawSynthFullNote(dst, r, ...) } else { p.drawSynthSignalGraph(dst, r, resolved) } }`. Mobile: `layoutSynthMacroLayer` reserves `graphH := 3*dv2.SynthRightCardMinH` at the top of `inner` when drilled (`dv.instEditorMobileFocusRect`), drawn with the same call.
- [x] **Step 3: Tests; screenshot** `crop_synth_drilled_category` desktop + mobile (graph shows the filter curve with caption), `crop_synth_macro_root` (right pane shows "Your sound" only).

---

### Task 7: Morph on edit

**Files:**
- Modify: `DESIGN.md` (`animations: synth-graph-morph: {kind: exp-decay, rate: 0.72, threshold: 0.01}` → `make gen-design-tokens` → `genAnimSynthGraphMorph`), `synth_signal_graph.go`, `synth_panel_zone.go` (`synthTabUpdate` ticks the graph)
- Test: `synth_signal_graph_test.go`

- [x] **Step 1: Failing tests** — `TestSignalGraph_MorphProgressSettles`: `g := &synthSignalGraph{}`; `g.hook([]float64{0,0,0,0}, yAt)` (first call → t=1, returns same); `g.hook([]float64{1,1,1,1}, yAt)` (change → t=0, returns prev); loop `g.tick()` up to 120 times → t reaches 1 and `g.hook([]float64{1,1,1,1}, yAt)` returns exactly the target; assert settle takes between 8 and 20 ticks (≈200 ms at 60 fps). `TestSignalGraph_SpriteKeyStableWhenSettled`: key equality across two frames when `t==1`.
- [x] **Step 2: Implement** — `hook(samples, yAt)`: resample `samples` into `g.scratch` of len(samples) if lengths differ from `g.target` (linear); if `!equalSamples(samples, g.target)`: `g.prev = current blend (or samples on first call)`, `g.target = copy(samples)`, `g.t = 0` (first ever call: t = 1). Then `e := ease(g.t)` (= `g.t*(2-g.t)`, ease-out); fill `g.blend[i] = g.prev[i] + (g.target[i]-g.prev[i])*e`; store `g.yAt = yAt`; return `g.blend`. `tick() bool`: `if g.t >= 1 { return false }; rem := 1 - g.t; rem, ok := DecayStep(rem, genAnimSynthGraphMorph); if !ok { g.t = 1 } else { g.t = 1 - rem }; return true`. Sprite key adds `q := int(g.t * 64)`. `synthTabUpdate`: `if dv.synthGraph().tick() { changed = true }`. All slices are reused (no per-frame alloc once sized).
- [x] **Step 3: Tests; screenshot** `crop_synth_graph_morph_mid` — scene: drill violin/brightness, set cutoff 3 kHz, layout+draw once, set cutoff 800 Hz, layout, tick 4 frames, capture. Inspect: the curve sits between the two shapes.

---

### Task 8: Play-through + trigger sink

**Files:**
- Modify: `synth_signal_graph.go`, `game_highlight_state.go:219` (after `applySequencerHighlight`: `if g.drum != nil && ev.row < len(g.drum.Rows) && g.drum.Rows[ev.row] != nil { g.drum.onInstrumentTriggered(g.drum.Rows[ev.row].Instrument) }`), `synth_panel_zone.go` (`previewActiveSynth` calls `dv.onInstrumentTriggered(resolved)`), `synth_full_note.go` (playhead overlay on the root card), `synth_panel_zone.go` (`synthTabUpdate`: `changed = true` while playing)
- Test: `synth_signal_graph_test.go`, `synth_trigger_sink_test.go`

**Interfaces:**
```go
func (dv *DrumView) onInstrumentTriggered(instID string)   // sync; no-op unless Synth tab active AND resolve(active) == instID
func synthGraphTimeWindowSec(instID string, def audio.ParamDef) (sec float64, timeAxis bool)
func (g *synthSignalGraph) startPlay(now time.Time, dur time.Duration)
func (g *synthSignalGraph) playFrac(now time.Time, windowSec float64) (frac float64, active bool)
func synthNoteDurationSec(instID string) float64 // audio.ConfigForInstrument(instID).DurationSec, min 0.25, max 2
```

- [x] **Step 1: Failing tests** — `TestTriggerSink_NoopWhenTabHidden` (EQ tab active → `onInstrumentTriggered("violin")` leaves `playing == false`); `TestTriggerSink_StartsPlayForActiveInstrument` (Synth tab, violin → `playing == true`, `playDur` == violin duration); `TestTriggerSink_IgnoresOtherInstrument`; `TestTimeWindow_Groups` (env → a+d+0.25+r; lfo → 1; pitchenv → 0.4; filter → timeAxis false); `TestPlayhead_ClearsAfterDuration` (inject `now` func). `TestHighlightDrain_CallsTriggerSink`: push one event on `g.hlCh`, call `g.drainAndDecayHighlights()`, assert graph playing.
- [x] **Step 2: Implement** — graph gets `now func() time.Time` (nil → `time.Now`). Time windows: `env` → ADSR sum (`synthADSRParamsFor`) with 0.25 sustain hold; `lfo` → 1.0; `pitchenv` → 0.4; `burst` → 0.25; FM time-domain knobs (`isFMTimeDomainKnob`) → 0.4; pitch controls (`isSynthPitchControl`) → 0.024; everything else → `timeAxis=false`. Draw (in `drawSynthSignalGraph`, OUTSIDE the sprite, after the blit): if `timeAxis`: `frac, active := g.playFrac(now, window)`; `x := graphR.Min.X + int(frac*float64(graphR.Dx()-1))`; `drawRect(dst, image.Rect(x, graphR.Min.Y, x+1, graphR.Max.Y), WithAlpha(genColorPrimary, AlphaStrong), true)`; dot: `i := int(frac*float64(len(g.blend)-1)); y := g.yAt(g.blend[i]); drawCapsule(dst, image.Rect(x-3, y-3, x+4, y+4), genColorPrimary, true)`. Else glow: `if el := now.Sub(g.glowStart); el < 150ms { a := uint8(float64(AlphaStrong) * (1 - el.Seconds()/0.15)); drawRoundedRect(dst, graphR, WithAlpha(genColorPrimary, a), RadiusControl, false) }`. `playFrac`: `el := now.Sub(playStart).Seconds(); frac = el / windowSec; active = el <= max(windowSec, playDur)`; clamp frac to 1. Root full-note card: same playhead over the trace rect with window = `playDur`. `synthTabUpdate`: `if g.playing || glowing { changed = true }`; when `!active` set `playing=false`.
- [x] **Step 3: Tests; screenshot** `crop_synth_graph_playthrough_mid` (drill violin/attack, `onInstrumentTriggered` with injected now = start+40% of window, capture) and the root with a playhead at 30%. Inspect the line and dot.

---

### Task 9: Mobile pass

**Files:**
- Modify: `synth_macro_render.go` (mobile: 1 column cards; strip wraps; graph band above essentials), `scene_catalog.go` (every new scene gets `MobileSetup` via `mobileMacroSceneSetup` / `mobileSynthStageSceneSetup`)
- Test: `synth_mobile_focus_test.go` (rewrite to the new layout: graph band non-empty when drilled, empty at root)

- [x] **Step 1:** capture `make screenshots-all SCENES=crop_synth_macro_root,crop_synth_macro_perc_root,crop_synth_drilled_category,crop_synth_drilled_stage,crop_synth_drilled_more,crop_synth_graph_morph_mid MOBILE=1`; inspect every PNG; fix overlaps (Back pill vs title, `+` pill wrap, gauge names at Spacious density).
- [x] **Step 2:** tests green.

---

### Task 10: Scenes + CLAUDE.md

**Files:**
- Modify: `scene_catalog.go`: add `crop_synth_root_kick` (dnb-kick), `crop_synth_root_violin`, `crop_synth_root_snare`, `crop_synth_root_fm_bell`, `crop_synth_drilled_category`, `crop_synth_drilled_stage`, `crop_synth_drilled_more`, `crop_synth_graph_morph_mid`, `crop_synth_graph_playthrough_mid`, `crop_synth_header`; helpers `synthStageSceneSetup(inst, id)`, `synthMoreSceneSetup(inst, cat)`, `synthMorphMidSceneSetup`, `synthPlaythroughMidSceneSetup`. Remove the legacy scenes: `crop_synth_tab_kick_voice`, `crop_synth_advanced_divider`, `crop_synth_tab_violin_osc`, `crop_synth_tab_chips_collapsed`, `crop_synth_control_viz`, `crop_synth_tab_modular_play`, `crop_synth_tab_modular_noise`, `crop_synth_tab_drum_generator`, `crop_synth_tab_drum_revoiced`, `crop_synth_mirror`, `crop_synth_focus_*` (re-point any that document a still-existing behaviour at the stage drill instead of deleting).
- Modify: `CLAUDE.md` Synth-tab row of the per-tab map; `scripts/basic_scenes.txt` if it lists removed scenes.
- [x] Run `TestSceneCropEntriesDeclareSubject`, `TestSceneCropSubjectsResolveToVisibleRect`; capture all `crop_synth_*` desktop + mobile; inspect.

---

### Task 11: Legacy deletion + test migration

**Files:**
- Modify: `synth_panel_zone.go` — delete: `synthChip`, `instEditorChips` users, `layoutSynthSections`, `appendSynthChip`, `synthChipWrapRows`, `synthChipIdealWidth`, `placeKnobsInSection`, `growArmedCardLegacy`, `chipStripBounds`, `chipEnableHash`, `drawSynthChipStrip*`, `instEditorchipPrev`, `drawSynthDetailPane`, `synthDetailEnablePillRect`, `synthSelectedSection` (replace by drill target), `selectedSynthSection`/`setSelectedSynthSection`/`instEditorSelectedSection` (keep only if the drill target does not cover the use), `SelectSynthSectionByLabel`, `SynthTabChips`, `synthChipHitAdapter`, the `else` branch in `synthTabHitAreas`, the legacy branch after the macro early-return in `buildSynthTab` (keep the `sections` build + prune → `dv.instEditorSections`).
- Delete files: `synth_stage_thumbs.go` stays; `synth_preview_layer.go` keeps only `synthPreviewWidth`, `autoZoom*`, `mirrorDisplayWindow`, `mirrorTraceGain`, `synthOscSample*`, `synthOscParamsFor`, `adsrCurveShape`, `synthADSRParamsFor`, `synthFilterParamsFor`, `synthVoiceExposesParam`; `synth_chip_strip_*_test.go`, `synth_advanced_divider_test.go`, `synth_unified_sections_test.go` (keep the section ORDER assertions by re-targeting at `instEditorSections`), `synth_disabled_stage_interactivity_test.go` (re-target at stage drill).
- Modify `drumview.go`: remove `instEditorChips`, `instEditorDetailR`, `instEditorDetailHeaderH`, `instEditorReadoutRects` (if unused), `instEditorSectionGrids` (replace by the single macro grid), `synthMacroEnabled`.
- [x] **Step 1:** delete, build (`go vet -tags test ./internal/ui/`), then run the whole ui package: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"`. For each failing file in the 34-file list: re-target at the drill paths (stage semantics tests → `setSynthStageOpen`), or delete when the behaviour no longer exists. Re-baseline `synth_drawcall_budget_test.go` / `eq_panel_draw_alloc_discipline_test.go` `TabSynth` with a rationale line citing the pipeline strip + gauges.
- [x] **Step 2:** `go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ ./internal/i18n/` green; `TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot` green.
- [x] **Step 3:** final screenshot sweep: `make screenshots-all SCENES=$(all crop_synth_*)` desktop and `MOBILE=1`; open and inspect each.

---

## Self-review notes

- Spec §1.2 strip / §1.3 gauges / §1.4 root pane → Tasks 3, 4, 6. §2.1–2.3 → Tasks 5, 6. §2.4 → Tasks 7, 8. §3 → Task 2. §4 → Task 1. §6 → Task 9. §7 → each task's tests + Task 10. §8 → Task 11.
- Names used across tasks: `synthDrillTarget`, `setSynthStageOpen`, `drilledKnobIdxs`, `instEditorPipePills`, `synthCurveHook`, `synthSignalGraph.hook/tick/startPlay/playFrac`, `onInstrumentTriggered`, `macroCardGauge`, `layoutTieredGrid` — defined once, referenced consistently.
