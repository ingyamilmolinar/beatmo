# Audio Panel Tree Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `DrumView` so a single **root tree** composes two fully-isolated child subtrees — the **drum-view subtree** (transport/timeline/row-rack) and the **audio-panel subtree** (the entire bottom tab section). Each subtree owns its own `HitIndex`, capture lifecycle, and overlay portal. This makes cross-section input structurally impossible and fixes the bug where a two-finger trackpad drag (wheel) over a synth knob scrolls the unrelated drum rows. Everything that renders or receives input lives inside the tree hierarchy — no ad-hoc DrumView-level draw or dispatch passes.

**Architecture:** Today every zone — including the whole audio panel (`eqPanelZone` at z=130, which internally hosts the sticky bar, tab controls, EQ/Wave/Spectrum/Meters/Chain/Synth/Sampler) — lives in ONE `DrumViewTree` with ONE shared `HitIndex`. A wheel over the panel that no panel control consumes falls through the shared index to the row-rack scroll handler at z=119. We split `DrumViewTree`'s frame loop into `Tick()`/`DispatchInput()` and its draw into `DrawContent()`/`DrawOverlays()`, then introduce a `RootTree` that owns two `DrumViewTree` instances: `dv.tree` (drum-view subtree, mostly unchanged) and `dv.audioTree` (audio-panel subtree). The root ticks both, routes each frame's input to exactly one subtree (capture-holder → modal-owner → top-down by z), and draws all subtree content then all subtree overlays. Because each subtree's hit areas are spatially disjoint, a coordinate in the panel region only ever consults the audio subtree's index — the row rack can never see it.

**Tech Stack:** Go 1.23, Ebiten, the existing `DrumViewTree`/`HitIndex`/`OverlayPortal`/`Layer`/`Zone` machinery in `src/go/internal/ui/`. Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.

**Root cause (confirmed by reading code):**
- Wheel dispatch loop `drumview_tree.go:591-609` walks `HitIndex.At(mx,my)` top-down by z, stopping at the first handler returning != `InputIgnored`.
- `synthKnobHitAdapter.OnWheel` (`synth_panel_zone.go:2949-2971`) returns `InputIgnored` when the knob's section is not scrollable (`sectionGridForKnob()` → nil at `synth_panel_zone.go:2812-2827` when `!g.HasScroll()`).
- Panel catch-all `inputCaptureHandler.OnWheel` (`input_capture.go:58-61`) ALSO returns `InputIgnored` by design.
- So the wheel falls to `rowRackScrollHitAdapter.OnWheel` (`row_rack_zone.go:1271-1295`, z=119), scrolling the drum rows.

**Scope decision (from the user):** This round delivers the audio panel as a proper subtree under a real root tree, with the drum-view section as its sibling subtree. The legacy out-of-tree input paths that CLAUDE.md deliberately excluded (grid node tap/drag gestures, camera pan/zoom, the splitter, the node sidebar — all via `inputDispatcher` in `game_update.go`) STAY as-is this round and are documented as the next migration. The `RootTree` is built so they can later be added as additional child subtrees.

---

## What moves where

| Element | Today | After |
|---|---|---|
| transport, timeline, row-rack zones | `dv.tree` | `dv.tree` (drum-view subtree) — unchanged |
| background / rack-mask / EQ-peek / transport-pulse / row-zoom-chips / notifications / layout-guides layers | `dv.tree` | `dv.tree` — unchanged (none overlap the bottom panel; EQ-peek/rack-mask/zoom-chips belong to the rows region) |
| drum-view portal (row menus, naming dialog, instrument picker, master vol popup, keyboard-shortcuts, overflow) | `dv.tree.portal` | `dv.tree.portal` — unchanged |
| **eqPanelZone** (z=130: sticky bar + tab controls + EQ/Wave/Spectrum/Meters/Chain/Synth/Sampler + synth knobs) | `dv.tree` | **`dv.audioTree`** (audio-panel subtree) |
| **chainPanelZone** | delegated inside `eqPanelZone` (`SetChainZone`) | moves implicitly with `eqPanelZone` |
| **row↔EQ divider layer** (z=186) | `dv.tree` | **`dv.audioTree`** (must draw above the panel) |
| **mobile view-switch layer** (z=150) | `dv.tree` | **`dv.audioTree`** (must draw above the panel) |
| **audio portal** (EQ channel dropdown, Synth/Sampler "Save As", legend popups) | `dv.tree.portal` | **`dv.audioTree.portal`** |
| composition (Draw/Update dispatch) | `DrumView.Draw`/`DrumView.Update` call `dv.tree` directly | **`dv.rootTree`** composes `[dv.tree, dv.audioTree]`; `DrumView` calls only `rootTree` |

