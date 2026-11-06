# Synth Macro Layer — Percussion Set + Per-Card Graphs Implementation Plan

> **STATUS: COMPLETE (2026-07-22).** All 5 tasks implemented + screenshot-verified (desktop `crop_synth_macro_{root,drilled,perc_root,perc_drilled}`, mobile via `make screenshots-all MOBILE=1`). Full `internal/ui` + `internal/audio` fast suite green; audio Golden/Recipe/Wired real-build unchanged. Deviations from plan as written: (a) the "More" reachability bucket in Task 4 was intentionally DROPPED — on kicks it would surface 100+ `gen*` partials; replaced by a `TestSynthMacro_NoEmptyDrill` guard (every root concept drills into ≥1 bound knob). (b) Per-card graph (Task 3) is a HORIZONTAL right-band, not a bottom band — the ~122px desktop panel only fits 2 short card rows, so a bottom graph hid half the concepts. (c) `newKickMacroGame` uses the shipped `kick-808` id (no rebind) to avoid a binding leak.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This continues `2026-07-22-synth-drilldown-macro-ui.md` (Tasks 1–4 of that plan are DONE + screenshot-verified). This is a central-UI feature — every task that changes rendering ends with a **screenshot verification** step (green Go tests are NOT sufficient for UI per CLAUDE.md).

**Goal:** Make the beginner "macro" drill-down layer (a) appear for **percussion/kick** rows too (today it is melodic-only, so the common default drum rows show nothing — the reported "I see nothing new"), and (b) render a **distinct per-card graph** on each concept card (today cards are label+gloss only), plus close the remaining Task 5/6 follow-ups (reachability, guardrail test, mobile scenes, doc refresh).

**Architecture:** Pure **UI-only lens** over existing `ParamDef`s — NO engine/ABI/golden/export change; every leaf still writes through `audio.SetInstrumentParam`. Extend the existing `synth_macro_groups.go` model: add a percussion concept table selected by `schemaUsesKickStage(schema)`, and pass `kickActive` through `buildMacroGroups`. Widen the concept-card cell and route `macroGraphKind` → the existing `synth_concept_viz.go` renderers so `drawMacroCard` paints a graph. Add a trailing "More" bucket so no visible param is unreachable.

**Tech Stack:** Go 1.26, Ebitengine 2.9, `internal/ui` synth panel (`synthPanel`), `internal/audio` recipe schema. Screenshot harness: `scene_catalog.go` + the `./cmd` screenshot binary.

## Global Constraints

- **UI-only lens:** writes go through the existing `audio.SetInstrumentParam` per leaf; NO new persisted state, NO change to `RecipeParams` / recipe hash / synth goldens / JSON export / plugin ABI. (verbatim from the parent proposal spec)
- **Bundled Go:** `.tools/go/bin/go`. Fast tests from `src/go`: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <RE> -count=1`.
- **Screenshots mandatory for UI:** `CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` then `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; **open and inspect the PNG**.
- **Discrete pickers stay pickers** (never fused into a continuous macro): `osc_type, body_model, fm_algorithm, filter_type, lfo_target, formant_vowel, formant_voice_type`.
- **Never stage or commit** (repo convention): leave all work in the working tree.
- **Percussion reality (verified 2026-07-22):** modular kicks (`synth-modular-kick-808`, `…-acoustic/-dnb/-electro/-punchy/-raw/-zgump`) do **NOT** contain `kick_attack`/`kick_env0`-style params — those belong to the retired configurable-kick. Modular kicks carry the SAME full modular schema as melodic recipes PLUS `kick_enabled=1`, the kick **`post_*`** shaping stage (`post_pitch/post_decay/post_decay_rate/post_body/post_brightness/post_tone/post_drive`), and `voice_freq_hz` (fundamental). The percussion concept table MUST be built from THESE real param names, not the parent plan's fictional list.
- **`synthParamHiddenInContext` does NOT hide by stage-enable flag** — it hides only `SynthHiddenGroup`, kick-voice knobs on non-kick recipes, and physical-model osc knobs for the wrong `osc_type`. So a concept whose params live in a disabled stage still renders; gate a whole concept via the `active func(RecipeParams) bool` predicate on its `macroGroupDef`.

