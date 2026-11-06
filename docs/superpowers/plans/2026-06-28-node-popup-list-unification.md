# Node Pop-up List + List-Button Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route the node pop-up (`NodeSidebar`) Logic/Groove dropdown lists through the shared `drawMenuRow` primitive, accented by the node's own color, with their list-backing buttons on `DropdownStyle` — so node lists render identically to the other unified menus.

**Architecture:** Two surgical changes in `game_node_sidebar.go`: (1) the `wireButtons` `mk` closure creates `logic:*` / `groove:*` dropdown-item buttons with `DropdownStyle` instead of `PopupButtonStyle`; (2) `drawDropdownItem` routes through `drawMenuRow(dst, btn, MenuRowSpec{Accent: nodeAccent(), State, Label})` instead of `drawBtn` + a post-hoc selected overlay. Node color stays sourced from `nodeAccent()` (SSOT = `DrumRow.Color`). No new abstraction.

**Tech Stack:** Go 1.23, Ebiten v2, the project's `internal/ui` design tokens. Bundled Go at `.tools/go/bin/go`.

## Global Constraints

- Use the bundled Go: `.tools/go/bin/go` (never system `go`).
- Verify with scoped runs (the full suite is disrupted by concurrent external churn in the `audio` package / instrument-label store): `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui` (must exit 0) and `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run '<pattern>'`.
- **No git staging or commits.** The user forbids `git add` / `git commit`. Each task ends with a test checkpoint, not a commit. Leave all work uncommitted.
- No new inline `color.RGBA{}` / `color.NRGBA{}` literals in non-test `internal/ui/` files — use existing tokens / `nodeAccent()`. Guarded by `token_discipline_test.go`.
- Node color MUST stay sourced from `NodeSidebar.nodeAccent()` (resolves `game.nodeRows[node.ID]` → `drum.Rows[row].Color`, fallback `colAccent`). Do not duplicate color logic.
- Only touch `game_node_sidebar.go` (+ a new/extended test file). Out of scope: steppers (`ComponentButtonStepper`), Logic/Groove selector buttons, Audible/Move, Close, value pills, section headers.
- `IconID`/`TextRole`/`MenuRowSpec` are pre-existing (`MenuRowSpec`/`drawMenuRow` in `menu_chrome.go`).

---

### Task 1: Dropdown-item buttons use `DropdownStyle`

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`wireButtons` `mk` closure, ~line 388-396)
- Test: `src/go/internal/ui/node_sidebar_dropdown_style_test.go` (create)