---

## File Structure

**New files:**
- `src/go/internal/ui/root_tree.go` — the `RootTree` composer: holds ordered child `DrumViewTree`s, ticks all, coordinates input dispatch, draws content-then-overlays, ORs the suppress flag.
- `src/go/internal/ui/audio_panel_tree_isolation_test.go` — reproduction test (Task 1) + discipline tests (Task 8).

**Modified files:**
- `drumview_tree.go` — split `Update()`→`Tick()`+`DispatchInput()`; split `Draw()`→`DrawContent()`+`DrawOverlays()`; add `Capturing()`/`PortalHasModal()`/`Suppressing()`/`ClearCapture()` accessors. Keep `Update()`/`Draw()` as back-compat wrappers so existing direct-tree tests don't break.
- `drumview.go` (DrumView struct decl) — add `audioTree *DrumViewTree` and `rootTree *RootTree` fields.
- `drumview_ctor.go` — construct `audioTree` + `rootTree`; register `eqPanelZone` + divider + view-switch on `audioTree`; point `SetPortal` at `audioTree.Portal()`; add both subtrees to `rootTree`.
- `drumview_draw.go` — replace the `dv.tree.Draw` block with `dv.rootTree`.
- `drumview_update.go` — replace the `dv.tree.Update` block with `dv.rootTree`.
- `drumview_layout.go` — set the panel zone rect + bounds on `audioTree`.
- `drumview_close_popups.go` — clear capture/focus on both subtrees via the root.
- `drumview_layer_zorder_test.go` — update the pinned layer-order expectations for the two subtrees.

---

## Task 1: Reproduction test — wheel over a synth knob must NOT scroll the drum rows

**Files:**
- Test: `src/go/internal/ui/audio_panel_tree_isolation_test.go` (create)

This must FAIL before the refactor (proving the bug) and PASS after. It drives the REAL `DrumView` input loop (per `feedback_functional_tests_over_isolation`), not a synthetic HitIndex.

- [ ] **Step 1: Find the harness patterns to copy.** Read `testutil_test.go` header (DrumView/Game construction, `updateBeatInfos`, thread-safe accessors). Find an existing test that switches to the Synth tab and injects input through the tree: `grep -rn "ViaTreeDispatch\|SetInputForTest\|TabSynth\|setViewMode(viewModeSynth)" internal/ui/*_test.go`. Reuse the same construction + Synth-tab activation (on desktop the panel is always visible).

- [ ] **Step 2: Write the failing test.**

```go
package ui

import "testing"

// TestWheelOverSynthKnobDoesNotScrollRows reproduces the bug where a
// two-finger trackpad drag (wheel events) over a synth knob whose section
// is NOT scrollable leaked through the shared HitIndex to the row-rack
// scroll handler, scrolling the unrelated drum rows. Drives the real
// DrumView input loop.
func TestWheelOverSynthKnobDoesNotScrollRows(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12) // enough rows that rowOffset CAN change
	activateSynthTab(t, dv)              // desktop: panel visible, Synth tab active
	dv.layoutForTest()                   // full layout so knob rects exist

	knob := firstNonScrollableSynthKnobRect(t, dv)
	if knob.Empty() {
		t.Fatal("no synth knob rect available; check activateSynthTab/layout")
	}
	cx, cy := knob.Min.X+knob.Dx()/2, knob.Min.Y+knob.Dy()/2

	if dv.VisibleRowsForTest() >= len(dv.Rows) {
		t.Fatal("test is vacuous: all rows visible, no scroll possible")
	}
	before := dv.RowOffsetForTest()
	for i := 0; i < 6; i++ {
		injectWheelAt(t, dv, cx, cy, 0, -1) // negative = scroll down
		dv.updateForTest()
	}
	if after := dv.RowOffsetForTest(); before != after {
		t.Fatalf("wheel over synth knob scrolled the drum rows: rowOffset %d -> %d (must stay %d)", before, after, before)
	}
}
```