---

## Existing symbols this plan builds on (already in the tree)

- `macroGraphKind` + consts `macroGraphOsc/Filter/Attack/Decay/Spread/Wavy/Bumps/Softclip/Pitch/Level` (`synth_macro_groups.go`).
- `type macroGroup struct{ ID,Label,Gloss string; Graph macroGraphKind; Params []string }`, `type macroGroupDef struct{ …; active func(audio.RecipeParams) bool }`.
- `enableOn(name) func(audio.RecipeParams) bool`, `voicesGT1`, `bodyModelOn`.
- `func buildMacroGroups(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup` — currently returns `nil` when `schemaUsesKickStage(schema)`.
- `schemaUsesKickStage(schema []audio.ParamDef) bool` (`synth_panel_zone.go:453`, `gen1_source default==5`).
- `synthParamHiddenInContext(def, kickActive, oscType)`, `isSynthEnableParam(name)`.
- Wiring gate `synth_panel_zone.go:1333-1347`: `if dv.synthMacroEnabled && audio.IsMelodicRecipe(recipeID) { macros = buildMacroGroups(schema, genMerged) }`.
- Render: `drawMacroCard(dst, r, label, gloss string, st paramCardState)` (`synth_macro_render.go:327`), `layoutSynthMacroLayer` (`:39`, root `cellH` at `:77`), `drawSynthMacroLayer` (`:154`).
- Concept-viz renderers (`synth_concept_viz.go`), signature `func(dst *ebiten.Image, r image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64)`: `conceptOsc`(:220), `conceptFilter`(:423), `conceptEnvelope`(:272), `conceptMotion`(:781), `conceptLFO`(:1078), `conceptPitchEnvSweep`(:1107), `conceptBurst`(:932), `conceptPost`(:1156), `conceptValueBar`(:1277), `conceptFM`(:592).
- Scenes: `synthTabSceneSetup(instrumentID)` sets `Rows[0].Instrument`; existing `crop_synth_macro_root` / `crop_synth_macro_drilled` set `g.drum.synthMacroEnabled = true` then `LayoutZoneNow("eq-panel")`.

---

## Task 1: Percussion concept table + `buildMacroGroups` kick branch (pure logic, TDD)

**Files:**
- Modify: `src/go/internal/ui/synth_macro_groups.go` (add `macroGroupDefsPercussion`; branch `buildMacroGroups` on `schemaUsesKickStage`; pass `kickActive` through)
- Test: `src/go/internal/ui/synth_macro_groups_test.go` (extend existing)

**Interfaces:**
- Consumes: `schemaUsesKickStage`, `synthParamHiddenInContext`, `isSynthEnableParam`, `enableOn`, `audio.RecipeRegistrations`.
- Produces: `var macroGroupDefsPercussion []macroGroupDef`; `buildMacroGroups` now returns non-nil for kick recipes.

- [ ] **Step 1: Write failing tests** — append to `synth_macro_groups_test.go`:

```go
func TestBuildMacroGroups_Percussion_Kick808(t *testing.T) {
	reg, ok := audio.RecipeRegistrations()["synth-modular-kick-808"]
	if !ok {
		t.Fatal("kick-808 recipe missing")
	}
	merged := audio.RecipeDefaultParams("synth-modular-kick-808")
	groups := buildMacroGroups(reg.Params, merged)
	if len(groups) == 0 {
		t.Fatal("percussion kick should now yield macro groups (was nil)")
	}
	ids := map[string]bool{}
	present := map[string]bool{}
	for _, d := range reg.Params {
		present[d.Name] = true
	}
	for _, g := range groups {
		ids[g.ID] = true
		if len(g.Params) == 0 {
			t.Errorf("group %q has zero params", g.ID)
		}
		for _, p := range g.Params {
			if !present[p] {
				t.Errorf("group %q param %q not in kick schema", g.ID, p)
			}
		}
	}
	for _, want := range []string{"punch", "boom", "tone", "length", "source"} {
		if !ids[want] {
			t.Errorf("expected percussion concept %q, got ids %v", want, ids)
		}
	}
	if ids["width"] {
		t.Errorf("kick-808 has one voice; Width concept must be hidden")
	}
}

func TestBuildMacroGroups_Melodic_Unchanged(t *testing.T) {
	reg := audio.RecipeRegistrations()["synth-modular-violin"]
	merged := audio.RecipeDefaultParams("synth-modular-violin")
	groups := buildMacroGroups(reg.Params, merged)
	ids := map[string]bool{}
	for _, g := range groups {
		ids[g.ID] = true
	}
	for _, want := range []string{"sound", "brightness", "attack", "length"} {
		if !ids[want] {
			t.Errorf("melodic set regressed: missing %q (got %v)", want, ids)
		}
	}
	if ids["punch"] || ids["boom"] {
		t.Errorf("melodic instrument must not get percussion concepts: %v", ids)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (kick returns nil today; `punch`/`boom` undefined).
  Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestBuildMacroGroups' -count=1`
  Expected: FAIL — `percussion kick should now yield macro groups (was nil)`.

- [ ] **Step 3: Implement.** In `synth_macro_groups.go`, add the percussion table above `buildMacroGroups`. Every param name below is verified present in the kick-808 schema:

```go
// macroGroupDefsPercussion is the ordered concept table for kick/percussion
// recipes (schemaUsesKickStage). Modular kicks share the full modular schema plus
// the kick `post_*` shaping stage + voice_freq_hz fundamental — this table maps
// those REAL params to drum-friendly concept names. Ordering = what a beatmaker
// reaches for first (transient → body → tone → tail → engine → grit).
var macroGroupDefsPercussion = []macroGroupDef{
	{ID: "punch", Label: "Punch", Gloss: "A soft thump, or a sharp crack?", Graph: macroGraphAttack, Params: []string{
		"amp_attack", "burst_sharp", "burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp",
	}},
	{ID: "boom", Label: "Boom", Gloss: "How low and heavy the body is.", Graph: macroGraphBumps, Params: []string{
		"voice_freq_hz", "post_body", "body_model", "body_mix",
	}},
	{ID: "pitchdrop", Label: "Pitch Drop", Gloss: "A downward pitch punch, like an 808.", Graph: macroGraphPitch, Params: []string{
		"pitchenv_amt", "pitchenv_decay", "post_pitch", "post_decay_rate",
	}, active: pitchDropActive},
	{ID: "tone", Label: "Tone", Gloss: "Dull and dark, or bright and clicky?", Graph: macroGraphFilter, Params: []string{
		"filter_cutoff", "filter_resonance", "filter_type", "post_brightness", "post_tone",
	}},
	{ID: "length", Label: "Length", Gloss: "A tight tap, or a long tail?", Graph: macroGraphDecay, Params: []string{
		"amp_decay", "amp_sustain", "amp_release", "amp_curve", "post_decay",
	}},
	{ID: "source", Label: "Source", Gloss: "The engine that makes the tone.", Graph: macroGraphOsc, Params: []string{
		"osc_type", "osc_octave", "osc_detune", "pitch",
		"fm_algorithm", "fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
		"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
		"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level",
	}},
	{ID: "grit", Label: "Grit", Gloss: "Clean, or warm and distorted?", Graph: macroGraphSoftclip, Params: []string{
		"drive", "post_drive",
	}},
}

// pitchDropActive shows the Pitch Drop concept when the recipe actually sweeps
// pitch: either the pitch-env stage is on, or the kick post-stage pitch shaping
// is engaged (post_pitch != 0). Kicks with neither (a flat click) hide it.
func pitchDropActive(m audio.RecipeParams) bool {
	return m["pitchenv_enabled"] >= 0.5 || m["post_pitch"] > 0.001 || m["post_pitch"] < -0.001
}
```

  Then refactor `buildMacroGroups` to branch and thread `kickActive`:

```go
func buildMacroGroups(schema []audio.ParamDef, merged audio.RecipeParams) []macroGroup {
	kickActive := schemaUsesKickStage(schema)
	defs := macroGroupDefsMelodic
	if kickActive {
		defs = macroGroupDefsPercussion
	}
	present := make(map[string]audio.ParamDef, len(schema))
	for _, d := range schema {
		present[d.Name] = d
	}
	oscType := merged["osc_type"]

	var out []macroGroup
	for _, def := range defs {
		var ps []string
		for _, name := range def.Params {
			d, ok := present[name]
			if !ok {
				continue
			}
			if isSynthEnableParam(name) || synthParamHiddenInContext(d, kickActive, oscType) {
				continue
			}
			ps = append(ps, name)
		}
		if len(ps) == 0 {
			continue
		}
		if def.active != nil && !def.active(merged) {
			continue
		}
		out = append(out, macroGroup{ID: def.ID, Label: def.Label, Gloss: def.Gloss, Graph: def.Graph, Params: ps})
	}
	return out
}
```

  Delete the old top-of-function early-return (`if schemaUsesKickStage(schema) { return nil }`) and the `const kickActive = false`.

- [ ] **Step 4: Run — expect PASS.** Same command as Step 2, plus `-run 'TestBuildMacroGroups|TestConceptGroups|Macro'`.

- [ ] **Step 5: Update the file header comment** in `synth_macro_groups.go` — replace the "v1 covers MELODIC … Percussion/kick recipes return nil" paragraph with a one-liner noting both melodic + percussion tables now exist, selected by `schemaUsesKickStage`.

## Task 2: Wire percussion recipes into the panel gate + scenes + screenshots

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:1333-1336` (the macro gate)
- Modify: `src/go/internal/ui/scene_catalog.go` (add two percussion crop scenes)
- Test: `src/go/internal/ui/synth_macro_render_test.go` OR extend an existing macro test (functional: drive `buildSynthTab` for a kick and assert `dv.instEditorMacros != nil`)

**Interfaces:**
- Consumes: `buildMacroGroups` (Task 1), `schemaUsesKickStage`, `audio.IsMelodicRecipe`.
- Produces: kick rows now populate `dv.instEditorMacros`; scenes `crop_synth_macro_perc_root`, `crop_synth_macro_perc_drilled`.

- [ ] **Step 1: Change the gate** at `synth_panel_zone.go:1333-1336` from:

```go
	var macros []macroGroup
	if dv.synthMacroEnabled && audio.IsMelodicRecipe(recipeID) {
		macros = buildMacroGroups(schema, genMerged)
	}
```

  to:

```go
	var macros []macroGroup
	if dv.synthMacroEnabled && (audio.IsMelodicRecipe(recipeID) || schemaUsesKickStage(schema)) {
		macros = buildMacroGroups(schema, genMerged)
	}
```

  Update the adjacent comment (`synth_panel_zone.go:1328-1332`): the macro layer now covers melodic AND percussion; only WAV/sample instruments (no recipe) fall through to the legacy stage path.

- [ ] **Step 2: Write a failing functional test** in `synth_macro_render_test.go`:

```go
func TestSynthMacro_KickPopulatesMacroLayer(t *testing.T) {
	g := newTestGame(t) // existing synth-test ctor; see synth_macro_render.go peers / synth_panel_*_test.go
	dv := g.drum
	dv.synthMacroEnabled = true
	dv.Rows[0].Instrument = "synth-modular-kick-808"
	_ = g.SetActiveEQTab("synth")
	dv.eqPanelZone.SetActiveChannel("synth-modular-kick-808")
	dv.audioTree.LayoutZoneNow("eq-panel")
	if dv.instEditorMacros == nil {
		t.Fatal("kick row should activate the macro layer, got nil instEditorMacros")
	}
	ids := map[string]bool{}
	for _, m := range dv.instEditorMacros {
		ids[m.ID] = true
	}
	if !ids["punch"] || !ids["boom"] {
		t.Errorf("kick macro root should include punch+boom, got %v", ids)
	}
}
```

  (Use whatever game/DrumView test constructor the existing macro/synth tests use — grep `func newTest` / how `synth_macro_nav_test.go` builds a `*DrumView`. Follow the functional-test convention: real `LayoutZoneNow`, not hand-built structs — per `feedback_functional_tests_over_isolation`.)

- [ ] **Step 3: Run — expect FAIL if gate not yet applied; PASS after Step 1.**
  Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthMacro_Kick' -count=1`

