# Synth-tab component render cache (TraceSprite adoption) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the Synth tab's per-frame pixel re-rasterization (the profiled crackle/lag bottleneck) by wrapping its five expensive static-until-change surfaces in the existing `TraceSprite` cache, and PROVE the improvement with before/after measurements.

**Architecture:** Reuse `TraceSprite` (`internal/ui/trace_sprite.go`) verbatim. Each surface's draw becomes `sprite.Draw(dst, rect, key, render)` where `render` is the existing draw code at a `(0,0)`-relative origin and `key` is the state that changes its pixels. Idle → 0 re-renders; drag → 30 Hz (throttled). Hover/highlight stays the external tree overlay it already is — never baked in. Sprites are `Release()`d on tab-hide via the existing chokepoint.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten, `TraceSprite`. Acceptance uses real `main.wasm` (Harness B) + the Go `-tags test` byte/alloc budgets.

## Global Constraints

- **Bundled Go only:** `.tools/go/bin/go`. Absolute `GO=` for browser tests: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- **Never stage or commit** (`feedback_never_stage_or_commit`). Every "checkpoint" means stop for review of the uncommitted diff — no `git add`/`git commit`.
- **Before/after proof is a deliverable** (explicit user ask). Task 1 captures the baseline BEFORE any production change; Task 6 captures after and reports the delta.
- **No `TraceSprite` API changes** — adopt only. API: `Draw(dst *ebiten.Image, rect image.Rectangle, key any, render func(img *ebiten.Image, r image.Rectangle))`, `Invalidate()`, `Release()`, `resident()`. `key` is any comparable; rect SIZE is handled internally (size change forces re-render); 33 ms throttle applies only when `minInterval > 0`.
- **Pixel-identical:** every cached surface must render byte-identical to the direct (`RENDER_SAFE=1`) path — the mandatory gate for each adopter (mirror `TestEQCurveSpriteMatchesDirect`).
- **Hover/highlight is NOT cached and NOT a key input.** Cards/chips implement `glowTarget`/`glowRect`; the tree hover overlay composites glow on top. Only discrete `paramCardState{Selected, Dim}` enters a key. The Sampler's recent `hoverHandle` highlight work is on the sampler waveform (not cached here) and must stay green.
- **Draw bugs are invisible under `play_ui.wasm`** (Update-only) — end-to-end acceptance uses `main.wasm` via Harness B.
- **Never mutate the RO merged-params map** inside a render callback (ghost-clone bite) — clone first if a callback injects values.

**Key facts (verified):**
- `paramCardState = struct{ Selected, Dim bool }` (discrete). `paramCardGauge` is a comparable struct. `drawParamCard(dst, r, name, value string, g paramCardGauge, st paramCardState)` — caller-agnostic, no signature change.
- `synthParamsHash(instID)` is O(1) (`recipeID:instRev:defsRev`, rev-gated) — safe to use in keys.
- Chip strip draws from pre-laid-out `dv.instEditorChips[i].rect` (Layout sets rects; draw is pure) — caching pixels does not affect hit-testing.
- Eviction chokepoint: `(*EQPanelZone).releaseHiddenTabSprites(active)` (`eq_panel_zone.go:547`).

---

## File Structure

- `src/go/internal/ui/synth_sprite_cache.go` — **create**: a `synthSpriteCache` holder (the trace sprites + the per-card map + a `releaseAll()`), owned by `synthPanel`.
- `src/go/internal/ui/synth_panel_zone.go` — **modify**: embed the holder on `synthPanel`; wrap the ParamCard loop (`:2573`), the chip strip (`:2346`).
- `src/go/internal/ui/synth_full_note.go`, `synth_preview_layer.go`, `synth_focus_graph.go` — **modify**: wrap the three trace draws.
- `src/go/internal/ui/eq_panel_zone.go` — **modify**: add `ReleaseSynthSprites func()` to `EQCallbacks`; call it from `releaseHiddenTabSprites` when `active != TabSynth`.
- `src/go/internal/ui/drumview_ctor.go` — **modify**: wire `ReleaseSynthSprites` to `dv.synp().sprites.releaseAll()`.
- `src/go/internal/ui/synth_component_sprite_test.go` — **create**: pixel-identity + render-count tests for all five surfaces.
- `src/go/internal/ui/synth_live_param_edit_alloc_test.go`, `eq_panel_draw_alloc_discipline_test.go` — **modify** (Task 6): re-baseline budgets.