**Interfaces:**
- Consumes: `DropdownStyle`, `PopupButtonStyle` (theme.go); `NewButton`; the existing `mk(id, label, onClick)` closure inside `wireButtons`.
- Produces: dropdown-item buttons (`id` prefixed `logic:` or `groove:`) created with `DropdownStyle`; all other node buttons unchanged (`PopupButtonStyle` / role specs).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/node_sidebar_dropdown_style_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// After unification, the Logic/Groove dropdown LIST items use DropdownStyle
// (the shared menu list style) while structural node controls (steppers) keep
// their role-specific PopupButtonStyle.
func TestNodeDropdownItemsUseDropdownStyle(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)
	node := g.nodeAt(7, 7)
	if node == nil {
		t.Fatal("failed to add test node")
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.updateBeatInfos()
	g.sidebar.layout()

	// A logic dropdown item must be DropdownStyle.
	const itemID = "logic:probability"
	b := g.sidebar.btns[itemID]
	if b == nil {
		t.Fatalf("dropdown item %q missing after layout", itemID)
	}
	bs, ok := b.Style.(ButtonStyle)
	if !ok {
		t.Fatalf("dropdown item %q Style is not a ButtonStyle: %T", itemID, b.Style)
	}
	if bs != DropdownStyle {
		t.Errorf("dropdown item %q: Style = %+v, want DropdownStyle %+v", itemID, bs, DropdownStyle)
	}

	// A stepper must NOT have been switched — it keeps its role spec.
	if sb := g.sidebar.btns["vol-"]; sb != nil {
		if ss, ok := sb.Style.(ButtonStyle); ok && ss == DropdownStyle {
			t.Error("stepper vol- was incorrectly switched to DropdownStyle; role specs must be preserved")
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestNodeDropdownItemsUseDropdownStyle`
Expected: FAIL — the item's Style is `PopupButtonStyle`, not `DropdownStyle`.

Note: if `b.Style` does not type-assert to `ButtonStyle` (the test's first `ok` check fails), inspect `NewButton`'s `Style` field type and adjust the assertion to the real concrete type — keep the intent (item is the DropdownStyle visual, stepper is not). Report the adjustment.

- [ ] **Step 3: Implement — `mk` picks the style by id prefix**

In `wireButtons`'s `mk` closure (`game_node_sidebar.go`), the button is created with `NewButton(label, PopupButtonStyle, …)`. Change the creation branch to select the style by id prefix. Replace:

```go
		if _, ok := sb.btns[id]; !ok {
			sb.btns[id] = NewButton(label, PopupButtonStyle, func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			})
		} else {
```

with:

```go
		if _, ok := sb.btns[id]; !ok {
			// Logic/Groove dropdown LIST items use the shared menu list style
			// (DropdownStyle) so they render identically to the other menus via
			// drawMenuRow; every other node control keeps its role spec.
			itemStyle := PopupButtonStyle
			if strings.HasPrefix(id, "logic:") || strings.HasPrefix(id, "groove:") {
				itemStyle = DropdownStyle
			}
			sb.btns[id] = NewButton(label, itemStyle, func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			})
		} else {
```

(`strings` is already imported in this file — it's used by the `HasSuffix("-"/"+")` checks in the same closure.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestNodeDropdownItemsUseDropdownStyle`
Expected: PASS.

- [ ] **Step 5: Checkpoint — vet + existing node-sidebar tests**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui` (expect exit 0)
Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Sidebar|NodeSidebar|node_sidebar|Dropdown'`
Expected: PASS. (No commit.)

---

### Task 2: `drawDropdownItem` routes through `drawMenuRow`

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`drawDropdownItem`, ~line 1468-1478)
- Test: `src/go/internal/ui/node_sidebar_dropdown_style_test.go` (extend — created in Task 1)

**Interfaces:**
- Consumes: `drawMenuRow`, `MenuRowSpec`, `menuItemRest/Hover/Active` (menu_chrome.go); `sb.nodeAccent()`; `sb.btns`, `sb.rects`, `sb.inViewport`; `drawRect` (swappable var, drawing.go:34) for the test.
- Produces: `drawDropdownItem` renders each dropdown row via `drawMenuRow` (background accent + keycap `btn.Draw` + label) with the node color as `Accent`; no behavior change to selection.

- [ ] **Step 1: Write the failing test (extend the Task 1 file)**

Append to `src/go/internal/ui/node_sidebar_dropdown_style_test.go`:

```go
import "image" // (add to the existing import block if not present)
import "image/color"

// drawDropdownItem must route through drawMenuRow, which paints the node-color
// active stripe for the SELECTED item (drawMenuItemBackgroundAccent → a filled
// rect of exactly nodeAccent at the row's left edge). We intercept drawRect to
// confirm a node-accent-colored fill lands inside the selected item's rect.
func TestNodeDropdownItemDrawsNodeAccentStripe(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)
	node := g.nodeAt(7, 7)
	if node == nil {
		t.Fatal("failed to add test node")
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.updateBeatInfos()
	g.sidebar.layout()

	// A fresh regular node has empty LogicKind → the selected item is "logic:".
	const selID = "logic:"
	r, ok := g.sidebar.rects[selID]
	if !ok || r.Empty() {
		t.Fatalf("selected dropdown item %q has no rect", selID)
	}
	if !g.sidebar.inViewport(r) {
		t.Skipf("selected item %q scrolled out of viewport; cannot assert stripe", selID)
	}
	want := color.RGBAModel.Convert(g.sidebar.nodeAccent()).(color.RGBA)

	// Intercept drawRect to record filled rects of the node-accent color.
	orig := drawRect
	defer func() { drawRect = orig }()
	found := false
	drawRect = func(dst *ebitenImage, rr image.Rectangle, c color.Color, filled bool) {
		if filled && color.RGBAModel.Convert(c).(color.RGBA) == want && !rr.Intersect(r).Empty() {
			found = true
		}
		orig(dst, rr, c, filled)
	}

	dst := newTestImage(64, 256) // any non-nil *ebiten.Image sized to the row
	g.sidebar.drawDropdownItem(dst, selID, true)

	if !found {
		t.Errorf("drawDropdownItem(%q, selected) drew no node-accent stripe (want a filled %v rect inside %v)", selID, want, r)
	}
}
```

Note on placeholders in the snippet: `*ebitenImage` is shorthand — use the real type `*ebiten.Image` and add the `github.com/hajimehoshi/ebiten/v2` import. `newTestImage(w,h)` is shorthand — replace with the repo's real way to make an image in tests (e.g. `ebiten.NewImage(64, 256)`, the same call other `internal/ui` tests use; grep `ebiten.NewImage(` in `internal/ui/*_test.go` for the canonical form). The drawRect signature must match drawing.go:34 exactly: `func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool)`. Consolidate the two `import` lines into the file's existing import block (no duplicate `import` keywords).

- [ ] **Step 2: Run the test to verify it fails or errors**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestNodeDropdownItemDrawsNodeAccentStripe`
Expected: With the OLD `drawDropdownItem` (`drawBtn` then overlay) the stripe IS still drawn for selected items, so this test may already PASS against the old code — that is acceptable: it is a guard that the node-accent stripe survives the refactor. The PRIMARY new-path guard is Task 1's DropdownStyle assertion. If the test fails to COMPILE (placeholder types), fix the shorthand per the Step 1 note first. Record whether it passed pre-implementation.

- [ ] **Step 3: Implement — route `drawDropdownItem` through `drawMenuRow`**

Replace `drawDropdownItem` (`game_node_sidebar.go`):

```go
func (sb *NodeSidebar) drawDropdownItem(dst *ebiten.Image, id string, selected bool) {
	sb.drawBtn(dst, id)
	if !selected {
		return
	}
	if r, ok := sb.rects[id]; ok && !r.Empty() && sb.inViewport(r) {
		drawMenuItemBackgroundAccent(dst, r, menuItemActive, sb.nodeAccent())
	}
}
```

with:

```go
// drawDropdownItem renders one Logic/Groove dropdown LIST row through the shared
// drawMenuRow primitive (same as every other menu list): background accent +
// keycap chrome + label. The node's own color (nodeAccent — SSOT DrumRow.Color)
// is the accent, so the selected row shows a node-color stripe and a hovered row
// a node-color tint. The clip guard keeps off-viewport rows from drawing.
func (sb *NodeSidebar) drawDropdownItem(dst *ebiten.Image, id string, selected bool) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	r, ok := sb.rects[id]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	state := menuItemRest
	if selected {
		state = menuItemActive
	} else if b.hovered || b.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, b, MenuRowSpec{
		Accent: sb.nodeAccent(),
		State:  state,
		Label:  b.Text,
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestNodeDropdownItem'`
Expected: PASS (both `TestNodeDropdownItemsUseDropdownStyle` and `TestNodeDropdownItemDrawsNodeAccentStripe`).

- [ ] **Step 5: Checkpoint — vet + node-sidebar + menu-chrome tests**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui` (expect exit 0)
Run: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Sidebar|NodeSidebar|node_sidebar|Dropdown|MenuChrome|MenuRow|Undo'`
Expected: PASS. Pay special attention to `undo_node_sidebar_test.go` (exercises logic/groove dropdown items) and `sidebar_*` tests. If a test asserted the old `drawBtn`-tinted border on a dropdown item, that assertion is now stale — confirm whether it's testing the list item (update) or another control (leave). (No commit.)

---

### Task 3: Verification

**Files:** none (verification only).

- [ ] **Step 1: Full node-sidebar + menu sweep (scoped, churn-resistant)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Sidebar|NodeSidebar|node_sidebar|Dropdown|Undo|Inst|Context|Subdiv|Template|Overflow|MenuChrome|MenuRow'`
Expected: PASS, except any pre-existing failure caused by external churn (e.g. `TestOverflowMenu_TemplatePageListsGenres` — a template-rename mismatch unrelated to this work; and instrument-label tests polluted by the externally-edited global label store, which pass in isolation). Confirm any failure is external by re-running that single test alone.

- [ ] **Step 2: Design-token guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS. The change adds no inline color literals (uses `nodeAccent()` + existing tokens), so `TestTokenDiscipline` is unaffected.

- [ ] **Step 3: vet**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui`
Expected: exit 0.

- [ ] **Step 4: Summary**

Confirm: node Logic/Groove dropdown lists route through `drawMenuRow` with `nodeAccent()` as accent; dropdown-item buttons are `DropdownStyle`; steppers/selectors/audible/move/close/value-pills/section-headers unchanged; node-color SSOT (`nodeAccent` → `DrumRow.Color`) preserved; nothing committed.

---

## Self-Review Notes

**Spec coverage:**
- Lists → `drawMenuRow` with node accent → Task 2. ✓
- List-backing buttons → `DropdownStyle` → Task 1. ✓
- Node color SSOT via `nodeAccent()` → both tasks pass `sb.nodeAccent()` as `Accent`; never duplicate color logic. ✓
- Out-of-scope controls untouched → only `wireButtons` `mk` (prefix-gated) and `drawDropdownItem` change; steppers/selectors/etc. unaffected. ✓
- No new abstraction → uses existing `drawMenuRow`/`MenuRow Spec`/`Button.Draw` seams. ✓
- Testing (DropdownStyle assertion, node-accent stripe via drawRect interception, vet, design guards) → Tasks 1-3. ✓

**Type consistency:** `MenuRowSpec` fields (`Accent`, `State`, `Label`) match `menu_chrome.go`. `menuItemRest/Hover/Active`, `nodeAccent()`, `btns`, `rects`, `inViewport`, `drawRect` all exist (verified in source). `strings` already imported in `game_node_sidebar.go`.

**Flagged for implementer (confirmations, not placeholders):**
- Task 1 Step 2: confirm `b.Style` type-asserts to `ButtonStyle`; adjust the assertion to the real type if not.
- Task 2 Step 1: replace the `*ebitenImage`/`newTestImage` shorthand with the repo's real `*ebiten.Image` + `ebiten.NewImage(...)` (grep existing `internal/ui` tests), and fold imports into one block; match `drawRect`'s exact signature.
- Task 2 Step 2: the stripe guard may pass pre-implementation (old path also drew the selected stripe) — that is fine; Task 1 is the new-path discriminator.