- [ ] **Step 4: Add two percussion scenes** to `scene_catalog.go`, mirroring `crop_synth_macro_root`/`_drilled` but with a kick instrument and drilling into `"boom"`:

```go
	{Name: "crop_synth_macro_perc_root", Description: "Synth tab — beginner macro layer at the Basic root for a PERCUSSION instrument (808 kick): Punch/Boom/Tone/Length/Source concept cards",
		Subject: SubjectSynthPanel,
		Setup: func(g *Game) {
			synthTabSceneSetup("synth-modular-kick-808")(g)
			if g.drum != nil {
				g.drum.synthMacroEnabled = true
				if g.drum.audioTree != nil {
					g.drum.audioTree.LayoutZoneNow("eq-panel")
				}
			}
		}},
	{Name: "crop_synth_macro_perc_drilled", Description: "Synth tab — percussion macro layer drilled into Boom: its params + the '◀ Basic' pill",
		Subject: SubjectSynthPanel,
		Setup: func(g *Game) {
			synthTabSceneSetup("synth-modular-kick-808")(g)
			if g.drum != nil {
				g.drum.synthMacroEnabled = true
				inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
				g.drum.setSynthMacroOpen(inst, "boom")
				if g.drum.audioTree != nil {
					g.drum.audioTree.LayoutZoneNow("eq-panel")
				}
			}
		}},
```

  (If `synthTabSceneSetup` rejects the full id, first confirm the exact instrument id the default demo uses for a kick row via `grep -rn "synth-modular-kick" internal/ui/scene_catalog.go` or `-list-scenes`; the override is assigned verbatim to `Rows[0].Instrument`.)

- [ ] **Step 5: Build + capture** both percussion scenes AND re-capture the melodic ones (regression):

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
cd /home/ymolinar/Repos/beatmo
for s in crop_synth_macro_root crop_synth_macro_drilled crop_synth_macro_perc_root crop_synth_macro_perc_drilled; do \
  xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene $s -screenshot /tmp/$s.png; done