---

### Task 1: Capture the BEFORE baseline (evidence gate — no production code)

Record the current draw cost so Task 6 can prove the delta. No production files change here.

**Files:**
- Create: `bench-results/synth-cache-before.md` (the recorded baseline)

- [ ] **Step 1: Capture the current per-tab alloc + byte budgets**

Run and record the numbers (pass or fail — we want the current state):

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestSynthTab_LiveKnobEditFrameByteBudget' ./internal/ui/ -v 2>&1 | tail -30
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestEQPanelDrawAllocBudget|AllocBudget' ./internal/ui/ -v 2>&1 | tail -40
```

Record in `bench-results/synth-cache-before.md`: the idle & drag KB/frame for `TestSynthTab_LiveKnobEditFrameByteBudget` (PASS/FAIL + measured bytes), and the `perTabAllocBudget[TabSynth]` current value (1198) with the measured allocs/run.

- [ ] **Step 2: Capture the Harness B before-numbers (the money metric)**

Run the real-UI bench (foreground, single run — overlapping runs cause port contention):

```
timeout 560 bash -c 'GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/synth_stage_toggle_ui_bench.browser.test.js' 2>&1 | tail -20
```

Copy `bench-results/synth_stage_toggle_ui.json` to `bench-results/synth_stage_toggle_ui.before.json`. Record the `synth` run's `drawMaxMS`, `maxGapMs`, `deferAbsMsP90`, `fpsAvg` and the `eqControl` run's same fields into `bench-results/synth-cache-before.md` (the eqControl is the floor we're driving the synth run toward).

- [ ] **Step 3: Checkpoint** — stop for review; confirm the baseline file exists with real numbers. Do NOT commit.

---

### Task 2: Synth-sprite holder + the three trace sprites + eviction wiring

The three trace draws (`drawSynthFullNote`, `drawSynthMirror`, `drawSynthFocusGraph`) already take `(dst, rect, …)`, so wrapping is mechanical. Ship these first (biggest bang, lowest risk) together with the holder and eviction.

**Files:**
- Create: `src/go/internal/ui/synth_sprite_cache.go`
- Modify: `src/go/internal/ui/synth_panel_zone.go` (embed holder), `synth_full_note.go`, `synth_preview_layer.go`, `synth_focus_graph.go`, `eq_panel_zone.go`, `drumview_ctor.go`
- Test: `src/go/internal/ui/synth_component_sprite_test.go`

**Interfaces:**
- Produces:
  - `type synthSpriteCache struct { fullNote, mirror, focus TraceSprite; chip TraceSprite; cards map[string]*TraceSprite; cardsSeen map[string]bool }`
  - `func (c *synthSpriteCache) card(id string) *TraceSprite` — lazily create per-paramID card sprite.
  - `func (c *synthSpriteCache) beginCardFrame()` / `func (c *synthSpriteCache) endCardFrame()` — mark-and-sweep eviction of cards not drawn this frame (used in Task 3).
  - `func (c *synthSpriteCache) releaseAll()` — `Release()` every sprite + clear the map.
  - `EQCallbacks.ReleaseSynthSprites func()` consumed by `releaseHiddenTabSprites`.

- [ ] **Step 1: Write the failing pixel-identity test for the focus graph**

Create `src/go/internal/ui/synth_component_sprite_test.go`. Use the existing synth-tab test harness pattern (see `synth_focus_graph_split_test.go` for how to build a `DrumView` with a synth instrument and a content rect). The test renders the focus graph twice — once cached, once with `RENDER_SAFE=1` (set `envRenderSafe` via its test hook, or `t.Setenv("RENDER_SAFE","1")` before constructing) — into two equal images and asserts byte-identical pixels:

```go
func TestSynthFocusGraphSprite_MatchesDirect(t *testing.T) {
	dv, instID, rect := newSynthTabForSprites(t) // helper: builds DrumView+synth inst, returns a valid focus-graph rect
	cached := renderToImage(rect, func(dst *ebiten.Image) { dv.synp().drawSynthFocusGraph(dst, rect, instID) })
	t.Setenv("RENDER_SAFE", "1"); reloadRenderSafe() // envRenderSafe re-read hook
	direct := renderToImage(rect, func(dst *ebiten.Image) { dv.synp().drawSynthFocusGraph(dst, rect, instID) })
	assertImagesEqual(t, cached, direct)
}
```

(If no `renderToImage`/`assertImagesEqual` helper exists, add small local ones using `newTrackedImage` + `img.At`/`ReadPixels`; mirror whatever `TestEQCurveSpriteMatchesDirect` uses — reuse its helpers if present.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSynthFocusGraphSprite_MatchesDirect ./internal/ui/`
Expected: FAIL — `drawSynthFocusGraph` isn't sprite-wrapped yet, so either the helper `newSynthTabForSprites` is undefined (write it first) or (once wrapping is added) the test drives the new path. First run should fail to compile / fail the assertion, proving the test exercises the path.