- [ ] **Step 3: Add the test-only helpers** (reuse existing equivalents where they already exist — prefer reuse). Each wraps a production path:
  - `newTestDrumViewWithRows(t, n)` — standard DrumView construction from `testutil_test.go`; ensure ≥ n rows so a scroll is possible.
  - `activateSynthTab(t, dv)` — select an instrument that has a synth recipe as the active channel, then `dv.eqPanelZone.SetActiveTab(TabSynth)` (routes through `EnsureAnalyzersForTab`).
  - `dv.layoutForTest()` / `dv.updateForTest()` — thin wrappers that call the SAME per-frame entry the production loop uses. After Task 5 the production loop runs `dv.rootTree`; keep these wrappers pointed at `dv` per-frame entry (`dv.tree.Update()` today → `dv.rootTree.Update()` after Task 5) so the test always exercises production composition.
  - `firstNonScrollableSynthKnobRect(t, dv)` — iterate `dv.instEditorSections`; pick a section where `dv.sectionGrid(s.id) == nil || !dv.sectionGrid(s.id).HasScroll()`; return its first knob's hit-area rect.
  - `injectWheelAt(t, dv, x, y, dx, dy)` — set cursor to (x,y) and wheel delta to (dx,dy) for ONE frame via the input-override seam used by other tree-dispatch tests. A package-level `wheel` func var exists at `input.go:60`; if no test setter exists, add `setWheelForTest(dx, dy float64) (restore func())` mirroring the cursor override.
  - `dv.RowOffsetForTest()`, `dv.VisibleRowsForTest()` — thread-safe getters for `dv.rowOffset` / `dv.visibleRows()` (add following the existing `*ForTest` convention if missing).

- [ ] **Step 4: Run — confirm it FAILS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelOverSynthKnobDoesNotScrollRows ./internal/ui/ -v`
Expected: FAIL — `wheel over synth knob scrolled the drum rows: rowOffset 0 -> N`.

- [ ] **Step 5:** Do NOT commit (per `feedback_no_staging_or_committing`). Confirm the file is present in `git status` and proceed.

---

## Task 2: Split `DrumViewTree.Update()` into `Tick()` + `DispatchInput()`

**Files:**
- Modify: `src/go/internal/ui/drumview_tree.go:258-308`

The root must tick both subtrees, THEN coordinate input. Splitting the frame loop exposes that seam while keeping `Update()` working for every existing direct-tree test.

- [ ] **Step 1: Extract `Tick()` and `DispatchInput()`** from the current `Update()` (lines 258-308). `Tick` = phases 1, 2, 2.5a/b/c (layout, zone updates, auto-focus, `portal.Update`). `DispatchInput` = phase 3 (`handleInput`), 3.5 (`portal.CleanupClosed`), and the suppress write — but gated by `allowNewPress`, and returning whether the tree is "busy" so the root can defer lower subtrees.

```go
// Update runs the full frame loop for a STANDALONE tree (Tick then
// DispatchInput with new presses allowed). Kept for direct-tree callers
// (tests, any single-tree use). A RootTree composes subtrees by calling
// Tick() on all of them then DispatchInput() on the chosen one.
func (t *DrumViewTree) Update() {
	t.Tick()
	t.DispatchInput(true)
}

// Tick runs the non-input phases: layout, per-zone Update, auto-focus,
// and portal Update. Safe to run on every subtree every frame regardless
// of which one will receive input.
func (t *DrumViewTree) Tick() {
	t.inputHandled = false
	t.wheelHandled = false
	t.layoutPass() // Phase 1
	for i := range t.zones {
		t.zones[i].zone.Update() // Phase 2
	}
	// Phase 2.5a/b: auto-focus transport (BPM box) / eq-panel (dB inputs).
	t.autoFocusPass() // extract lines 273-292 verbatim into this helper
	t.portal.Update() // Phase 2.5c
}

// DispatchInput runs Phase 3 (input) + 3.5 (portal cleanup) and writes the
// tree's suppress flag. allowNewPress=false makes the tree inert to NEW
// presses/wheel this frame (a sibling subtree owns the gesture/modal); an
// in-flight capture this tree already holds is still serviced so drags and
// releases complete. Returns busy = the tree captured/consumed input this
// frame OR is holding capture OR has a modal open — the root uses this to
// stop dispatching to lower subtrees.
func (t *DrumViewTree) DispatchInput(allowNewPress bool) (busy bool) {
	t.allowNewPress = allowNewPress
	t.handleInput() // Phase 3 — reads t.allowNewPress (Step 2)
	t.portal.CleanupClosed()
	suppressClicksUntilRelease = t.suppress
	return t.inputHandled || t.wheelHandled || t.Capturing() || t.PortalHasModal()
}
```

- [ ] **Step 2: Add the `allowNewPress` gate inside `handleInput`** (`drumview_tree.go:460-627`). Add a field `allowNewPress bool` to the struct. In `handleInput`, the existing "start a new press" branch (`pressed && !t.wasPressed`, ~line 501) and the wheel branch (~line 591) must early-return when `!t.allowNewPress` — but the in-flight capture branch (`pressed && t.capturedHandler != nil`, ~line 487) and the release branch (`!pressed && t.wasPressed`, ~line 474) must still run. Concretely, guard the press-start and wheel blocks:

```go
	// New press: only when the root allows this subtree to start one.
	if t.allowNewPress && pressed && !t.wasPressed {
		// ... existing press dispatch ...
	}