```

- [ ] **Step 6: OPEN all four PNGs and inspect.** Percussion root shows Punch/Boom/Tone/Length/Source cards (no Width — one voice); drilled Boom shows `voice_freq_hz`/`post_body`/`body_model`/`body_mix` param cards + "◀ Basic". Melodic ones unchanged. Fix + re-capture until correct.

## Task 3: Per-card concept graphs (Task 5 of the parent plan)

**Files:**
- Modify: `src/go/internal/ui/synth_macro_render.go` (`drawMacroCard` gains a graph; `layoutSynthMacroLayer` root `cellH` grows; `drawSynthMacroLayer` passes a graph callback + the macro's `Graph`/instID)
- Modify: `src/go/internal/ui/synth_macro_groups.go` — nothing (Graph already carried); optionally set each percussion def's `Graph` to the best-fit kind (already set in Task 1)
- Test: `src/go/internal/ui/synth_macro_graph_test.go` (pixel/smoke: a drawn concept card writes non-background ink in its graph band)

**Interfaces:**
- Consumes: the `synth_concept_viz.go` renderers (signature `func(dst, r, instID, def, ghost)`), `conceptMergedParams(instID)`.
- Produces: `func macroGraphRenderer(kind macroGraphKind, instID string) func(*ebiten.Image, image.Rectangle)`; `drawMacroCard(dst, r, label, gloss string, graph func(*ebiten.Image, image.Rectangle), st paramCardState)`.

- [ ] **Step 1: Add the dispatcher** in `synth_macro_render.go`. It maps a `macroGraphKind` to an existing concept-viz renderer, supplying a synthetic headline `audio.ParamDef` where the renderer needs one:

```go
// macroGraphRenderer returns a paint callback that draws the concept's signature
// graph using instID's live params, dispatching to the existing synth_concept_viz
// renderers. Returns nil when a kind has no graph (card falls back to label+gloss).
func macroGraphRenderer(kind macroGraphKind, instID string) func(*ebiten.Image, image.Rectangle) {
	ghost := conceptMergedParams(instID)
	draw := func(fn func(*ebiten.Image, image.Rectangle, string, audio.ParamDef, map[string]float64), name string) func(*ebiten.Image, image.Rectangle) {
		return func(dst *ebiten.Image, r image.Rectangle) {
			fn(dst, r, instID, audio.ParamDef{Name: name}, ghost)
		}
	}
	switch kind {
	case macroGraphOsc:
		return draw(conceptOsc, "osc_type")
	case macroGraphFilter:
		return draw(conceptFilter, "filter_cutoff")
	case macroGraphAttack, macroGraphDecay:
		return draw(conceptEnvelope, "amp_decay")
	case macroGraphSpread, macroGraphWavy:
		return draw(conceptMotion, "unison_detune")
	case macroGraphBumps:
		return draw(conceptFilter, "filter_cutoff") // body/resonance as a spectral hump
	case macroGraphSoftclip:
		return draw(conceptPost, "drive")
	case macroGraphPitch:
		return draw(conceptPitchEnvSweep, "pitchenv_amt")
	default:
		return nil
	}
}
```

  (Verify each renderer tolerates a bare `audio.ParamDef{Name:…}` — they read `instID`+`ghost` for the curve and use `def` mainly for labeling. If one panics on an empty `def`, populate `Min/Max/Default` from `present[name]` threaded in, or drop that kind to `nil`. Confirm at Step 4's screenshot.)

- [ ] **Step 2: Grow the root cell + pass the graph.** In `layoutSynthMacroLayer` root branch (`:77`), change `cellH` to include a graph band:

```go
		cellH := StyledTextHeight(RoleBody) + StyledTextHeight(RoleCaption) + macroCardGraphH() + SpaceSM + SpaceMD
```

  Add `func macroCardGraphH() int { return StyledTextHeight(RoleBody) * 2 }` near `macroLayerHeaderH`. In `drawSynthMacroLayer` root loop (`:171-176`), pass the graph:

```go
		for i, c := range dv.instEditorMacroCards {
			if c.r.Empty() {
				continue
			}
			var gr func(*ebiten.Image, image.Rectangle)
			if i < len(dv.instEditorMacros) {
				gr = macroGraphRenderer(dv.instEditorMacros[i].Graph, resolved)
			}
			drawMacroCard(dst, c.r, c.label, c.gloss, gr, paramCardState{})
		}
```

  (Card slots and `instEditorMacros` share index order — both built in group order — but guard the bound as above.)

- [ ] **Step 3: Update `drawMacroCard`** (`:327`) to take the `graph` param and paint it in a band between the label and the gloss (or below the gloss). Insert after the label/caret draw, before/after the gloss:

```go
	if graph != nil {
		gh := macroCardGraphH()
		gr := image.Rect(inner.Min.X, inner.Max.Y-gh, inner.Max.X, inner.Max.Y)
		if !gr.Empty() {
			graph(dst, gr)
		}
	}
```

  Adjust the gloss `gy` so it sits above the graph band, not overlapping. Update the signature and its call sites (only `drawSynthMacroLayer`; a test may call it too).

- [ ] **Step 4: Write the pixel smoke test** `synth_macro_graph_test.go` (mirror `audio_panel_render_pixels_test.go` style — render a card to an offscreen `*ebiten.Image`, count non-background pixels in the graph band > 0). Run it; expect PASS.
  Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthMacroGraph|Macro' -count=1`

- [ ] **Step 5: Rebuild + re-capture** `crop_synth_macro_root` + `crop_synth_macro_perc_root` (and the two drilled — should be visually unchanged). **OPEN the PNGs:** each concept card shows a distinct small graph (waveform / filter curve / envelope / spread / pitch sweep / soft-clip), no overlap with label or gloss, cells not clipped. Fix + re-capture until correct.