- [ ] **Step 3: Create the holder**

Create `src/go/internal/ui/synth_sprite_cache.go`:

```go
package ui

// synthSpriteCache holds the Synth tab's component render caches (TraceSprite
// adopters). Owned by synthPanel; Release()d on tab-hide via the EQ panel's
// releaseHiddenTabSprites chokepoint. Not goroutine-safe (UI goroutine only).
type synthSpriteCache struct {
	fullNote TraceSprite
	mirror   TraceSprite
	focus    TraceSprite
	chip     TraceSprite
	cards    map[string]*TraceSprite
	seen     map[string]bool
}

func (c *synthSpriteCache) card(id string) *TraceSprite {
	if c.cards == nil {
		c.cards = map[string]*TraceSprite{}
	}
	s := c.cards[id]
	if s == nil {
		s = &TraceSprite{tag: "synthCard", minInterval: traceSpriteDragInterval}
		c.cards[id] = s
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	c.seen[id] = true
	return s
}

// beginCardFrame / endCardFrame mark-and-sweep evict card sprites whose param
// left the visible set (section switch / scroll), keeping the map bounded.
func (c *synthSpriteCache) beginCardFrame() {
	for k := range c.seen {
		delete(c.seen, k)
	}
}
func (c *synthSpriteCache) endCardFrame() {
	for id, s := range c.cards {
		if !c.seen[id] {
			s.Release()
			delete(c.cards, id)
		}
	}
}

func (c *synthSpriteCache) releaseAll() {
	c.fullNote.Release()
	c.mirror.Release()
	c.focus.Release()
	c.chip.Release()
	for id, s := range c.cards {
		s.Release()
		delete(c.cards, id)
	}
}
```

Set the trace sprites' throttle where they're owned: give `fullNote`, `mirror`, `focus` `minInterval = traceSpriteDragInterval` by initializing them in the `synthPanel` constructor (or lazily set on first Draw — simplest: set the field once when the holder is first used). Embed the holder on `synthPanel`: add field `sprites synthSpriteCache` to the `synthPanel` struct and initialize the trace-sprite throttles in the panel's constructor next to the existing field setup.

- [ ] **Step 4: Wrap the three trace draws**

In each trace function's caller (or the function itself), route through the sprite. Prefer wrapping at the call site inside `drawSynthTab` so the leaf functions stay simple and testable. For the focus graph (method on `synthPanel`), wrap its body:

```go
// synth_focus_graph.go — (*synthPanel).drawSynthFocusGraph
func (p *synthPanel) drawSynthFocusGraph(dst *ebiten.Image, rect image.Rectangle, instID string) {
	key := struct{ hash string; sel int; }{p.synthParamsHash(instID), p.dv.synthSelectedKnobIdx(instID, p.dv.synthSelectedSection())}
	p.sprites.focus.Draw(dst, rect, key, func(img *ebiten.Image, r image.Rectangle) {
		p.drawSynthFocusGraphContent(img, r, instID) // the former body, verbatim, using r not rect
	})
}
```

