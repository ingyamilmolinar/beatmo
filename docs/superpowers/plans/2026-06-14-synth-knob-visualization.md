# Synth Tab Knob-Effect Visualization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every synth knob's effect obvious via (A) a live final-output mirror of the real rendered note in the right pane and (B) a per-knob concept picture under each knob, both with before/after ghosts.

**Architecture:** Reuse the existing Synth-tab preview helpers (`drawSynthOscPlot`, `drawSynthADSRPlot`, `drawSynthFilterPlot`) as concept renderers keyed by a param's `Group`; add three new cheap concept renderers (FM bars, motion curve, post waveshape). Promote the test-only offline render harness to a public `audio.RenderInstrumentPreview` and draw it in the right pane, refreshed off-thread on knob release (debounced, cached). Mini concept pictures are pure math and update live during drag.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/audio` recipe engine, `internal/async` pools, `drawRect` interceptor pixel tests.

**Repo conventions:** Use `.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`. **Do NOT `git add`/commit** (per project rule — leave all changes in the working tree; the "Commit" steps below are recorded for completeness but the executor must SKIP the actual commit and instead just tick the box). Screenshots: `make screenshots-all SCENES=<name>` → `screenshots/`.

---

## File Structure

| File | Responsibility | New/Modify |
|---|---|---|
| `src/go/internal/audio/preview_render.go` | Public `RenderInstrumentPreview(instID, durationMs) []float64` — renders one note of an instrument's recipe to PCM, off realtime path. `//go:build !test` + a `//go:build test` stub. | Create |
| `src/go/internal/audio/preview_render_stub.go` | `-tags test` stub returning a deterministic synthetic decaying wave (so UI tests run without C DSP). | Create |
| `src/go/internal/audio/preview_render_test.go` | Tests for the real render path (`!test && !js`). | Create |
| `src/go/internal/ui/synth_concept_viz.go` | Concept-renderer registry: `conceptRendererForGroup(group) conceptRenderer` + the 3 new renderers (FM bars, motion, post waveshape) + ghost param plumbing. | Create |
| `src/go/internal/ui/synth_concept_viz_test.go` | Registry coverage + pixel/reactivity tests. | Create |
| `src/go/internal/ui/synth_preview_layer.go` | Right pane becomes the final-output mirror (`drawSynthMirror`) + keeps osc/adsr/filter helpers (now reused by the registry). | Modify |
| `src/go/internal/ui/synth_mirror.go` | Mirror state: async render request, debounce, cache by `(recipeID, paramsHash)`, ghost buffer. | Create |
| `src/go/internal/ui/synth_mirror_test.go` | Mirror cache/debounce/coalesce + ghost state tests. | Create |
| `src/go/internal/ui/synth_panel_zone.go` | Detail-pane knob cells grow to host a mini-visual; draw concept picture under each knob; capture ghost on knob press. | Modify |
| `src/go/internal/ui/synth_ghost.go` | Per-knob ghost capture/fade state machine. | Create |
| `src/go/internal/ui/synth_ghost_test.go` | Ghost state machine tests. | Create |
| `src/go/internal/ui/scene_catalog.go` (+ `screenshot_subjects.go`) | Add a `crop_synth_knob_viz` scene exposing the synth tab with the new visuals for screenshots. | Modify |
| `src/go/internal/ui/eq_panel_draw_alloc_discipline_test.go` | Add/justify alloc budget for new Synth per-frame draws. | Modify |
| `src/js/synth_knob_viz.browser.test.js` | One browser test: real-WASM mirror render produces a visible trace. | Create |

---

## STAGE 1 — Audio preview render path

Produces `audio.RenderInstrumentPreview`. No UI yet. (Screenshot N/A — verified by Go test + a printed PCM stat.)

### Task 1.1: `RenderInstrumentPreview` test (real path)