## Task 4: Reachability "More" bucket + guardrail discipline test + full verification

**Files:**
- Modify: `src/go/internal/ui/synth_macro_groups.go` (append a trailing `more` bucket in `buildMacroGroups`)
- Create: `src/go/internal/ui/synth_macro_discipline_test.go`

**Interfaces:**
- Consumes: both concept tables, `audio.RecipeRegistrations`, `synthParamIsGridControlIn`.
- Produces: nothing new public; `buildMacroGroups` output now covers every visible param.

- [ ] **Step 1: Failing reachability test** in `synth_macro_discipline_test.go`:

```go
func TestMacroGroups_EveryVisibleParamReachable(t *testing.T) {
	for _, id := range []string{"synth-modular-violin", "synth-modular-kick-808"} {
		reg := audio.RecipeRegistrations()[id]
		merged := audio.RecipeDefaultParams(id)
		kickActive := schemaUsesKickStage(reg.Params)
		oscType := merged["osc_type"]
		claimed := map[string]bool{}
		for _, g := range buildMacroGroups(reg.Params, merged) {
			for _, p := range g.Params {
				claimed[p] = true
			}
		}
		for _, d := range reg.Params {
			if d.Name == "gain" { // row volume, deliberately excluded
				continue
			}
			if !synthParamIsGridControlIn(d, kickActive, oscType) {
				continue // hidden/enable params have no card by design
			}
			if !claimed[d.Name] {
				t.Errorf("%s: visible param %q is unreachable in the macro layer", id, d.Name)
			}
		}
	}
}

func TestMacroGroups_NoParamNameTypos(t *testing.T) {
	// Every Params entry in both tables must be a real ParamDef on at least one
	// shipped recipe (guards against typos / param renames).
	real := map[string]bool{}
	for _, reg := range audio.RecipeRegistrations() {
		for _, d := range reg.Params {
			real[d.Name] = true
		}
	}
	for _, tbl := range [][]macroGroupDef{macroGroupDefsMelodic, macroGroupDefsPercussion} {
		for _, def := range tbl {
			for _, p := range def.Params {
				if !real[p] {
					t.Errorf("concept %q references unknown param %q", def.ID, p)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL** on reachability (gen*/post*/formant params unclaimed) until the `more` bucket exists.
  Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMacroGroups_' -count=1`

- [ ] **Step 3: Implement the trailing `more` bucket** in `buildMacroGroups`, after the `defs` loop and before `return out`:

```go
	claimed := map[string]bool{}
	for _, g := range out {
		for _, p := range g.Params {
			claimed[p] = true
		}
	}
	var rest []string
	for _, d := range schema {
		if d.Name == "gain" || claimed[d.Name] {
			continue
		}
		if isSynthEnableParam(d.Name) || synthParamHiddenInContext(d, kickActive, oscType) {
			continue
		}
		rest = append(rest, d.Name)
	}
	if len(rest) > 0 {
		out = append(out, macroGroup{ID: "more", Label: "More", Gloss: "Extra fine-tuning controls.", Graph: macroGraphLevel, Params: rest})
	}
```

  Note: `macroGraphLevel` maps to `nil` in `macroGraphRenderer` (Task 3) → the More card is label+gloss only, which is fine.

- [ ] **Step 4: Run — expect PASS** for `TestMacroGroups_*` and re-run Task 1's tests (More must not break `Width`-hidden assertions — it won't; `unison_*` on a 1-voice kick are visible params so they land in More, which is acceptable).

- [ ] **Step 5: Full fast sweep — no regressions:**
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ -count=1
  ```
  Expect PASS (the lens touches no audio path).

- [ ] **Step 6: Confirm ZERO golden/ABI impact** (real-build audio):
  ```bash
  cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go test ./internal/audio/ -run 'Golden|Recipe|Wired' -count=1
  ```
  Expect PASS, unchanged.

- [ ] **Step 7: Re-capture** `crop_synth_macro_perc_root` — the "More" card appears last. **OPEN it.**

## Task 5: Mobile scenes + doc/memory refresh

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` (add `MobileSetup` to all four macro scenes)
- Modify: `docs/superpowers/plans/2026-07-22-synth-drilldown-macro-ui.md` (mark Task 5/6 + percussion done; fix the stale `concept*`→`macro*` naming note) OR add a "superseded by 2026-07-22-synth-macro-percussion-and-graphs.md" banner
- Modify: memory `project_synth_macro_drilldown_ui.md` (percussion set + graphs shipped)
- Modify: `AGENTS.synth.md` (add the macro/percussion concept-layer entry if a Synth-UI section exists)