Split the former body into a `…Content` method that draws relative to the passed `r` (replace `rect.Min` origin uses with `r.Min`). Do the same for `drawSynthFullNote`/`drawSynthMirror` (free functions — add a `…Content` and have the wrapper pass the owning `synthPanel`'s sprite). Keys: mirror `{p.synthParamsHash(instID)}`; full-note `{p.synthParamsHash(instID), ghost bool}`. **Confirm the content functions use only the passed rect for positioning** (no captured absolute rect) — the pixel-identity test in Step 1 is the proof.

- [ ] **Step 5: Wire eviction**

In `eq_panel_zone.go`: add `ReleaseSynthSprites func()` to `EQCallbacks` (near `DrawSynthTab`, `:110`), and in `releaseHiddenTabSprites` add:

```go
	if active != TabSynth && z.callbacks.ReleaseSynthSprites != nil {
		z.callbacks.ReleaseSynthSprites()
	}
```

In `drumview_ctor.go` (near `DrawSynthTab: …`, `:504`) add:

```go
		ReleaseSynthSprites: func() { dv.synp().sprites.releaseAll() },
```

- [ ] **Step 6: Add the render-count test, then make all pass**

Add to `synth_component_sprite_test.go` a render-count test: a counting wrapper (or assert via `resident()` + a re-render counter injected through the render callback) proving the focus/mirror/full-note sprite re-renders 0× across repeated identical-param frames and exactly 1× after a param change (mirror `TestEQCurveSpriteRenderCount`). Add pixel-identity tests for mirror + full-note too.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthFocusGraphSprite|SynthMirrorSprite|SynthFullNoteSprite' ./internal/ui/`
Expected: PASS (pixel-identity + render-count).

- [ ] **Step 7: Verify eviction + no leaks + build**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Synth|EQPanel|TabSwitch' ./internal/ui/` and `GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: PASS + WASM build exit 0. Add a small eviction test: after switching away from the Synth tab, `dv.synp().sprites.focus.resident()` is false.

- [ ] **Step 8: Checkpoint** — stop for review. Do NOT commit.

---

### Task 3: Per-card ParamCard sprites (the caption-alloc win)

Wrap each visible ParamCard in its own sprite keyed on `(name, value, gauge, state)`. Moves per-frame caption formatting into the render callback (runs only on re-render).

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (the card loop at `:2573-2589`)
- Test: `src/go/internal/ui/synth_component_sprite_test.go`

**Interfaces:**
- Consumes: `synthSpriteCache.card(id)`, `beginCardFrame`/`endCardFrame` (Task 2).

- [ ] **Step 1: Write the failing test — only the edited card re-renders**

Add `TestSynthParamCards_OnlyEditedCardReRenders`: build the synth detail pane with ≥2 visible cards, draw two frames identical → assert 0 re-renders; change ONE param's value → assert exactly that card's sprite re-rendered and the others did not (instrument re-render via a counting render callback or a per-card `resident()`+key check). Also add `TestSynthParamCardSprite_MatchesDirect` (cached vs `RENDER_SAFE` byte-identical for one card).

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthParamCard' ./internal/ui/`
Expected: FAIL — cards aren't sprite-wrapped; every frame re-rasterizes.

- [ ] **Step 3: Wrap the card loop**

In `synth_panel_zone.go` `drawSynthDetailPane`, wrap the `for _, kIdx := range sel.knobIdxs` body. Bracket the loop with `p.sprites.beginCardFrame()` / `p.sprites.endCardFrame()`:

```go
	p.sprites.beginCardFrame()
	for _, kIdx := range sel.knobIdxs {
		if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) { continue }
		k := dv.instEditorKnobs[kIdx]
		r := k.Rect()
		if r.Empty() { continue }
		b := dv.instEditorBindings[kIdx]
		st := paramCardState{Selected: dv.synthSelectedKnobIdx(instID, sel) == kIdx, Dim: !stageEnabled}
		name := synthParamDisplayName(b.def.Name)
		value := knobPillLabel(k, b.def)
		gauge := paramCardGaugeFor(k, b.def)
		key := struct {
			name, value string
			gauge       paramCardGauge
			st          paramCardState
		}{name, value, gauge, st}
		p.sprites.card(b.def.Name).Draw(dst, r, key, func(img *ebiten.Image, rr image.Rectangle) {
			drawParamCard(img, rr, name, value, gauge, st)
		})
	}
	p.sprites.endCardFrame()
```

Note: `name`/`value`/`gauge` are captured per-iteration (Go 1.22+ loop-var semantics make this safe; the repo is Go 1.26). The caption strings are still computed each frame to build the key — that is O(1) formatting, far cheaper than the multi-layer keycap raster; the alloc win is the raster, and the `FOLLOW-UP` note's concern (re-format every frame) is dominated by the raster cost the sprite removes. If the key-building `knobPillLabel`/`synthParamDisplayName` allocs themselves show up in Task 6's budget, memoize them per `(paramID,value)` then — do not pre-optimize here.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthParamCard' ./internal/ui/`
Expected: PASS — only the edited card re-renders; cached card is byte-identical to direct.

- [ ] **Step 5: Guard against the readout-rect regression**

The card loop also populates `instEditorReadoutRects`/hit rects elsewhere — confirm those are still set every frame (they are set OUTSIDE the sprite render, from `k.Rect()`). Run the existing synth input tests:
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthKnob|SynthTab|ParamCard|synth_card' ./internal/ui/`
Expected: PASS (no input/hit-rect regression).

- [ ] **Step 6: Checkpoint** — stop for review. Do NOT commit.

---

### Task 4: Chip-strip sprite (coord-relative refactor — riskiest, isolated)

Cache the whole chip strip. It draws from pre-laid-out absolute `dv.instEditorChips[i].rect`, so the render callback must translate coords by the strip origin. Hit-testing is unaffected (rects come from Layout).

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`drawSynthChipStrip` `:2346`)
- Test: `src/go/internal/ui/synth_component_sprite_test.go`