**Files:**
- Test: `src/go/internal/audio/preview_render_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build !test && !js

package audio

import "testing"

func TestRenderInstrumentPreview_NonEmptyAndDeterministic(t *testing.T) {
	// Use a known builtin instrument id present in the default demo.
	const inst = "kick"
	a := RenderInstrumentPreview(inst, 200)
	if len(a) == 0 {
		t.Fatalf("expected non-empty PCM")
	}
	_, peak := argmaxAbs(a)
	if peak <= 0 {
		t.Fatalf("expected audible peak, got %v", peak)
	}
	b := RenderInstrumentPreview(inst, 200)
	if hashFloat64PCM(a) != hashFloat64PCM(b) {
		t.Fatalf("render must be deterministic for identical params")
	}
}

func TestRenderInstrumentPreview_ReactsToParamChange(t *testing.T) {
	const inst = "kick"
	ResetInstrumentParams(inst)
	base := RenderInstrumentPreview(inst, 200)
	SetInstrumentParam(inst, "fundamental", 120) // shift a core knob
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	changed := RenderInstrumentPreview(inst, 200)
	if hashFloat64PCM(base) == hashFloat64PCM(changed) {
		t.Fatalf("changing a param must change the rendered preview")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestRenderInstrumentPreview`
Expected: FAIL — `undefined: RenderInstrumentPreview` (and verify `ResetInstrumentParams`/`SetInstrumentParam` exist; if `ResetInstrumentParams` is absent, use `SetInstrumentParam` only and restore the old value in cleanup).

- [ ] **Step 3: Implement `preview_render.go`**

```go
//go:build !test && !js

package audio

// RenderInstrumentPreview renders ONE note of an instrument's resolved recipe to
// PCM, off the realtime audio path, for UI preview. durationMs is clamped to a
// short window so the call stays cheap. Returns float64 mono samples at the
// engine sample rate. Safe to call from a worker goroutine (no Oto, no mixer
// state mutation — it renders the recipe directly).
func RenderInstrumentPreview(instrumentID string, durationMs int) []float64 {
	if durationMs <= 0 {
		durationMs = 200
	}
	if durationMs > 1000 {
		durationMs = 1000 // hard cap: keep preview render bounded
	}
	recipeID := RecipeForInstrument(instrumentID)
	if recipeID == "" {
		return nil
	}
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		return nil
	}
	sr := int(sampleRate)
	n := sr * durationMs / 1000
	buf := make([]float32, n)
	params := MergeRecipeDefaults(recipeID, GetInstrumentParams(instrumentID))
	recipe.Render(buf, sr, n, 0, params)
	out := make([]float64, n)
	for i, v := range buf {
		out[i] = float64(v)
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestRenderInstrumentPreview`
Expected: PASS. (If `kick` is not registered in this build, switch the test const to an id from `RecipeRegistrations()` — assert the chosen id exists at test top.)

- [ ] **Step 5: Commit** (SKIP actual commit per repo rule; just tick)

### Task 1.2: `-tags test` stub for the UI build

**Files:**
- Create: `src/go/internal/audio/preview_render_stub.go`