```

```go
	// Wheel: only when allowed (a busier sibling subtree gets priority).
	if t.allowNewPress && steps != 0 {
		// ... existing wheel dispatch ...
	}
```

- [ ] **Step 3: Add the accessors** referenced above (skip any that already exist):

```go
func (t *DrumViewTree) Capturing() bool { return t.capturedHandler != nil }

func (t *DrumViewTree) Suppressing() bool { return t.suppress }

func (t *DrumViewTree) PortalHasModal() bool {
	if t.portal == nil {
		return false
	}
	return t.portal.HasModal() // add to overlay_portal*.go if missing (Step 4)
}

func (t *DrumViewTree) ClearCapture() {
	t.capturedHandler = nil
	t.capturedTag = ""
	t.suppress = false
	t.wasPressed = false
}
```

- [ ] **Step 4: Add `OverlayPortal.HasModal()`** in `overlay_portal*.go` if absent. Return true when any open overlay is modal/blocking; if the portal has no modal concept, return `IsOpen()` (any open overlay blocks the sibling) and note the choice in a comment.

- [ ] **Step 5: Build + run the full UI suite.** Behavior must be unchanged (single-tree path still `Tick`+`DispatchInput(true)`).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS modulo the pre-existing branch failures recorded in memory.

---

## Task 3: Split `DrumViewTree.Draw()` into `DrawContent()` + `DrawOverlays()`

**Files:**
- Modify: `src/go/internal/ui/drumview_tree.go:412-457`

So the root can draw ALL subtree content first, then ALL subtree portals — keeping each portal a tree member while letting a full-screen modal from either subtree composite on top of the other subtree's content.

- [ ] **Step 1: Split the method.** The current `Draw` (lines 412-457) walks layers/zones, draws hover glow, then `t.portal.Draw(screen)`. Separate the portal draw:

```go
// Draw renders this tree standalone: content then overlays. Kept for
// direct-tree callers. A RootTree calls DrawContent on all subtrees, then
// DrawOverlays on all subtrees, so portals (and full-screen modals)
// composite above every subtree's content.
func (t *DrumViewTree) Draw(screen *ebiten.Image) {
	t.DrawContent(screen)
	t.DrawOverlays(screen)
}

// DrawContent draws the merged Layer/Zone slice in ascending z plus the
// hover-glow affordance — everything EXCEPT portal overlays.
func (t *DrumViewTree) DrawContent(screen *ebiten.Image) {
	// ... existing lines 413-453 (layer loop + drawHoverGlow) verbatim ...
}

// DrawOverlays draws this tree's portal overlays (menus, dropdowns,
// modals) on top.
func (t *DrumViewTree) DrawOverlays(screen *ebiten.Image) {
	t.portal.Draw(screen)
}
```

  (Confirm the exact tail of `Draw` past line 454 — it should be `t.portal.Draw(screen)`. Move only that into `DrawOverlays`; everything above into `DrawContent`.)

- [ ] **Step 2: Build + run the UI suite.** Unchanged behavior for single-tree callers.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Draw|Pixel|Crop|ZOrder' ./internal/ui/`
Expected: PASS modulo pre-existing failures.

---

## Task 4: Add the `RootTree` composer

**Files:**
- Create: `src/go/internal/ui/root_tree.go`

- [ ] **Step 1: Write `RootTree`.**

