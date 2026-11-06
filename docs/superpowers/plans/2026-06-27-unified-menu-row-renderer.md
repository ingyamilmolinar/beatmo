# Unified Menu-Row Renderer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every overlay list menu (instrument, row context, subdivision, template/overflow) render rows through one shared `drawMenuRow` primitive so styling and animation (keycap chrome, press-depth travel, hover glow, accent background) are identical, while each menu keeps its own behavior.

**Architecture:** Add a `MenuRowSpec` struct + `drawMenuRow` function in `menu_chrome.go` that generalizes the existing `drawInstRow` body (background accent → `btn.Draw` keycap chrome → optional swatch/icon → highlights → label). Route the four overlay menus' per-row draw functions through it. The template/overflow menu additionally gets a button-persistence refactor (it currently rebuilds buttons every frame, which prevents animation state from accumulating).

**Tech Stack:** Go 1.23, Ebiten v2, the project's `internal/ui` design-token system. Bundled Go at `.tools/go/bin/go`.

## Global Constraints

- Use the bundled Go: `.tools/go/bin/go` (never system `go`).
- Fast test command: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`.
- **No git staging or commits.** The user forbids `git add` / `git commit`. Each task ends with a verification checkpoint (run tests), NOT a commit. Leave all work uncommitted.
- No new inline `color.RGBA{}` / `color.NRGBA{}` literals in `internal/ui/` — use existing tokens (`colAccent`, `colMenuIcon`, `colTextPrimary`, `colTextAccent`, etc.). Guarded by `token_discipline_test.go`.
- No forbidden glyphs; icons go through `IconID` + `DrawIcon`. Guarded by design tests.
- Node sidebar (`game_node_sidebar.go`) is OUT OF SCOPE — do not touch it.
- Behavior of each menu's `onClick` must not change. Only draw + animation is unified.
- `IconID` is a string-based type; its zero value is `""`. `TextRole` zero value is treated as "unset" → default `RoleBody` (note: confirm `RoleBody` is not literally the zero constant; see Task 1 Step 1a).

---

### Task 1: Add the shared `drawMenuRow` primitive

**Files:**
- Modify: `src/go/internal/ui/menu_chrome.go` (append after `drawMenuItemBackgroundAccent`, ~line 73)
- Test: `src/go/internal/ui/menu_row_test.go` (create)

**Interfaces:**
- Consumes: existing `menu_chrome.go` helpers (`drawMenuItemBackgroundAccent`, `menuRowIconRect`, `menuRowLabelX`), `Button.Draw`, `Button.Rect`, `Button.Text`, `drawRoundedRect`, `DrawIcon`, `DrawTextStyled`, `StyledTextHeight`, `StyledTextWidth`, `drawButtonHighlights`, `instMenuSwatchSz`, tokens (`colAccent`, `colMenuIcon`, `colTextPrimary`), `RadiusXXS`, `SpaceMD`, `SpaceSM`.
- Produces: `MenuRowSpec` struct and `func drawMenuRow(dst *ebiten.Image, btn *Button, spec MenuRowSpec)`.

- [ ] **Step 1a: Confirm `RoleBody` is not the zero value of `TextRole`**

Run: `cd src/go && grep -n "RoleBody\|RoleCaption\|TextRole =\|RoleUnset\|iota" internal/ui/text*.go internal/ui/*role*.go 2>/dev/null | head`

Expected: Find the `TextRole` const block. If `RoleBody` is the first iota value (== 0), the `if role == 0 { role = RoleBody }` default below is a harmless no-op and still correct. If there is a distinct zero/unset role, the default still works because spec callers always set `LabelRole` explicitly OR want `RoleBody`. No code change needed; this step is to confirm the default is safe.

- [ ] **Step 2: Write the failing test**

Create `src/go/internal/ui/menu_row_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawMenuRow must render every spec variant without panicking and must
// advance the button's press animation (it draws the keycap via btn.Draw).
func TestDrawMenuRowVariants(t *testing.T) {
	dst := ebiten.NewImage(120, 40)
	mk := func() *Button {
		b := NewButton("Kick", DropdownStyle, nil)
		b.SetRect(image.Rect(0, 0, 120, 28))
		return b
	}

	cases := []struct {
		name string
		spec MenuRowSpec
	}{
		{"rest-plain", MenuRowSpec{State: menuItemRest, Label: "Kick"}},
		{"hover-icon", MenuRowSpec{State: menuItemHover, IconID: IconRows, Label: "Bach — Toccata"}},
		{"active-stripe", MenuRowSpec{State: menuItemActive, Accent: colAccent, Label: "8"}},
		{"swatch", MenuRowSpec{State: menuItemRest, Swatch: colAccent, Label: "Snare"}},
		{"centered", MenuRowSpec{State: menuItemActive, CenterLabel: true, Label: "16", LabelColor: colTextAccent}},
		{"highlighted", MenuRowSpec{State: menuItemHover, Highlights: []int{0, 1}, Label: "Kick"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			drawMenuRow(dst, mk(), c.spec) // must not panic
		})
	}
}

// drawMenuRow must tick the press animation: a button drawn while its press
// target is engaged advances pressDepth toward 1 across successive draws.
func TestDrawMenuRowAdvancesPressAnim(t *testing.T) {
	dst := ebiten.NewImage(120, 40)
	b := NewButton("Kick", DropdownStyle, nil)
	b.SetRect(image.Rect(0, 0, 120, 28))
	b.pressTarget = 1 // simulate a held press
	start := b.pressDepth
	for i := 0; i < 5; i++ {
		drawMenuRow(dst, b, MenuRowSpec{State: menuItemHover, Label: "Kick"})
	}
	if b.pressDepth <= start {
		t.Fatalf("drawMenuRow did not advance press animation: depth %v <= start %v", b.pressDepth, start)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestDrawMenuRow`
Expected: FAIL — `undefined: MenuRowSpec` / `undefined: drawMenuRow`.

- [ ] **Step 4: Implement `MenuRowSpec` + `drawMenuRow`**

Append to `src/go/internal/ui/menu_chrome.go` (after `drawMenuItemBackgroundAccent`, before `drawMenuSeparator`):

```go
// MenuRowSpec describes the variable content of one list-menu row. The styling
// and animation (keycap chrome via btn.Draw, press-depth travel, hover glow,
// accent background/stripe) are fixed and identical for every menu; only these
// fields vary. Behavior (open/close/toggle) lives in the caller's onClick, not
// here. Shared by the instrument menu, row context menu, subdivision selector,
// and template/overflow menu so styling + animation are byte-identical.
type MenuRowSpec struct {
	Accent      color.Color   // hover/active tint + active stripe; nil → colAccent
	State       menuItemState // rest / hover / active (caller's selection model)
	Swatch      color.Color   // optional leading rounded-square swatch; nil → none
	IconID      IconID        // optional leading icon; "" → none
	IconTint    color.Color   // icon tint; nil → colMenuIcon
	Highlights  []int         // optional fuzzy-match highlight runs (rune indices)
	Label       string        // row text (drawn here; btn.Text is blanked during btn.Draw)
	LabelColor  color.Color   // nil → colTextPrimary
	LabelRole   TextRole      // zero → RoleBody
	CenterLabel bool          // true → center label in the row (subdiv); else left-align
}

// drawMenuRow renders one list-menu row with the unified styling + animation:
// background accent → keycap chrome (btn.Draw with blanked text, which also
// advances AdvancePressAnim) → optional leading swatch or icon → fuzzy-match
// highlights → label. Swatch wins the leading slot over an icon when both set
// (mirrors the instrument menu, which has a swatch and no icon).
func drawMenuRow(dst *ebiten.Image, btn *Button, spec MenuRowSpec) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	accent := spec.Accent
	if accent == nil {
		accent = colAccent
	}
	// 1. Per-state background (rest = nothing, hover = tint, active = stripe+tint).
	drawMenuItemBackgroundAccent(dst, r, spec.State, accent)

	// 2. Keycap chrome + press/hover animation. Blank the text so the label is
	// drawn by us below (centered/iconned variants need custom placement).
	saved := btn.Text
	btn.Text = ""
	btn.Draw(dst)
	btn.Text = saved

	role := spec.LabelRole
	if role == 0 {
		role = RoleBody
	}
	labelCol := spec.LabelColor
	if labelCol == nil {
		labelCol = colTextPrimary
	}
	th := StyledTextHeight(role)
	ty := r.Min.Y + (r.Dy()-th)/2

	// 3. Leading element: swatch takes the slot if present, else an optional icon.
	labelX := r.Min.X + SpaceMD
	switch {
	case spec.Swatch != nil:
		sz := instMenuSwatchSz
		sx := r.Min.X + SpaceSM
		sy := r.Min.Y + (r.Dy()-sz)/2
		sr := image.Rect(sx, sy, sx+sz, sy+sz)
		drawRoundedRect(dst, sr, spec.Swatch, RadiusXXS, true)
		labelX = sr.Max.X + SpaceSM
	case spec.IconID != "":
		tint := spec.IconTint
		if tint == nil {
			tint = colMenuIcon
		}
		DrawIcon(dst, spec.IconID, menuRowIconRect(r), tint)
		labelX = menuRowLabelX(r)
	}

	// 4. Centered label (subdivision selector) overrides the leading-x layout.
	if spec.CenterLabel {
		tw := StyledTextWidth(spec.Label, role)
		labelX = r.Min.X + (r.Dx()-tw)/2
	}

	// 5. Fuzzy-match highlights behind the label, then the label itself.
	if len(spec.Highlights) > 0 {
		drawButtonHighlights(dst, spec.Label, spec.Highlights, labelX, ty, 1.0)
	}
	DrawTextStyled(dst, spec.Label, labelX, ty, role, labelCol)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestDrawMenuRow`
Expected: PASS (both `TestDrawMenuRowVariants` and `TestDrawMenuRowAdvancesPressAnim`).

- [ ] **Step 6: Checkpoint — package still compiles & existing menu tests pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'TestDrawMenuRow|TestMenuChrome|MenuChrome'`
Expected: PASS. (No commit — leave uncommitted.)

---

### Task 2: Route instrument + category rows through `drawMenuRow` (regression anchor)

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go` (`drawInstRow` 1851-1895, `drawCategoryRow` 1899-1924)

**Interfaces:**
- Consumes: `drawMenuRow`, `MenuRowSpec` (Task 1); existing `m.instColorFor`, `m.menuAccent`, `m.ActiveInstrumentID`, `btn.Highlights`, `btn.Style`, `PopupButtonStyle`.
- Produces: no new symbols. This is the visual regression anchor — the instrument menu must look identical after the refactor.

- [ ] **Step 1: Replace `drawInstRow` body with a `drawMenuRow` call**

In `drumview_overlay_inst_comp.go`, replace the body of `drawInstRow` (keep the signature and the doc comment, update the comment's wording) with:

```go
func (m *InstrumentMenuComponent) drawInstRow(dst *ebiten.Image, btn *Button, id string) {
	if btn.Rect().Empty() {
		return
	}
	// LIVE selection (ActiveInstrumentID) → exactly one row reads active; the
	// highlight follows audition picks while the menu stays open.
	state := menuItemRest
	if id != "" && id == m.ActiveInstrumentID() {
		state = menuItemActive
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	accent := m.instColorFor(id)
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent:     accent,
		State:      state,
		Swatch:     accent, // instrument swatch == its accent color
		Highlights: btn.Highlights,
		Label:      btn.Text,
	})
}
```

- [ ] **Step 2: Replace `drawCategoryRow` body with a `drawMenuRow` call**

```go
func (m *InstrumentMenuComponent) drawCategoryRow(dst *ebiten.Image, btn *Button) {
	if btn.Rect().Empty() {
		return
	}
	state := menuItemRest
	if btn.Style == PopupButtonStyle {
		// PopupButtonStyle is set on the active category/Favorites row.
		state = menuItemActive
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		Accent: m.menuAccent(),
		State:  state,
		Label:  btn.Text,
	})
}
```

Note: previously `drawInstRow` placed the swatch at `r.Min.X + SpaceSM` and the label at `swatchRect.Max.X + SpaceSM` — identical to `drawMenuRow`'s swatch branch. `drawCategoryRow` placed the label at `r.Min.X + SpaceMD` — identical to `drawMenuRow`'s no-leading-element default. So both are pixel-identical.

- [ ] **Step 3: Run the instrument-menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Inst|Instrument'`
Expected: PASS (no behavior change).

- [ ] **Step 4: Checkpoint — full UI package compiles & passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`
Expected: PASS. (No commit.)

---

### Task 3: Route the subdivision selector through `drawMenuRow` (gains keycap + animation)

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_subdiv_comp.go` (`drawSubdivRow` 288-315)
- Test: `src/go/internal/ui/drumview_overlay_subdiv_comp_test.go` (extend)

**Interfaces:**
- Consumes: `drawMenuRow`, `MenuRowSpec`; `s.props.Current`, `s.props.Options`, `colTextAccent`.
- Produces: no new symbols. Subdiv rows now animate (press travel + hover glow).

- [ ] **Step 1: Replace `drawSubdivRow` body with a `drawMenuRow` call**

```go
func (s *SubdivMenuComponent) drawSubdivRow(dst *ebiten.Image, btn *Button, v int) {
	if btn.Rect().Empty() {
		return
	}
	// Current subdivision → active; hover/press → hover.
	state := menuItemRest
	var labelCol color.Color // nil → colTextPrimary in drawMenuRow
	if v == s.props.Current {
		state = menuItemActive
		labelCol = colTextAccent
	} else if btn.hovered || btn.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, btn, MenuRowSpec{
		State:       state,
		Label:       btn.Text,
		LabelColor:  labelCol,
		CenterLabel: true,
	})
}
```

Ensure the file imports `image/color` (it likely already does; if `goimports`/build complains it's unused or missing, add/remove accordingly). Verify with the build in Step 3.

- [ ] **Step 2: Add an animation-persistence assertion for subdiv**

The subdiv component persists `s.buttons` and draws via `btn.Draw` now. Add to `drumview_overlay_subdiv_comp_test.go`:

```go
func TestSubdivRowAdvancesPressAnimAfterRefactor(t *testing.T) {
	s := &SubdivMenuComponent{}
	s.SetScreenBounds(image.Rect(0, 0, 200, 400))
	s.SetProps(SubdivMenuProps{Options: []int{4, 8, 16}, Current: 8})
	s.Open()
	dst := ebiten.NewImage(200, 400)
	if len(s.buttons) == 0 {
		t.Fatal("no subdiv buttons built")
	}
	b := s.buttons[0]
	b.pressTarget = 1
	start := b.pressDepth
	for i := 0; i < 5; i++ {
		s.Draw(dst)
	}
	if b.pressDepth <= start {
		t.Fatalf("subdiv row did not animate after drawMenuRow refactor: %v <= %v", b.pressDepth, start)
	}
}
```

(If `SetScreenBounds` / `SetProps` / `Open` signatures differ, match the existing test setup in this file — read the top of `drumview_overlay_subdiv_comp_test.go` for the canonical constructor pattern and reuse it.)

- [ ] **Step 3: Run subdiv tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run Subdiv`
Expected: PASS. The existing `drumview_overlay_subdiv_comp_test.go:109` assertion ("Style should be DropdownStyle; active highlight is drawn via drawMenuItemBackground") still holds — buttons remain `DropdownStyle` and the active highlight still comes from the shared background helper (now via `drawMenuRow`). If that test's comment references the old flat path and now fails, update only its comment string, not its assertion.

- [ ] **Step 4: Checkpoint — full UI package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`
Expected: PASS. (No commit.)

---

### Task 4: Route the row context menu through `drawMenuRow` (gains keycap + animation)

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`drawContextMenuItem` closure ~514-542)

**Interfaces:**
- Consumes: `drawMenuRow`, `MenuRowSpec`; `dv.contextMenuItemIcons[btn]` (string icon name), `dv.contextMenuAccent`, `btn.TextColor`, `IconID`, `IconCircle`.
- Produces: no new symbols. Context menu rows now animate; per-item text color (red Delete) preserved.

- [ ] **Step 1: Replace the `drawContextMenuItem` closure body with a `drawMenuRow` call**

The current closure (inside the larger draw function) does background + icon (via `drawContextMenuIcon`) + label with per-item `TextColor`. Replace its body with:

```go
	drawContextMenuItem := func(i int, btn *Button) {
		if btn.Rect().Empty() {
			return
		}
		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		// Resolve the context menu's string icon name to an IconID, mirroring
		// drawContextMenuIcon's mapping (the "color" entry uses IconCircle).
		var iconID IconID
		if ico := dv.contextMenuItemIcons[btn]; ico != "" {
			iconID = IconID(ico)
			if ico == "color" {
				iconID = IconCircle
			}
		}
		// Preserve per-item label color (e.g. red Delete).
		var labelCol color.Color // nil → colTextPrimary
		if btn.TextColor != nil {
			labelCol = btn.TextColor
		}
		drawMenuRow(dst, btn, MenuRowSpec{
			Accent:     dv.contextMenuAccent(),
			State:      state,
			IconID:     iconID,
			IconTint:   colMenuIcon,
			Label:      btn.Text,
			LabelColor: labelCol,
		})
	}
```

Notes:
- `drawMenuRow`'s icon path uses `menuRowIconRect(r)` and `menuRowLabelX(r)` — exactly what `contextMenuItemGeom(i)` returns, so icon + label placement is unchanged. The `i` parameter is now unused inside the closure; keep the closure signature `func(i int, btn *Button)` if the call site passes `i`, and add `_ = i` only if the compiler flags it (closures don't flag unused params, so no change needed).
- `drawContextMenuIcon` and `contextMenuItemGeom` may become unused after this change. If the build reports them unused, leave `contextMenuItemGeom` if other code uses it (grep first); remove `drawContextMenuIcon` only if grep shows zero remaining callers. Verify in Step 2.

- [ ] **Step 2: Check for now-unused helpers**

Run: `cd src/go && grep -rn "drawContextMenuIcon\|contextMenuItemGeom" internal/ui/*.go | grep -v _test`
Expected: If the only remaining reference to `drawContextMenuIcon` is its own definition, delete the function. If `contextMenuItemGeom` has no remaining non-definition references, delete it too. If either still has callers, leave it. (Go fails the build on unused *functions* only if they are unexported and unreferenced? No — Go does NOT error on unused functions, only unused imports/locals. So removal is optional cleanup; do it to keep the file tidy, but it is not required for compilation.)

- [ ] **Step 3: Run context menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Context|ContextMenu'`
Expected: PASS. Pay attention to any test asserting the Color/Rename icon identity (`contextMenuItemIcons[btn] == "pencil"` at line 1236 is unrelated input logic and untouched).

- [ ] **Step 4: Checkpoint — full UI package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`
Expected: PASS. (No commit.)

---

### Task 5: Persist overflow buttons + route the template/overflow menu through `drawMenuRow`

**Files:**
- Modify: `src/go/internal/ui/drumview.go` (struct field, ~near `overflowMenuScroll` at line 653)
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowPopupBtns` 876-904, `drawOverflowMenu` 913-979, and the two other `overflowPopupBtns` call sites at 602 & 629)
- Test: `src/go/internal/ui/template_menu_test.go` (extend) or `template_menu_scroll_test.go`

**Interfaces:**
- Consumes: `drawMenuRow`, `MenuRowSpec`; `dv.overflowItems()`, `dv.overflowPage`, `dv.contextMenuAccent`, `dv.overflowMenuScroll`, `NewButton`, `DropdownStyle`, `PopupButtonStyle`, `closeButtonRect`, `touchMinTargetPx`, `insetRect`.
- Produces: a persisted `dv.overflowBtns []*Button` field and a rebuild method `dv.rebuildOverflowBtns()`. `overflowPopupBtns` is retained as a thin wrapper returning `dv.overflowBtns` so existing call sites (input handler at 602, 629) keep working.

**Background:** `overflowPopupBtns` currently calls `NewButton` on every invocation (from draw AND input). New `Button` objects reset `pressDepth`/`hovered` each frame, so press animation can never accumulate. The fix: build the buttons once per layout (open / page switch / item-count change) into `dv.overflowBtns`, and only update their rects each frame for scroll.

- [ ] **Step 1: Write the failing animation-persistence test**

Add to `src/go/internal/ui/template_menu_test.go` (match the file's existing DrumView test-harness setup — read the top of the file for the canonical `newTestDrumView`-style constructor and reuse it; the snippet below assumes a helper `dv` that is laid out and a way to open the overflow menu to the Templates page):

```go
// After the persistence refactor, opening the template (overflow) menu and
// drawing repeatedly must keep the SAME *Button instances so press animation
// accumulates across frames (was: NewButton every frame, animation impossible).
func TestOverflowButtonsPersistAcrossFrames(t *testing.T) {
	dv := newTestDrumViewForOverflow(t) // see existing template_menu_test helpers
	dv.openOverflowMenuPortal()
	dv.overflowPage = 1 // Templates page
	dv.rebuildOverflowBtns()

	first := dv.overflowBtns
	if len(first) == 0 {
		t.Fatal("no overflow buttons built")
	}
	firstPtr := first[1] // index 0 is the Back row; pick a template row

	dst := ebiten.NewImage(400, 600)
	firstPtr.pressTarget = 1
	start := firstPtr.pressDepth
	for i := 0; i < 5; i++ {
		dv.drawOverflowMenu(dst)
	}
	if dv.overflowBtns[1] != firstPtr {
		t.Fatal("overflow buttons were recreated across frames (animation state lost)")
	}
	if firstPtr.pressDepth <= start {
		t.Fatalf("overflow row did not animate: %v <= %v", firstPtr.pressDepth, start)
	}
}
```

If `template_menu_test.go` has no reusable open-and-layout helper, mirror the setup already used by `TestOverflow*`/`TestTemplate*` tests in that file (read it first; do NOT invent a new harness).

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestOverflowButtonsPersist`
Expected: FAIL — `dv.overflowBtns undefined` and/or `dv.rebuildOverflowBtns undefined`.

- [ ] **Step 3: Add the persisted field**

In `src/go/internal/ui/drumview.go`, next to `overflowMenuScroll` (line ~653), add:

```go
	overflowBtns []*Button // persisted overflow/template rows (rebuilt on open/page switch); rects updated per frame for scroll
```

- [ ] **Step 4: Add `rebuildOverflowBtns` and make `overflowPopupBtns` a thin accessor**

In `drumview_context_menu.go`, replace `overflowPopupBtns` (876-904) with a rebuild method plus an accessor that updates rects each frame:

```go
// rebuildOverflowBtns rebuilds the persisted overflow row buttons for the
// current page. Called on open and on page switch (item set changes). Button
// objects persist between rebuilds so their press-animation state survives
// across frames; only their rects are refreshed per frame (see overflowPopupBtns).
func (dv *DrumView) rebuildOverflowBtns() {
	items := dv.overflowItems()
	itemStyle := ButtonVisual(DropdownStyle)
	btns := make([]*Button, 0, len(items)+1)
	for _, item := range items {
		if item.header {
			continue // headers are not buttons
		}
		btn := NewButton(item.label, itemStyle, item.onClick)
		btns = append(btns, btn)
	}
	// Close button at top-right (rect set in overflowPopupBtns each frame).
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeOverflowMenu() })
	closeB.Icon = "close"
	closeB.IconColor = closeIconColor()
	closeB.ConsumeOnPress = true
	btns = append(btns, closeB)
	dv.overflowBtns = btns
}

// overflowPopupBtns returns the persisted overflow buttons with their rects
// refreshed for the current scroll offset and popup geometry. It rebuilds the
// button set if the count is stale (e.g. first call, or page switch happened
// without an explicit rebuild). Safe to call every frame from draw and input.
func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	dv.configureOverflowScroll()
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	// Count non-header items; rebuild if the persisted set doesn't match
	// (covers first call and File↔Templates page switches).
	want := 0
	for _, item := range items {
		if !item.header {
			want++
		}
	}
	if len(dv.overflowBtns) != want+1 { // +1 close button
		dv.rebuildOverflowBtns()
	}
	// Refresh rects for the current scroll offset.
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	for _, item := range items {
		if item.header {
			curY += rowH
			continue
		}
		r := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		if btnIdx < len(dv.overflowBtns)-1 {
			dv.overflowBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
		}
		btnIdx++
		curY += rowH
	}
	// Close button rect (last element).
	if len(dv.overflowBtns) > 0 {
		dv.overflowBtns[len(dv.overflowBtns)-1].SetRect(closeButtonRect(popupRect, SpaceXS))
	}
	return dv.overflowBtns
}
```

Important: the `onClick` closures capture `item` per rebuild. Because rebuild happens on page switch, the Templates-page closures correctly capture each template's `tp`. Keep the existing `tp := tp` shadowing inside `overflowItems()` (it is already there at line 814) so each closure binds its own template.

- [ ] **Step 5: Route the draw rows through `drawMenuRow`**

In `drawOverflowMenu` (913-979), replace the per-row draw block (the `if item.iconID != "" { DrawIcon... }` + label section, lines ~948-971) so the action-row branch uses `drawMenuRow`. The header branch (`drawMenuTitle`) is unchanged. Replace the body of the `for _, item := range items` loop's non-header path with:

```go
		btn := btns[btnIdx]
		btnIdx++

		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		var iconTint color.Color = colMenuIcon
		if item.active {
			iconTint = colFollowActive
		}
		drawMenuRow(clip, btn, MenuRowSpec{
			Accent:   dv.contextMenuAccent(),
			State:    state,
			IconID:   item.iconID,
			IconTint: iconTint,
			Label:    btn.Text,
		})
```

Keep the surrounding loop scaffolding (header handling, `curY`, `btnIdx`, `nActionBtns` bounds check, the trailing close button `btns[len(btns)-1].Draw(dst)` and scrollbar draw) exactly as-is. Note `drawMenuRow` is called with `clip` (the SubImage) just like the old per-row draws.

- [ ] **Step 6: Rebuild buttons on page switch**

The Back/Load-Template `onClick`s mutate `dv.overflowPage` (lines 810, 819, 854). Because `overflowPopupBtns` rebuilds when the non-header count changes, a File↔Templates switch (different item counts) auto-rebuilds. To be safe and explicit, also clear the persisted set on open. In `openOverflowMenuPortal` (drumview_portal_open.go ~149) and/or `closeOverflowMenu`, set `dv.overflowBtns = nil` on close so a fresh open rebuilds:

Find `closeOverflowMenu` (drumview_context_menu.go ~1012) and add `dv.overflowBtns = nil` alongside the existing scroll-state reset. Verify the exact reset site by reading the function first.

- [ ] **Step 7: Run the persistence test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run TestOverflowButtonsPersist`
Expected: PASS.

- [ ] **Step 8: Run all template/overflow tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui -run 'Template|Overflow'`
Expected: PASS — `template_menu_test.go`, `template_menu_scroll_test.go`, `template_menu_width_test.go`, `template_import_test.go`. If `template_menu_width_test.go` asserts width via `menuRowLabelX`/`overflowPopupBtns`, confirm the accessor still returns correctly-sized buttons (it builds the same labels). Fix any width assertion only if the label set genuinely changed (it did not).

- [ ] **Step 9: Checkpoint — full UI package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`
Expected: PASS. (No commit.)

---

### Task 6: Full verification — suite, parity, design guards, screenshots

**Files:** none (verification only).

- [ ] **Step 1: Full fast UI + audio + timeline suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline`
Expected: PASS.

- [ ] **Step 2: Design-token guards (no new literals, drift, lint)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS. `drawMenuRow` introduced no inline color literals, so `TestTokenDiscipline` is unaffected. If it fails citing `menu_chrome.go`, replace any stray literal with the appropriate `Token*` accessor.

- [ ] **Step 3: Visual parity screenshots (manual review)**

Run: `cd src/go/.. && cd /home/ymolinar/Repos/beatmo && make screenshots-all SCENES=$(./tmp/beatmo_screenshot -list-scenes | grep -Ei 'inst|menu|template|overflow|subdiv|context' | paste -sd ,)` (adjust scene filter to whatever menu scenes exist; run `./tmp/beatmo_screenshot -list-scenes` first to see names).
Expected: PNGs in `screenshots/`. Manually confirm:
- Instrument menu rows look IDENTICAL to before (regression anchor).
- Template list, context menu, subdivision rows now show the same keycap/hover treatment as the instrument menu (the unification goal).

- [ ] **Step 4: Browser bridge smoke (template loading still works through the bridge)**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS (this change is Go-render-only; the bridge catalogue is untouched, but run it to confirm nothing regressed in the overflow/import path).

- [ ] **Step 5: Final summary**

Confirm: all four overlay menus route through `drawMenuRow`; only behavior differences (open/close/toggle) remain per-menu; node sidebar untouched; nothing committed (work left uncommitted for user review).

---

## Self-Review Notes

**Spec coverage:**
- Shared `drawMenuRow` + `MenuRowSpec` → Task 1. ✓
- Instrument/category migration (regression anchor) → Task 2. ✓
- Subdivision migration → Task 3. ✓
- Row context menu migration → Task 4. ✓
- Template/overflow migration + persistence refactor → Task 5. ✓
- Node sidebar out of scope → respected (not touched). ✓
- Behavior preserved per-menu → each task keeps `onClick` untouched. ✓
- Testing (drawMenuRow unit test, overflow animation persistence, existing suites, screenshots, design guards) → Tasks 1, 3, 5, 6. ✓

**Type consistency:** `MenuRowSpec` field names (`Accent`, `State`, `Swatch`, `IconID`, `IconTint`, `Highlights`, `Label`, `LabelColor`, `LabelRole`, `CenterLabel`) are used identically in Tasks 2-5. `drawMenuRow` / `rebuildOverflowBtns` / `overflowBtns` names are consistent across Task 5.

**Open verification points flagged inline (not placeholders — confirmations the implementer must do against live code):**
- Task 1 Step 1a: confirm `RoleBody` vs `TextRole` zero value (default is safe either way).
- Task 3 Step 1/2: match the existing subdiv test constructor; confirm `image/color` import.
- Task 4 Step 2: confirm whether `drawContextMenuIcon`/`contextMenuItemGeom` become unused (optional cleanup).
- Task 5 Step 1/6: match the existing template-menu test harness; confirm the `closeOverflowMenu` reset site.