- [ ] **Step 1: Write the stub** (no separate failing test — it's exercised by Stage 4 UI tests; verify it compiles under `-tags test`)

```go
//go:build test || js

package audio

import "math"

// RenderInstrumentPreview (stub) returns a deterministic decaying sine whose
// frequency tracks the instrument's "fundamental" param when present, so UI
// preview tests have a stable, param-reactive signal without the C DSP backend.
func RenderInstrumentPreview(instrumentID string, durationMs int) []float64 {
	if durationMs <= 0 {
		durationMs = 200
	}
	sr := 48000.0
	n := int(sr * float64(durationMs) / 1000.0)
	freq := 220.0
	merged := MergeRecipeDefaults(RecipeForInstrument(instrumentID), GetInstrumentParams(instrumentID))
	if f, ok := merged["fundamental"]; ok && f > 0 {
		freq = f
	}
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / sr
		out[i] = 0.7 * math.Sin(2*math.Pi*freq*t) * math.Exp(-6*t)
	}
	return out
}
```

- [ ] **Step 2: Verify both build tags compile**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/audio/ && ../../.tools/go/bin/go vet ./internal/audio/`
Expected: no errors. (If `RenderInstrumentPreview` is now doubly-defined, confirm the real file carries `//go:build !test && !js` and the stub `//go:build test || js`.)

- [ ] **Step 3: Commit** (SKIP)

---

## STAGE 2 — Concept-renderer registry (per-knob pictures)

Pure-math draw functions keyed by param `Group`. Reuses osc/adsr/filter; adds FM/motion/post. Verified by drawRect-interceptor pixel tests. Screenshot at end of Stage 3 (registry alone has no on-screen surface yet).

### Task 2.1: Registry skeleton + reuse-existing test

**Files:**
- Create: `src/go/internal/ui/synth_concept_viz.go`
- Create: `src/go/internal/ui/synth_concept_viz_test.go`

The `Group` strings to cover (from `internal/audio`): `osc`, `filter`, `env`, `fm`, `post`, plus motion params routed by the `pitch`/`lfo`/`burst` enable-stage groups. Confirm exact group strings by grepping `Group:` in `internal/audio/modular_recipe.go` and `synth_recipe_wired.go` before writing the switch; route any unmapped/`core`/`generic`/voice group to the OSC picture as the safe default (it shows the seed wave).

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

// Every param group a synth knob can carry must resolve to a concept renderer.
func TestConceptRendererForGroup_TotalCoverage(t *testing.T) {
	groups := []string{"osc", "filter", "env", "fm", "post", "pitch", "lfo", "burst", "core", "generic", "voice", ""}
	for _, g := range groups {
		if conceptRendererForGroup(g) == nil {
			t.Errorf("group %q has no concept renderer", g)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConceptRendererForGroup`
Expected: FAIL — `undefined: conceptRendererForGroup`.

- [ ] **Step 3: Implement the registry**

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// conceptRenderer draws the abstract "what this knob does" picture for a param
// group into rect. instID resolves live params; ghost is the pre-drag param
// snapshot (nil = no ghost). All renderers are pure math (no audio render) so
// they update live during a drag.
type conceptRenderer func(dst *ebiten.Image, rect image.Rectangle, instID string, ghost map[string]float64)

// conceptRendererForGroup maps a ParamDef.Group to its picture. Unmapped groups
// fall back to the OSC seed-wave picture (never nil).
func conceptRendererForGroup(group string) conceptRenderer {
	switch group {
	case "filter":
		return conceptFilter
	case "env":
		return conceptEnvelope
	case "fm":
		return conceptFM
	case "post":
		return conceptPost
	case "pitch", "lfo", "burst":
		return conceptMotion
	default: // osc, core, generic, voice, ""
		return conceptOsc
	}
}

func conceptOsc(dst *ebiten.Image, r image.Rectangle, instID string, _ map[string]float64) {
	drawSynthOscPlot(dst, r, instID)
}
func conceptEnvelope(dst *ebiten.Image, r image.Rectangle, instID string, _ map[string]float64) {
	drawSynthADSRPlot(dst, r, instID, 1.0)
}
func conceptFilter(dst *ebiten.Image, r image.Rectangle, instID string, _ map[string]float64) {
	drawSynthFilterPlot(dst, r, instID)
}
```

(Plus `conceptFM`, `conceptMotion`, `conceptPost` added in Tasks 2.2–2.4.)

- [ ] **Step 4: Run** — still fails (conceptFM/Motion/Post undefined). That's expected; implement them next, then this test passes.

### Task 2.2: `conceptFM` (harmonic bars) — TDD

**Files:** `synth_concept_viz.go`, `synth_concept_viz_test.go`

- [ ] **Step 1: Failing pixel test** — assert the renderer draws ≥1 non-background rect, and that increasing `fm_op2_depth` increases drawn ink (more/taller bars). Use the existing drawRect interceptor pattern from `audio_panel_render_pixels_test.go` (find it: `grep -n "drawRectInterceptor\|countDrawRect\|installDrawRectCapture" internal/ui/*_test.go`). Skeleton:

```go
func TestConceptFM_DepthAddsInk(t *testing.T) {
	const inst = "kick"
	restore := captureDrawRects(t) // helper from audio_panel_render_pixels_test.go
	dst := newTestImage(120, 48)
	r := image.Rect(0, 0, 120, 48)

	audio.SetInstrumentParam(inst, "fm_op2_depth", 0.1)
	low := countInkRects(func() { conceptFM(dst, r, inst, nil) })
	audio.SetInstrumentParam(inst, "fm_op2_depth", 6.0)
	high := countInkRects(func() { conceptFM(dst, r, inst, nil) })
	t.Cleanup(func() { audio.SetInstrumentParam(inst, "fm_op2_depth", 0); restore() })

	if high <= low {
		t.Fatalf("more FM depth should draw more ink: low=%d high=%d", low, high)
	}
}
```

Adapt the exact helper names to whatever `audio_panel_render_pixels_test.go` exposes (read it first). If no reusable counter exists, count via a local `drawRect` interceptor seam already present in the UI package's test build.

- [ ] **Step 2: Run — FAIL** (`undefined: conceptFM`).

- [ ] **Step 3: Implement `conceptFM`** — draw N vertical bars whose heights come from FM operator depths (read merged params `fm_op1_depth..fm_op4_depth`, or `fm_op2_depth` for 2-op). Use `drawRect` + a synthwave color (e.g. `WithAlpha(colAccent, AlphaStrong)`); background fill `WithAlpha(genColorVizScopeBg, AlphaOverlay)` to match the other plots. More depth → taller/more bars.

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Commit** (SKIP)

### Task 2.3: `conceptMotion` (pitch-drop / LFO wobble / burst) — TDD

**Files:** `synth_concept_viz.go`, `synth_concept_viz_test.go`

- [ ] **Step 1: Failing test** — assert non-empty draw, and that raising `lfo_rate` increases the number of oscillation direction-changes in the drawn curve (more wiggles). Sample the curve math directly via a helper `motionCurveSamples(instID, n) []float64` and count sign changes of the derivative.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** `conceptMotion` + `motionCurveSamples`: a time curve combining pitch-env decay (`pitchenv_amt`, `pitchenv_decay`), LFO (`lfo_rate`, `lfo_depth`), and burst markers (`burst_*`). Pure math. Draw with `drawRect` segments like `drawSynthADSRSegment`.

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Commit** (SKIP)

### Task 2.4: `conceptPost` (waveshape + gain, with ghost) — TDD

**Files:** `synth_concept_viz.go`, `synth_concept_viz_test.go`

- [ ] **Step 1: Failing test** — assert (a) non-empty draw; (b) when `ghost` carries a different `drive`, MORE total ink is drawn than with `ghost == nil` (the ghost trace adds a second curve).

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** `conceptPost`: draw a sine fed through `tanh(drive*x)*gain` waveshaping; if `ghost != nil`, draw the ghost curve faint first (`WithAlpha(genColorBorder, AlphaSubtle)`), then the live curve.

- [ ] **Step 4: Run** — now `TestConceptRendererForGroup_TotalCoverage` (Task 2.1) and 2.2–2.4 all PASS.

- [ ] **Step 5: Commit** (SKIP)

---

## STAGE 3 — Per-knob mini-visual integration

Wire the registry into the detail pane: each knob cell grows to host its concept picture; capture ghost on press. **Screenshot checkpoint at the end.**

### Task 3.1: Grow knob cells to reserve a mini-visual band

**Files:** `src/go/internal/ui/synth_panel_zone.go` (the `ControlGrid` cell sizing in `buildSynthTab`/`layoutSynthSections` — find with `grep -n "sectionGrid\|ControlGrid\|cellH\|SynthKnobIdeal" internal/ui/synth_panel_zone.go`), DESIGN.md token.

- [ ] **Step 1: Add a density token `synthConceptVizH`** (kebab in DESIGN.md `densities:` for all three densities, e.g. compact 26 / comfortable 32 / spacious 38). Run `make gen-design-tokens`. Read via `Profile().DensityValues().SynthConceptVizH`.

- [ ] **Step 2: Failing layout test** in `synth_concept_viz_test.go`:

```go
func TestSynthKnobCellReservesVizBand(t *testing.T) {
	g := newSynthTestGame(t)          // existing helper; grep newSynthTestGame / setup
	selectSynthInstrument(t, g, "kick")
	switchToSynthTab(t, g)
	cell, ok := firstSynthKnobCellRect(g) // new tiny accessor
	if !ok { t.Fatal("no knob cell") }
	vizH := Profile().DensityValues().SynthConceptVizH
	if cell.Dy() < knobDiameter(g)+captionH(g)+vizH {
		t.Fatalf("cell %d too short to host viz band %d", cell.Dy(), vizH)
	}
}
```

Adapt helper names to existing test utilities (read `synth_panel_*_test.go`).

- [ ] **Step 3: Run — FAIL.**

- [ ] **Step 4: Implement** — increase the per-cell height used by the section `ControlGrid` by `SynthConceptVizH + SpaceXS`. Keep columns/scroll intact (the grid already draws a scrollbar on overflow — taller cells mean fewer rows visible, exactly the "show a few, scroll for more" behavior the user asked for).

- [ ] **Step 5: Run — PASS.**

- [ ] **Step 6: Commit** (SKIP)

### Task 3.2: Draw the concept picture under each knob

**Files:** `synth_panel_zone.go` (`drawSynthDetailPane`, the `for j, kIdx := range sel.knobIdxs` loop, after the caption/badge block at ~line 1868).

- [ ] **Step 1: Failing test** — render the synth tab to a stub image and assert the viz band rect under a known knob contains ink (use the drawRect interceptor; assert ≥1 rect intersects the band below the caption). Also assert changing the bound param changes the band's pixels.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** — inside the loop, compute `vizR := image.Rect(slot.Min.X+pad, captionY+TextHeight()+SpaceXS, slot.Max.X-pad, slot.Max.Y-SpaceXS)`; resolve `group := dv.instEditorBindings[kIdx].def.Group`; call `conceptRendererForGroup(group)(dst, vizR, instID, dv.synthGhostFor(kIdx))`. Guard `if vizR.Dy() >= 12`. Respect the bypassed-stage scrim (the existing scrim at the end already overpaints disabled stages).

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Commit** (SKIP)

### Task 3.3: Screenshot checkpoint — Stage 3

**Files:** `scene_catalog.go`, `screenshot_subjects.go` (add `crop_synth_knob_viz`).

- [ ] **Step 1:** Add a crop scene whose `Setup` selects a modular synth instrument, switches to the Synth tab, and selects the FILTER section (so the curve picture shows). Follow the `crop_*` recipe in CLAUDE.md (declare subject const, return bounds from `SubjectRect`, set `Subject` on the catalog entry). `scene_crop_test.go` will exercise it.

- [ ] **Step 2:** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSceneCrop` — PASS.

- [ ] **Step 3:** Capture: `make screenshots-all SCENES=crop_synth_knob_viz` and inspect `screenshots/crop_synth_knob_viz*.png` — confirm mini-visuals render under the knobs. **(This is the Stage-3 visual confirmation.)**

- [ ] **Step 4: Commit** (SKIP)

---

## STAGE 4 — Final-output mirror (right pane)

Replace the right pane's stacked plots with the real-PCM mirror, refreshed off-thread on release, debounced, cached, with ghost. **Screenshot checkpoint at the end.**

### Task 4.1: Mirror state + cache/debounce (pure logic, TDD)

**Files:** Create `src/go/internal/ui/synth_mirror.go`, `src/go/internal/ui/synth_mirror_test.go`.

- [ ] **Step 1: Failing test**

```go
package ui

import "testing"

func TestSynthMirror_CachesByParamsHash(t *testing.T) {
	m := newSynthMirror()
	renders := 0
	render := func(inst string) []float64 { renders++; return []float64{0.1, 0.2} }
	m.request("kick", "hashA", render) // first → renders
	m.drainForTest()
	m.request("kick", "hashA", render) // same hash → cached, no render
	m.drainForTest()
	if renders != 1 {
		t.Fatalf("expected 1 render for identical hash, got %d", renders)
	}
	m.request("kick", "hashB", render) // new hash → renders
	m.drainForTest()
	if renders != 2 {
		t.Fatalf("expected 2 renders after hash change, got %d", renders)
	}
}

func TestSynthMirror_CoalescesRapidRequests(t *testing.T) {
	m := newSynthMirror()
	renders := 0
	render := func(inst string) []float64 { renders++; return []float64{1} }
	for i := 0; i < 10; i++ { m.request("kick", "h", render) } // identical → 1
	m.drainForTest()
	if renders != 1 {
		t.Fatalf("rapid identical requests must coalesce, got %d", renders)
	}
}
```

- [ ] **Step 2: Run — FAIL** (`undefined: newSynthMirror`).

- [ ] **Step 3: Implement `synth_mirror.go`** — `synthMirror` holds: last `(inst, hash)`, cached `pcm []float64`, `ghost []float64`, and an `*async.Pool` (acquired from `async.DefaultRegistry().Get("synth.preview", async.Options{Workers:1, Queue:8})` with a private-pool fallback per the `recording.lifecycle` recipe). `request(inst, hash, render)` returns immediately if `hash == m.hash` (cache hit); else moves current `pcm`→`ghost`, submits the render job (non-blocking `Submit`), and on completion stores the result + posts an invalidation flag the Draw loop reads. `drainForTest()` runs pending jobs synchronously (test seam). Add `t.Cleanup` releasing the named pool in tests (per CLAUDE.md async test discipline).

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Commit** (SKIP)

### Task 4.2: Draw the mirror in the right pane

**Files:** `synth_preview_layer.go` (`drawSynthPreviewPane` → top section becomes `drawSynthMirror`), `synth_panel_zone.go` (own a `dv.synthMirror`, request render on knob release).

- [ ] **Step 1: Failing test** — after a simulated knob release, `drawSynthMirror` draws a wave trace (drawRect ink in the mirror rect). Assert ink present once the mirror has PCM; assert ghost trace present after a second release with different params.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** — `drawSynthMirror(dst, rect, mirror)` uses `drawWaveTrace(dst, mirror.pcm, rect, midY, width, colWaveTrace, yGain, colSynthOscFill)` + `overdrawWaveClips` + a faint ghost trace behind. Wire the request: in the knob-release path (`synthKnobHitAdapter.OnRelease` / `propagateSynthSliderValue`) call `dv.synthMirror.request(instID, dv.synthParamsHash(instID), func(i string){ return audio.RenderInstrumentPreview(i, 250) })`. Hash = cheap FNV over the merged params map. Keep mini-visuals (Stage 3) updating live during drag; only the mirror waits for release.

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5:** Tick the mirror's async-completion flag in the Synth tab's Update path so a completed render triggers a redraw (no per-frame render). Add a test asserting `Update` consumes the flag exactly once.

- [ ] **Step 6: Commit** (SKIP)

### Task 4.3: Screenshot checkpoint — Stage 4

- [ ] **Step 1:** Extend `crop_synth_knob_viz` (or add `crop_synth_mirror`) so the mirror has PCM (call a release/request in `Setup`, then `drainForTest`-equivalent synchronous render).
- [ ] **Step 2:** `make screenshots-all SCENES=crop_synth_knob_viz crop_synth_mirror` and inspect PNGs — confirm the right pane shows the rendered wave with ghost. **(Stage-4 visual confirmation.)**
- [ ] **Step 3: Commit** (SKIP)

---

## STAGE 5 — Ghost state machine, alloc budget, browser test, polish

**Screenshot checkpoint at the end (final).**

### Task 5.1: Per-knob ghost capture/fade (TDD)

**Files:** Create `synth_ghost.go`, `synth_ghost_test.go`; wire into `synth_panel_zone.go` press/release.

- [ ] **Step 1: Failing test** — `synthGhost` state machine: `onPress(kIdx, params)` snapshots; `ghostFor(kIdx)` returns the snapshot while active; `onRelease(kIdx)` starts a fade that `advance(dt)` decays to nil after the fade duration. Assert snapshot present during drag, nil after fade completes.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** `synth_ghost.go`; call `onPress` from `synthKnobHitAdapter.OnPress`, `onRelease` from `OnRelease`, `advance` from the Synth tab Draw (reuse the existing `uiAnimFrame`/hover-anim tick cadence).

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5:** Wire `dv.synthGhostFor(kIdx)` (used in Task 3.2) to return `ghostFor`. Confirm Stage-3 reactivity test still passes.

- [ ] **Step 6: Commit** (SKIP)

### Task 5.2: Alloc budget + discipline

**Files:** `eq_panel_draw_alloc_discipline_test.go` (+ the per-tab budget map).

- [ ] **Step 1:** Run the alloc discipline test; if Synth-tab per-frame draws now exceed budget, update `perTabAllocBudget[TabSynth]` (or the synth entry) with a one-line rationale citing `synth_concept_viz.go` + `drawSynthMirror`.
- [ ] **Step 2:** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run Alloc` — PASS.
- [ ] **Step 3: Commit** (SKIP)

### Task 5.3: Browser test — real-WASM mirror

**Files:** Create `src/js/synth_knob_viz.browser.test.js`.

- [ ] **Step 1:** Write a minimal test (per the JS charter — WASM↔audio boundary only, not DSP logic): boot WASM, select a synth instrument, switch to the Synth tab, set a synth param via the JS export, assert `RenderInstrumentPreview`-backed export (or a `synthMirrorPCM()` debug export added in Task 4.2) returns a non-empty array. If adding a debug export, register it and add it to `wasm_bridge_smoke.browser.test.js`'s catalogue per CLAUDE.md.
- [ ] **Step 2:** Run: `GO=$(pwd)/.tools/go/bin/go node src/js/synth_knob_viz.browser.test.js` — PASS.
- [ ] **Step 3: Commit** (SKIP)

### Task 5.4: Full suite + final screenshots

- [ ] **Step 1:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/` — all PASS (note any PRE-EXISTING unrelated failures by diffing against a stash-free baseline; do not attribute them to this work).
- [ ] **Step 2:** `make screenshots-all SCENES=crop_synth_knob_viz crop_synth_mirror MOBILE=1` then again without `MOBILE=1` — inspect desktop + mobile PNGs. **(Final visual confirmation: per-knob pictures + mirror on both platforms.)**
- [ ] **Step 3:** Run `make gen-design-tokens` and the design-token guards (`TestDesignMDDrift|TestTokenDiscipline`) — PASS.
- [ ] **Step 4: Commit** (SKIP)

---

## Self-Review notes (spec coverage)

- A (mirror): Stage 1 (render path) + Task 4.1/4.2/4.3. Snap-on-release + debounce + cache + off-thread + ghost: Task 4.1/4.2 + 5.1. ✓
- B (per-knob concept pictures, by Group, live during drag): Stage 2 + Task 3.1/3.2. ✓
- Ghost (both surfaces): Task 4.2 (mirror) + 5.1 (per-knob). ✓
- Scroll "show a few, scroll for more": Task 3.1 (taller cells reuse existing grid scrollbar). ✓
- Both platforms desktop-first; mobile degrade: stub render + Task 5.4 mobile screenshot; mirror request is platform-agnostic, mobile uses same on-release path. ✓
- Chain strip dropped: not implemented (YAGNI). ✓
- Tests Go-first + 1 browser test; alloc budget: Stages 2–5. ✓
- No-commit repo rule: every "Commit" step marked SKIP. ✓