**Interfaces:** none (docs + scenes).

- [ ] **Step 1:** Add `MobileSetup` to the four macro scenes. Mirror an existing `mobileSynthTab*Setup` helper (e.g. `mobileSynthTabModularPlaySetup`) that exposes the mobile audio panel, then set `synthMacroEnabled = true` + drill state + `LayoutZoneNow("eq-panel")`. Grep `func mobileSynthTab` for the closest template; a Setup-only scene renders a blank panel under `MOBILE=1` (per CLAUDE.md audio-panel pixel-coverage note).

- [ ] **Step 2: Capture the mobile pass:**
  ```bash
  make screenshots-all SCENES=crop_synth_macro_root,crop_synth_macro_perc_root MOBILE=1
  ```
  **OPEN both** — concept cards render 2-up in portrait, labels+glosses+graphs legible, no overflow. Fix + re-capture.

- [ ] **Step 3: Refresh the parent plan doc** — update `2026-07-22-synth-drilldown-macro-ui.md`'s Task 5/6 checkboxes and add a note that the shipped symbols are `macro*` not `concept*`, and that percussion + graphs landed via this follow-up plan.

- [ ] **Step 4: Update memory** `project_synth_macro_drilldown_ui.md`: percussion concept set (`macroGroupDefsPercussion`, gated by `schemaUsesKickStage`, real params not the fictional kick_* list), per-card graphs (`macroGraphRenderer`), trailing `more` bucket, discipline test, mobile scenes. Note the flag is default-on in-app (already true).

- [ ] **Step 5: Final full verification** (the completion gate):
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ -count=1
  ```
  And confirm the four desktop + two mobile PNGs are all inspected and correct. Do NOT claim done without the screenshots (CLAUDE.md UI mandate).

---

## Phase 2 (later — NOT in this plan)
- Macro **drag-adjust** on a concept card (one drag drives all child params via a curve + reverse-mapping + "Custom" flag).
- Nested sub-macros (full recursive depth beyond one level).
- A settings toggle to turn the macro layer off (it is default-on in-app today).
- i18n of every gloss (currently English literals; move to `i18n.Key` + `plainEnglishMap`).
- Resolve the parent plan's noted screenshot-generation drift on the melodic stage/focus scenes (they now render the macro layer in the binary).

## Self-review notes
- **Spec coverage:** percussion set (Task 1-2 — closes the "I see nothing" gap for drum rows), per-card graphs (Task 3 = parent Task 5), reachability + guardrail + full verify (Task 4 = parent Task 6), mobile + docs (Task 5). ✔
- **Percussion param names all verified** against the live `synth-modular-kick-808` schema (dump 2026-07-22): amp_attack, burst_*, voice_freq_hz, post_body/pitch/decay/decay_rate/brightness/tone/drive, body_model/mix, filter_*, amp_*, drive, fm_*, osc_* all present. No fictional `kick_*` params used.
- **Type consistency:** `macroGroupDefsPercussion`, `buildMacroGroups(schema, merged)`, `macroGraphRenderer(kind, instID)`, `drawMacroCard(dst,r,label,gloss,graph,st)`, `macroCardGraphH()`, `pitchDropActive` used consistently across tasks.
- **No placeholders:** every code step shows real code; every render task ends in a screenshot-open step; test param names are schema-verified.
- **Risk:** zero audio-path change (Task 4 Step 6 proves goldens unchanged). Highest-risk step is `drawMacroCard` signature change (Task 3) — one call site + tests; and concept-viz renderers tolerating a bare `def` (mitigated by the Step-1 caveat + screenshot).