- [ ] **Step 1: Write the failing pixel-identity test**

Add `TestSynthChipStripSprite_MatchesDirect`: render the chip strip cached vs `RENDER_SAFE` and assert byte-identical. Add `TestSynthChipStrip_ReRendersOnSelection`: 0 re-renders across identical frames; 1 re-render after changing the selected section / an enable toggle.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthChipStrip' ./internal/ui/`
Expected: FAIL (not wrapped).

- [ ] **Step 3: Split layout-read from pixel-draw and wrap**

Compute the strip's bounding rect (union of non-empty `dv.instEditorChips[i].rect`), and refactor the drawing body into `drawSynthChipStripContent(dst, origin image.Point)` that subtracts `origin` from every chip/connector coordinate. Then:

```go
func (p *synthPanel) drawSynthChipStrip(dst *ebiten.Image) {
	strip := p.chipStripBounds() // union of instEditorChips[i].rect; empty → return
	if strip.Empty() { return }
	key := struct {
		sel      synthSectionID
		enable   uint64 // FNV of per-chip enabled flags
		thumb    uint64 // ensureStageThumbs hash
		w, h     int
	}{p.dv.synthSelectedSectionID(), p.chipEnableHash(), p.chipThumbHash(), strip.Dx(), strip.Dy()}
	p.sprites.chip.Draw(dst, strip, key, func(img *ebiten.Image, r image.Rectangle) {
		p.drawSynthChipStripContent(img, strip.Min) // translate absolute chip coords by -strip.Min
	})
}
```

The `chipEnableHash`/`chipThumbHash` fold the exact state the strip's pixels depend on (enabled dots + per-column watermark thumbs). If a helper for the thumb hash already exists (`ensureStageThumbs` maintains one), reuse it. `chip` sprite uses `minInterval = 0` (discrete changes).

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'SynthChipStrip' ./internal/ui/`
Expected: PASS (pixel-identical, re-renders only on selection/enable/thumb change).

- [ ] **Step 5: Full synth + input regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Synth' ./internal/ui/` and `GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: PASS + WASM build exit 0.

- [ ] **Step 6: Checkpoint** — stop for review. Do NOT commit.

---

### Task 5: Sampler-tab regression guard

Confirm the shared `drawParamCard` and the recent Sampler highlight work are untouched by the Synth adoption.

**Files:** none (verification only) unless a regression is found.

- [ ] **Step 1: Run the Sampler + shared-card + highlight suites**

Run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'Sampler|ParamCard|param_card|highlight|Highlight|HandleHover' ./internal/ui/ -v 2>&1 | tail -40
```
Expected: PASS — no signature change to `drawParamCard`, no Sampler caching added, hover overlays unchanged. If any Sampler/highlight test fails, STOP and report — the Synth change leaked into shared code; fix by ensuring `drawParamCard`'s signature/behavior is byte-identical and the Sampler still calls it directly.

- [ ] **Step 2: Checkpoint** — record the green Sampler result. Do NOT commit.

---

### Task 6: Capture the AFTER numbers + prove the delta + ratchet budgets

The deliverable proof. Re-run the same measurements as Task 1 and re-baseline the budgets.

**Files:**
- Create: `bench-results/synth-cache-after.md`
- Modify: `src/go/internal/ui/synth_live_param_edit_alloc_test.go` (re-baseline drag budget), `eq_panel_draw_alloc_discipline_test.go` (`perTabAllocBudget[TabSynth]` ratchet)

- [ ] **Step 1: Re-run the byte + alloc budgets and ratchet**