```go
package ui

import (
	"image"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// root_tree.go — the single root of the DrumView component hierarchy.
//
// RootTree composes child DrumViewTree subtrees, each of which owns its own
// HitIndex, capture lifecycle, and overlay portal. The root ticks every
// subtree (layout/update), routes each frame's input to exactly ONE subtree
// (capture-holder → modal-owner → top-down by z), and draws all subtree
// CONTENT then all subtree OVERLAYS so portals/modals composite above every
// subtree. Because each subtree's hit areas are spatially disjoint, a
// coordinate in one subtree's region can never reach another — this is what
// makes cross-section input bugs (e.g. a wheel over a synth knob scrolling
// the drum rows) structurally impossible.
//
// Children are ordered by z: lower z draws first / dispatched last; higher z
// draws last (on top) / dispatched first. Adding a future subtree (grid,
// camera, etc. — see the plan's deferred scope) is one AddChild call.
type RootTree struct {
	children []rootChild
	bounds   image.Rectangle
}

type rootChild struct {
	tree *DrumViewTree
	z    int
	name string
}

func NewRootTree() *RootTree { return &RootTree{} }

// AddChild registers a subtree at a z-index. Higher z = drawn on top and
// given input priority in spatial overlaps.
func (r *RootTree) AddChild(name string, tree *DrumViewTree, z int) {
	r.children = append(r.children, rootChild{tree: tree, z: z, name: name})
	sort.SliceStable(r.children, func(i, j int) bool { return r.children[i].z < r.children[j].z })
}

// SetBounds propagates the full DrumView bounds to every subtree (used for
// portal layout so full-screen modals center correctly). Per-zone content is
// confined by each zone's own rect during layout, not by these bounds.
func (r *RootTree) SetBounds(b image.Rectangle) {
	r.bounds = b
	for _, c := range r.children {
		c.tree.SetBounds(b)
	}
}

// Update ticks all subtrees, then dispatches input to exactly one.
func (r *RootTree) Update() {
	for _, c := range r.children {
		c.tree.Tick()
	}
	r.dispatchInput()
	// Suppress is the OR across subtrees so legacy non-tree consumers
	// (splitter.go, textinput.go) see "something is mid-gesture".
	sup := false
	for _, c := range r.children {
		sup = sup || c.tree.Suppressing()
	}
	suppressClicksUntilRelease = sup
}

func (r *RootTree) dispatchInput() {
	// 1) If a subtree holds capture, ONLY it acts (so a drag started in one
	//    subtree is never misread as a new press in another).
	if c := r.capturer(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// 2) Else if a subtree has a modal open, ONLY it acts.
	if c := r.modaler(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// 3) Else fresh dispatch top-down by z: the first subtree that handles
	//    input gates the rest. Disjoint hit areas mean at most one subtree
	//    has a hit at the cursor anyway; this also gives the on-top subtree
	//    priority in any boundary overlap (divider).
	busyAbove := false
	for i := len(r.children) - 1; i >= 0; i-- {
		busy := r.children[i].tree.DispatchInput(!busyAbove)
		busyAbove = busyAbove || busy
	}
}

func (r *RootTree) capturer() *DrumViewTree {
	for _, c := range r.children {
		if c.tree.Capturing() {
			return c.tree
		}
	}
	return nil
}

func (r *RootTree) modaler() *DrumViewTree {
	// Top-down: a modal on a higher subtree wins.
	for i := len(r.children) - 1; i >= 0; i-- {
		if r.children[i].tree.PortalHasModal() {
			return r.children[i].tree
		}
	}
	return nil
}

// Draw composites all subtree content (ascending z), then all subtree
// overlays (ascending z), so portals/modals sit above every subtree.
func (r *RootTree) Draw(screen *ebiten.Image) {
	for _, c := range r.children {
		c.tree.DrawContent(screen)
	}
	for _, c := range r.children {
		c.tree.DrawOverlays(screen)
	}
}

// EnsureLayouts re-runs each subtree's layout+publish pass (used by Draw
// paths that may run without a preceding Update).
func (r *RootTree) EnsureLayouts() {
	for _, c := range r.children {
		c.tree.EnsureLayouts()
	}
}

// ClearCapture/SetFocus convenience for transient-state teardown.
func (r *RootTree) ClearCapture() {
	for _, c := range r.children {
		c.tree.ClearCapture()
	}
}

// Child returns the named subtree (for layout/zone-rect routing and tests).
func (r *RootTree) Child(name string) *DrumViewTree {
	for _, c := range r.children {
		if c.name == name {
			return c.tree
		}
	}
	return nil
}
```

- [ ] **Step 2: Build.**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build (not wired yet).

---

## Task 5: Wire `DrumView` to the root tree + move the audio panel onto its own subtree

**Files:**
- Modify: `drumview.go` (struct), `drumview_ctor.go`, `drumview_draw.go`, `drumview_update.go`, `drumview_layout.go`, `drumview_close_popups.go`

- [ ] **Step 1: Add fields to the `DrumView` struct** (next to `tree *DrumViewTree`):

```go
	// audioTree is the audio-panel subtree: the entire bottom tab section
	// (eqPanelZone + chain + divider + view-switch + its own portal) with
	// its own isolated HitIndex/capture. rootTree composes it with `tree`
	// (the drum-view subtree). See root_tree.go.
	audioTree *DrumViewTree
	rootTree  *RootTree
```

- [ ] **Step 2: In `drumview_ctor.go`, build the subtrees + root.** After `dv.tree` is created and after `dv.eqPanelZone` exists, add:

```go
	dv.audioTree = NewDrumViewTree()
	dv.rootTree = NewRootTree()
	dv.rootTree.AddChild("drumview", dv.tree, 0)   // lower: drawn first / dispatched last
	dv.rootTree.AddChild("audio", dv.audioTree, 1) // upper: drawn last / dispatched first
```

- [ ] **Step 3: Point the panel portal at the audio subtree.** Change `dv.eqPanelZone.SetPortal(dv.tree.Portal())` (~`drumview_ctor.go:391`) to:

```go
	dv.eqPanelZone.SetPortal(dv.audioTree.Portal())
```

- [ ] **Step 4: Register `eqPanelZone` on the audio subtree.** Change the `dv.tree.RegisterZoneVisible(dv.eqPanelZone, ZEQPanel, …)` call (~`drumview_ctor.go:397`) to `dv.audioTree.RegisterZoneVisible(dv.eqPanelZone, ZEQPanel, <same predicate byte-for-byte>)`.

- [ ] **Step 5: Move the divider + view-switch layers to the audio subtree.** Find their `dv.tree.RegisterLayer(...)` calls and change to `dv.audioTree.RegisterLayer(...)`:

```go
	dv.audioTree.RegisterLayer(dv.rowEQDivider)        // ZRowEQDivider = 186
	dv.audioTree.RegisterLayer(newViewSwitchLayer(dv)) // ZViewSwitch  = 150 (mobile)
```

  Leave every other layer on `dv.tree`. (Rationale: only the divider and view-switch overlap/border the panel and must draw above it; the rest live in the rows/transport region.)

- [ ] **Step 6: Replace the Draw dispatch.** In `drumview_draw.go`, change the `EnsureLayouts`/`SetBounds`/`Draw` block (lines 57-90) from `dv.tree` to `dv.rootTree`:

```go
	if dv.rootTree != nil {
		dv.rootTree.EnsureLayouts()
	}
	// ... existing bookkeeping unchanged ...
	if dv.rootTree != nil {
		dv.rootTree.SetBounds(dv.Bounds)
		dv.rootTree.Draw(dst)
	}
```

- [ ] **Step 7: Replace the Update dispatch.** In `drumview_update.go`, change the `SetBounds`/`Update` block (lines 48-50) from `dv.tree` to `dv.rootTree`:

```go
	if dv.rootTree != nil {
		dv.rootTree.SetBounds(dv.Bounds)
		dv.rootTree.Update()
	}
```

  Also OR the wheel-handled check the legacy fallback (camera zoom) consults: replace any `dv.tree.wheelHandled` read with `(dv.tree.wheelHandled || dv.audioTree.wheelHandled)` (add a `WheelHandled()` accessor if the field isn't reachable here).

- [ ] **Step 8: Route the panel zone rect to the audio subtree.** In `drumview_layout.go`, change the eq-panel `SetZoneRect` call from `dv.tree.SetZoneRect("eq-panel", panelRect)` to `dv.audioTree.SetZoneRect("eq-panel", panelRect)`. Then audit ALL eq-panel tree calls: `grep -rn '"eq-panel"' internal/ui/*.go` — every `SetZoneRect`/`LayoutZoneNow` for `"eq-panel"` must target `dv.audioTree` (a lookup on the wrong tree is a silent no-op that strands the panel's HitIndex; see `project_hitindex_publish_chokepoint`).

- [ ] **Step 9: Update transient-state teardown.** In `drumview_close_popups.go:88-98` (`resetTransientTabState`), change `dv.tree.ClearCapture()` + `dv.tree.SetFocus("")` to clear BOTH subtrees:

```go
	if dv.rootTree != nil {
		dv.rootTree.ClearCapture()
	}
	if dv.tree != nil {
		dv.tree.SetFocus("")
	}
	if dv.audioTree != nil {
		dv.audioTree.SetFocus("")
	}
```

- [ ] **Step 10: Audit other `dv.tree` references.** `grep -rn "dv\.tree\.\|\.tree\.Portal\|\.tree\.LayoutZoneNow\|\.tree\.EnsureLayouts" internal/ui/*.go` (non-test). For each, decide: drum-view concern (keep `dv.tree`) or audio-panel concern (switch to `dv.audioTree`), or whole-hierarchy (use `dv.rootTree`). Key ones: portal access by the EQ panel (audio), `LayoutZoneNow("eq-panel")` (audio), forced layouts of transport/timeline/row-rack (drum-view).

- [ ] **Step 11: Build + run the reproduction test — must now PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWheelOverSynthKnobDoesNotScrollRows ./internal/ui/ -v`
Expected: PASS — the wheel at the knob is dispatched only by `audioTree`; `dv.tree`'s `HitIndex.At(panel coord)` returns nothing, so the row rack never sees it.

---

## Task 6: Update the layer z-order guard

**Files:**
- Modify: `src/go/internal/ui/drumview_layer_zorder_test.go`

- [ ] **Step 1: Run to see the mismatch.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'ZOrder|LayerOrder' ./internal/ui/ -v`
Expected: FAIL — `dv.tree` no longer contains `eq-panel`, `view-switch`, `row-eq-divider`.

- [ ] **Step 2: Split the expectation into two subtrees.** Assert `dv.tree`'s ordered layer IDs WITHOUT the three moved entries, and `dv.audioTree`'s ordered layer IDs WITH them (`eq-panel` z=130, `view-switch` z=150, `row-eq-divider` z=186). Keep the z-ascending assertion for both. If the test reaches `dv.tree.layers` directly, add a parallel block for `dv.audioTree.layers`.

- [ ] **Step 3: Run green.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'ZOrder|LayerOrder' ./internal/ui/ -v`
Expected: PASS.

---

## Task 7: Run the full fast suite + fix any new regression

- [ ] **Step 1: UI fast suite.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS except the pre-existing branch failures in memory (`TestColumnPillVisibleOnDesktop`, `TestKebabTapOverlapScrollerActive`, `TestRowControls_NoCramping_AllProfiles`, `TestMobileToDesktopRowControlsUseDesktopLayout`, and the documented full-suite timeout/panic). Watch especially: `zone_input_isolation_discipline_test.go`, `pads_input_regression_test.go`, `hit_index_layout_publish_test.go`, `audio_panel_dispatcher_test.go`, `scene_crop_test.go`, `audio_panel_render_pixels_test.go`, `drumview_zone_clip_test.go`. Any NEW failure is in scope — fix before proceeding. To prove a failure is pre-existing, stash-free check: temporarily revert your edits with the Edit tool (NOT git) only if needed; prefer comparing against the memory list.

- [ ] **Step 2: Timeline + audio fast suites.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline ./internal/audio/...`
Expected: PASS.

---

## Task 8: Discipline tests — lock the isolation so the bug class can't return

**Files:**
- Modify: `src/go/internal/ui/audio_panel_tree_isolation_test.go`

- [ ] **Step 1: Two subtrees, disjoint hit areas.** Add a tree accessor `hitAreaRectsForTest() []image.Rectangle` if the index isn't introspectable.

```go
func TestSubtreeHitAreasAreDisjoint(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()
	panel := dv.panelRectForTest()
	rows := dv.rowsRegionForTest()
	for _, r := range dv.tree.hitAreaRectsForTest() {
		if r.Overlaps(panel) {
			t.Errorf("drum-view subtree hit area %v overlaps audio panel %v — input can cross", r, panel)
		}
	}
	for _, r := range dv.audioTree.hitAreaRectsForTest() {
		if !r.Empty() && r.Overlaps(rows) {
			t.Errorf("audio subtree hit area %v overlaps rows region %v — input can cross", r, rows)
		}
	}
}
```

- [ ] **Step 2: Wheel anywhere in the panel never scrolls rows.** Sweep a grid of points across `panelRectForTest()`, inject a wheel at each, assert `RowOffsetForTest()` invariant.

- [ ] **Step 3: Bidirectional press isolation.** A press in panel whitespace must not change `RowOffsetForTest()` nor toggle any row mute/solo; a press in the rows region must not change the panel's active tab/channel.

- [ ] **Step 4: Capture in one subtree freezes presses in the other.** Begin a knob drag (press on a synth knob → `dv.audioTree.Capturing()` true), inject a press in the rows region, assert it is NOT dispatched (root routes only to the capturer); release; assert normal dispatch resumes. Symmetric: begin a timeline scrub in the drum-view subtree, inject a press in the panel, assert the panel doesn't react.

- [ ] **Step 5: A modal in one subtree blocks the other.** Open the Synth "Save As" dialog (audio subtree portal) → `dv.audioTree.PortalHasModal()` true; inject a press on a drum row; assert NOT dispatched. Close; assert dispatch resumes. Symmetric for a drum-view modal (e.g. keyboard-shortcuts) blocking the panel.

- [ ] **Step 6: Run green.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Subtree|WheelAnywhereInPanel|PressIsolation|CaptureFreezes|ModalBlocks' ./internal/ui/ -v`
Expected: PASS.

---

## Task 9: Verify the cross-cutting surfaces the split could break

**Files:**
- Modify: `src/go/internal/ui/audio_panel_tree_isolation_test.go` + run existing suites.

- [ ] **Step 1: Mobile view-mode swap (panel REPLACES rows).** Mobile profile, `setViewMode(viewModeSynth)` → audio subtree's `eq-panel` publishes hit areas, drum-view row-rack publishes scroll-only; then `setViewMode(viewModeRows)` → audio `eq-panel` invisible → publishes EMPTY hit areas (visibility gate) → a tap where the panel WAS reaches the row rack. This is `pads_input_regression_test.go` across two subtrees — assert no input swallowed by a hidden panel.

```go
func TestMobilePadsTapAfterPanelHiddenAcrossSubtrees(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	dv := newTestMobileDrumViewWithRows(t, 12)
	dv.setViewMode(viewModeSynth)
	dv.updateForTest()
	dv.setViewMode(viewModeRows)
	dv.updateForTest()
	pt := dv.formerPanelMidpointForTest()
	assertPressReachesRowRack(t, dv, pt.X, pt.Y)
}
```

- [ ] **Step 2: Full-screen modal from the panel draws ON TOP of the drum view.** Open Synth "Save As", run a frame, sample `dst.At()` at a point in the rows region that the modal scrim covers; assert it shows the scrim color (proving `DrawOverlays` composited the audio portal above drum-view content). Symmetric: a drum-view keyboard-shortcuts modal draws on top of the panel region.

- [ ] **Step 3: EQ channel dropdown opens, draws on top, dispatches.** Open it (lives in `dv.audioTree.Portal()`), assert its hit areas are in `dv.audioTree`'s index, click an item, assert the channel changed.

- [ ] **Step 4: Splitter resize still works.** The divider's DRAW moved to `audioTree`, but its INPUT still flows through the legacy `LayoutResizeHandler` (z=200, `inputDispatcher`, not the tree). Confirm dragging resizes the panel and the `eq-panel` zone rect follows.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Splitter|Divider|RowEQ|LayoutResize' ./internal/ui/ -v`
Expected: PASS modulo the pre-existing splitter-dark-band WIP failures in memory.

- [ ] **Step 5: Render isolation — panel paints only in the panel rect.** After a Synth-tab frame, sample `dst.At()` along a scanline just above `panel.Min.Y` in the rows region; assert those pixels are NOT changed by the audio subtree pass (compare a frame drawn with the audio subtree's zone visible vs. a control). Confirm `scene_crop_test.go` + `audio_panel_render_pixels_test.go` still pass.

---

## Task 10: Browser / cross-platform smoke (Go-only change, but verify WASM still loads & renders)

**Files:** none (verification only). Use the absolute GO path per the WASM gotcha.

- [ ] **Step 1: Bridge smoke (canonical export catalogue).**

Run (from repo root): `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS — all exports callable.

- [ ] **Step 2: Audio panel render in the real bridge (desktop + mobile).**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/audio_panel_render.browser.test.js`
Expected: PASS.

- [ ] **Step 3: Real-input e2e (knob/drag through the canvas).**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/e2e_real_input_live_edit.browser.test.js`
Expected: PASS.

---

## Self-Review notes (carried into execution)

- **Spec coverage:** entire tab in its own subtree (Task 5 Steps 3-5,8); root tree composes everything, nothing renders/inputs outside it (Tasks 4-5); input isolation (Tasks 1,5,8); render isolation (Task 9 Step 5); bug fixed (Task 1 flips at Task 5 Step 11); made impossible via discipline tests (Task 8); mobile/modal/dropdown/splitter intact (Task 9); z-order guard updated (Task 6); bridge intact (Task 10).
- **Why the two `DrumViewTree` splits:** `Tick`/`DispatchInput` lets the root tick both subtrees but dispatch input to one (preventing a cross-subtree drag from registering as a new press); `DrawContent`/`DrawOverlays` lets a full-screen modal from either subtree draw above the other while its portal stays a tree member. Both keep `Update()`/`Draw()` as back-compat wrappers so existing direct-tree tests don't change.
- **Deferred (next phase):** grid node gestures, camera pan/zoom, splitter, sidebar stay on the legacy `inputDispatcher`. `RootTree.AddChild` is the extension point to bring them in later as additional subtrees.
- **Type consistency check:** `Tick()`, `DispatchInput(bool) bool`, `DrawContent`, `DrawOverlays`, `Capturing()`, `PortalHasModal()`, `Suppressing()`, `ClearCapture()`, `WheelHandled()` on `DrumViewTree`; `NewRootTree`, `AddChild(name,tree,z)`, `SetBounds`, `Update`, `Draw`, `EnsureLayouts`, `ClearCapture`, `Child(name)` on `RootTree`; `OverlayPortal.HasModal()`. Names used identically across tasks.
- **Repo constraints (memory):** do NOT `git add`/commit (the user owns commits); do NOT `git checkout/restore/rm` to undo — revert manually with Edit; expect external-codex churn of unrelated orphan files on this branch — work around, don't repair.
```