Run the same commands as Task 1 Step 1. The byte-budget test is ALREADY GREEN pre-change (baseline: idle 71 KB / budget 128; drag **1055 KB** / budget **1536** — the stub reattribution from the TraceSprite work already fixed the old RED). So the win here is a **reduction**, not a red→green flip: the drag-frame bytes should drop materially with sprite caching. Tighten the drag budget from 1536 toward the new measured drag value + small headroom, with a one-line rationale citing the sprite adoption. Lower `perTabAllocBudget[TabSynth]` from 1198 to the new measured allocs/run + headroom (ratchet). NOTE: these stub-side budgets under-represent the real GPU draw-call win — the primary proof is the new draw-call-collapse test (Step 1b) and Harness B (Step 2).

- [ ] **Step 1b: Add a Synth-tab draw-call-collapse guard (mirror EQ)**

Mirror `TestEQTabSteadyStateDrawCallBudget` (`eq_curve_sprite_test.go:150`) with a new `TestSynthTabSteadyStateDrawCallBudget`: drive the Synth-tab Draw twice on identical params and assert the second frame's `bumpDrawCall` count collapses to a small constant (a handful of blits) instead of the thousands of per-column `drawRect`s. This is the machine-independent Go-side proof of the redraw collapse. Record the before-caching count (measured in Task 1) vs the after count in the assertion's comment.

- [ ] **Step 2: Re-run Harness B and compute the delta**

Run the same foreground command as Task 1 Step 2. Copy the result to `bench-results/synth_stage_toggle_ui.after.json`. Build `bench-results/synth-cache-after.md` with a before/after table for the `synth` run: `drawMaxMS`, `maxGapMs`, `deferAbsMsP90`, `fpsAvg` (before from `synth-cache-before.md`, after from this run), plus the `eqControl` floor for reference. The success criterion is that the synth-run draw/lateness numbers move materially toward the eqControl floor (the whole point of the change). Record the actual numbers whatever they are — this is measurement, not a gate that fails the build.

- [ ] **Step 3: Re-run the budget tests after the ratchet to confirm green**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSynthTab_LiveKnobEditFrameByteBudget|AllocBudget' ./internal/ui/`
Expected: PASS at the new tightened budgets.

- [ ] **Step 4: Full fast-suite sanity**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: no NEW failures vs the known pre-existing set (`TestFocusGraph_RespondsToEveryKnob` flake and any node-groups-WIP reds already present at HEAD — verify each red predates this work per `feedback_no_preexisting_failure_baselines`).

- [ ] **Step 5: Update the memory + present the before/after**

Append the measured before/after delta to `project_synth_stage_toggle_bench_attribution` (or a new `project_synth_tab_render_cache` memory) so the follow-up trail is complete, and present `synth-cache-before.md` vs `synth-cache-after.md` to the user.

- [ ] **Step 6: Final checkpoint** — stop for review. Do NOT commit.

---

## Self-Review

**Spec coverage:**
- TraceSprite adoption on all five surfaces → Tasks 2 (3 traces), 3 (cards), 4 (chip strip). ✓
- Per-card granularity keyed on paramID → Task 3. ✓
- Hover/highlight stays external, not cached/keyed → enforced (cards use `paramCardState` discrete only; Global Constraints). ✓
- Eviction on tab-hide via existing chokepoint → Task 2 Step 5. ✓
- Sampler untouched + green → Task 5; `drawParamCard` signature unchanged (Global Constraints). ✓
- Pixel-identity + render-count gates per surface → Tasks 2/3/4. ✓
- Before/after performance proof (explicit user ask) → Task 1 (before) + Task 6 (after, Harness B + budgets). ✓
- Budget re-baseline/ratchet → Task 6. ✓

**Placeholder scan:** No TBD/TODO. The one judgment note (memoize key strings only if Task 6's budget shows them) is explicitly conditional, not a deferral of required work.

**Type consistency:** `synthSpriteCache` fields/methods (`card`, `beginCardFrame`, `endCardFrame`, `releaseAll`, `focus`/`mirror`/`fullNote`/`chip`), `EQCallbacks.ReleaseSynthSprites`, and `TraceSprite.Draw/Release/resident` are used identically across tasks. Card key struct and `paramCardState{Selected,Dim}` match the verified source. `synthParamsHash(instID)` used consistently in keys.

**Never-commit:** every task ends in a checkpoint, not a commit.
