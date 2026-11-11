# Menu placement + minimalist unification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every row-opened menu pop at the top of the bottom (audio) panel, and unify all menus to a minimalist-flat look with per-instrument accent color.

**Architecture:** Placement is centralized in a new pure helper `AnchorPopupPanelTop` + a `DrumView.bottomPanelTopY()` accessor; the three row-opened menus adopt it. Style is centralized in `menu_chrome.go` (flatten the two shared helpers + add a separator helper); each menu's render path drops its keycap/group-box chrome. Accent color is already per-instrument across all menus and needs no change.

**Tech Stack:** Go 1.23, Ebiten, project `internal/ui` package. Bundled Go at `.tools/go/bin/go`.

## Global Constraints

- **Do NOT `git add` / commit / stage anything.** Leave all changes in the working tree; the user owns committing. The TDD "commit" step is replaced by a **checkpoint** (run the test, confirm green, move on).
- **Test command (fast path), explicit package + `-run` filter** (the branch has a phantom `./...` build — never use `./...`):
  ```
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '<RegExp>' ./internal/ui/
  ```
- **Build check:** `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` (gopls "undefined" flapping is noise; `go vet`/`go build` is truth).
- The `internal/ui` suite has known PRE-EXISTING failures unrelated to menus — verify only via the `-run` filters in this plan, never whole-suite pass/fail.
- Accent color is already per-instrument (`contextMenuAccent`, `(*InstrumentMenuComponent).menuAccent`, `longPressPopupAccent`); **do not** rewire accent. Destructive (Delete/Remove) stays red.
- Mobile (`Profile().UseBottomSheet`) keeps its bottom-sheet placement on every menu; placement changes are desktop-only branches. The flat restyle applies to both.

---

## File structure

- `overlay_anchor.go` — add `AnchorPopupPanelTop` (placement primitive).
- `drumview.go` — add `(*DrumView) bottomPanelTopY()` accessor.
- `drumview_context_menu.go` — context menu desktop placement + flatten rows.
- `drumview_overlay_inst_comp.go` + `drumview_instrument_helpers.go` — instrument picker desktop placement.
- `drumview_overlay_color_comp.go` + `drumview_context_menu.go` (`openColorPickerForRow`) — color picker desktop placement.
- `menu_chrome.go` — flatten header band + item background; add `drawMenuSeparator`.
- `drumview_context_menu.go` (`drawOverflowMenu`), `drumview_overlay_subdiv_comp.go` (`drawSubdivRow`) — drop keycap chrome.
- `DESIGN.md` + `design_tokens.gen.go` (regen) — narrative + token comments.

---

### Task 1: Placement primitive + bottom-panel accessor

**Files:**
- Modify: `src/go/internal/ui/overlay_anchor.go`
- Modify: `src/go/internal/ui/drumview.go`
- Test: `src/go/internal/ui/overlay_anchor_test.go` (create if absent; otherwise append)

**Interfaces:**
- Produces: `AnchorPopupPanelTop(bounds image.Rectangle, panelTopY, leftX, w, h int) image.Rectangle` — returns a rect whose top is pinned to `panelTopY`, left edge at `leftX`, size `(w,h)`, fully clamped inside `bounds` via `ClampRectInto`.
- Produces: `(dv *DrumView) bottomPanelTopY() int` — top Y of the audio/EQ panel; falls back to `dv.Bounds.Max.Y` when the panel is absent/empty.

- [ ] **Step 1: Write the failing test**

Append to `overlay_anchor_test.go`:

```go
func TestAnchorPopupPanelTop(t *testing.T) {
	bounds := image.Rect(0, 0, 800, 600)

	// Pins top to panelTopY, left to leftX, keeps size.
	got := AnchorPopupPanelTop(bounds, 400, 30, 160, 120)
	want := image.Rect(30, 400, 190, 520)
	if got != want {
		t.Fatalf("AnchorPopupPanelTop = %v, want %v", got, want)
	}

	// Clamps when it would overflow the right/bottom edges.
	got = AnchorPopupPanelTop(bounds, 550, 700, 160, 120)
	if !got.In(bounds) {
		t.Fatalf("rect %v escaped bounds %v", got, bounds)
	}
	if got.Dx() != 160 || got.Dy() != 120 {
		t.Fatalf("clamp must translate, not shrink: got size %dx%d", got.Dx(), got.Dy())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestAnchorPopupPanelTop ./internal/ui/`
Expected: FAIL — `undefined: AnchorPopupPanelTop`.

- [ ] **Step 3: Implement the helper**

Append to `overlay_anchor.go`:

```go
// AnchorPopupPanelTop positions a popup of size (w, h) with its top edge pinned
// to panelTopY and its left edge at leftX, growing downward, clamped fully
// inside bounds. Used by row-opened menus so they always land at the top of the
// bottom (audio) panel near the row controls instead of dropping off-screen.
func AnchorPopupPanelTop(bounds image.Rectangle, panelTopY, leftX, w, h int) image.Rectangle {
	if w > bounds.Dx() {
		w = bounds.Dx()
	}
	if h > bounds.Dy() {
		h = bounds.Dy()
	}
	return ClampRectInto(image.Rect(leftX, panelTopY, leftX+w, panelTopY+h), bounds)
}
```

- [ ] **Step 4: Add the accessor**

In `drumview.go`, add (near other `(*DrumView)` accessors):

```go
// bottomPanelTopY returns the top Y of the audio/EQ panel — the anchor line for
// row-opened menus (they pin here and grow down). Falls back to the DrumView's
// bottom edge when the panel is absent (tests / pre-layout).
func (dv *DrumView) bottomPanelTopY() int {
	if dv.eqPanelZone != nil {
		if r := dv.eqPanelZone.PanelRect(); !r.Empty() {
			return r.Min.Y
		}
	}
	return dv.Bounds.Max.Y
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestAnchorPopupPanelTop ./internal/ui/`
Expected: PASS. Then `go vet -tags test ./internal/ui/` clean.

- [ ] **Step 6: Checkpoint** (no commit — leave in working tree).

---

### Task 2: Context menu (desktop) → top of bottom panel

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go:106-142` (desktop branch of `openContextMenu`)
- Test: `src/go/internal/ui/drumview_context_menu_test.go`

**Interfaces:**
- Consumes: `AnchorPopupPanelTop`, `dv.bottomPanelTopY()` (Task 1).

- [ ] **Step 1: Write the failing test**

Append to `drumview_context_menu_test.go` (mirrors `TestContextMenuDesktopStaysOnScreen`):

```go
func TestContextMenuDesktopAnchorsAtPanelTop(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 12; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Row", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	dv.openContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}
	if got, want := dv.contextMenuRect.Min.Y, dv.bottomPanelTopY(); got != want {
		t.Fatalf("context menu top = %d, want panel top %d", got, want)
	}
	if !dv.contextMenuRect.In(dv.Bounds) {
		t.Fatalf("context menu rect %v escapes bounds %v", dv.contextMenuRect, dv.Bounds)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestContextMenuDesktopAnchorsAtPanelTop ./internal/ui/`
Expected: FAIL — top is the label-right anchor, not the panel top.

- [ ] **Step 3: Implement — swap the anchor call**

In `openContextMenu`, desktop branch, replace lines 112-120:

```go
		var anchor image.Rectangle
		if rowIdx < len(dv.rowLabels()) {
			anchor = dv.rowLabels()[rowIdx].Rect()
		} else {
			anchor = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+100, dv.Bounds.Min.Y+dv.headerH+dv.rowHeight())
		}
		menuW := 160
		menuH := nItems * rowH
		dv.contextMenuRect = AnchorPopupRect(dv.Bounds, anchor, menuW, menuH, PopupRight)
```

with:

```go
		// Pin to the top of the bottom (audio) panel, left-aligned under the
		// row control column, so the menu always lands fully-visible near the
		// controls instead of dropping off the bottom edge (see plan).
		leftX := dv.Bounds.Min.X
		if rowIdx < len(dv.rowLabels()) {
			leftX = dv.rowLabels()[rowIdx].Rect().Min.X
		}
		menuW := 160
		menuH := nItems * rowH
		dv.contextMenuRect = AnchorPopupPanelTop(dv.Bounds, dv.bottomPanelTopY(), leftX, menuW, menuH)
```

- [ ] **Step 4: Run to verify it passes + no regressions**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestContextMenuDesktopAnchorsAtPanelTop|TestContextMenuDesktopStaysOnScreen|TestContextMenuDesktopButtonIteration|TestContextMenuClickOutsideCloses' ./internal/ui/`
Expected: PASS (all four).

- [ ] **Step 5: Checkpoint.**

---

### Task 3: Instrument picker (desktop) → top of bottom panel

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go` (add `PanelTopY` prop; use it in `rebuildMenu`)
- Modify: `src/go/internal/ui/drumview_instrument_helpers.go:200-230` (`openInstMenuForRow` — set `PanelTopY` + extend `VertBounds`)
- Test: `src/go/internal/ui/inst_menu_open_close_test.go`

**Interfaces:**
- Consumes: `dv.bottomPanelTopY()` (Task 1).
- Produces: new field `InstrumentMenuProps.PanelTopY int` (desktop: bottom-panel top; 0 = legacy up/down behavior).

- [ ] **Step 1: Add the prop**

In `drumview_overlay_inst_comp.go`, add to the `InstrumentMenuProps` struct (next to `VertBounds`):

```go
	// PanelTopY, when > 0 and not a bottom sheet, pins the menu's top edge to
	// this Y (the bottom-panel top) and forces it to grow downward. 0 keeps the
	// legacy "open toward whichever side has more room" behavior.
	PanelTopY int
```

- [ ] **Step 2: Write the failing test**

Append to `inst_menu_open_close_test.go`:

```go
func TestInstMenuDesktopAnchorsAtPanelTop(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 12; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Row", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	dv.openInstMenuForRow(0)
	r := dv.instMenuComp.fullRect
	if r.Empty() {
		t.Fatal("instrument menu should have a non-empty rect")
	}
	if got, want := r.Min.Y, dv.bottomPanelTopY(); got != want {
		t.Fatalf("instrument menu top = %d, want panel top %d", got, want)
	}
	if !r.In(dv.Bounds) {
		t.Fatalf("instrument menu rect %v escapes bounds %v", r, dv.Bounds)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestInstMenuDesktopAnchorsAtPanelTop ./internal/ui/`
Expected: FAIL — top is the space-based above/below position, not panel top.

- [ ] **Step 4: Honor `PanelTopY` in `rebuildMenu`**

In `drumview_overlay_inst_comp.go`, in `rebuildMenu`, after the existing direction block:

```go
	// Decide direction based on available space
	spaceDown := vertBounds.Max.Y - base.Max.Y
	spaceUp := base.Min.Y - vertBounds.Min.Y
	openUp := spaceDown < spaceUp
	if Profile().UseBottomSheet {
		openUp = true // always bottom sheet on mobile
	}
```

insert:

```go
	// Panel-top pin (desktop): collapse the anchor to the panel-top line and
	// force downward growth; widen vertBounds downward so the list builds into
	// the bottom panel. buildInstrumentsMode then computes startY = base.Max.Y.
	if m.props.PanelTopY > 0 && !Profile().UseBottomSheet {
		base = image.Rect(base.Min.X, m.props.PanelTopY, base.Max.X, m.props.PanelTopY)
		if vertBounds.Max.Y < m.props.PanelTopY {
			vertBounds.Max.Y = m.props.PanelTopY
		}
		if vertBounds.Min.Y > m.props.PanelTopY {
			vertBounds.Min.Y = m.props.PanelTopY
		}
		openUp = false
	}
```

> Note: `buildInstrumentsMode` already clamps `startY+totalH` into `vertBounds` and translates the whole picker inside `vertBounds`, so extending `vertBounds` down to the screen bottom (Step 5) keeps the menu on-screen.

- [ ] **Step 5: Set the prop + extend VertBounds in `openInstMenuForRow`**

In `drumview_instrument_helpers.go`, where `vertBounds` is computed (currently `dv.widgetRects[WidgetRack]` / `dv.Bounds` on mobile), and in the `SetProps` call, change to:

```go
	vertBounds := dv.widgetRects[WidgetRack]
	panelTopY := 0
	if Profile().IsMobile() {
		vertBounds = dv.Bounds
	} else {
		// Desktop: pin to bottom-panel top and let the list grow into the panel.
		panelTopY = dv.bottomPanelTopY()
		vertBounds = image.Rect(vertBounds.Min.X, vertBounds.Min.Y, vertBounds.Max.X, dv.Bounds.Max.Y)
	}
```

and add `PanelTopY: panelTopY,` to the `InstrumentMenuProps{...}` literal (next to `VertBounds: vertBounds,`).

- [ ] **Step 6: Run to verify it passes + no regressions**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestInstMenu|TestInstrumentMenu' ./internal/ui/`
Expected: PASS (new test + existing instrument-menu tests).

- [ ] **Step 7: Checkpoint.**

---

### Task 4: Color picker (desktop) → top of bottom panel

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_color_comp.go` (add `PanelTopY` prop; honor it in `rebuildWheel`)
- Modify: `src/go/internal/ui/drumview_context_menu.go:726-770` (`openColorPickerForRow` — set `PanelTopY`, extend `Bounds`)
- Test: `src/go/internal/ui/color_picker_menu_test.go`

**Interfaces:**
- Consumes: `dv.bottomPanelTopY()` (Task 1).
- Produces: new field `ColorWheelProps.PanelTopY int`.

- [ ] **Step 1: Add the prop**

In `drumview_overlay_color_comp.go`, add to `ColorWheelProps` (next to `AnchorRect`):

```go
	// PanelTopY, when > 0 and not a bottom sheet, pins the panel's top edge to
	// this Y (the bottom-panel top). 0 keeps the legacy anchor-relative placement.
	PanelTopY int
```

- [ ] **Step 2: Write the failing test**

Append to `color_picker_menu_test.go`:

```go
func TestColorPickerDesktopAnchorsAtPanelTop(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	for i := 0; i < 12; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{Name: "Row", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0})
	}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	dv.openColorPickerForRow(0)
	r := dv.colorWheelComp.WheelRect()
	if r.Empty() {
		t.Fatal("color wheel should have a non-empty rect")
	}
	if got, want := r.Min.Y, dv.bottomPanelTopY(); got != want {
		t.Fatalf("color wheel top = %d, want panel top %d", got, want)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestColorPickerDesktopAnchorsAtPanelTop ./internal/ui/`
Expected: FAIL — top is anchor-relative.

- [ ] **Step 4: Honor `PanelTopY` in `rebuildWheel`**

In `drumview_overlay_color_comp.go`, replace the desktop placement block (lines 189-197):

```go
	} else {
		spaceDown := bounds.Max.Y - c.props.AnchorRect.Max.Y
		spaceUp := c.props.AnchorRect.Min.Y - bounds.Min.Y
		originY := c.props.AnchorRect.Max.Y
		if spaceDown < panelH && spaceUp > spaceDown {
			originY = c.props.AnchorRect.Min.Y - panelH // open upward
		}
		rect = rect.Add(image.Pt(c.props.AnchorRect.Min.X, originY))
	}
```

with:

```go
	} else {
		originY := c.props.AnchorRect.Max.Y
		if c.props.PanelTopY > 0 {
			originY = c.props.PanelTopY // pin to bottom-panel top
		} else {
			spaceDown := bounds.Max.Y - c.props.AnchorRect.Max.Y
			spaceUp := c.props.AnchorRect.Min.Y - bounds.Min.Y
			if spaceDown < panelH && spaceUp > spaceDown {
				originY = c.props.AnchorRect.Min.Y - panelH // open upward
			}
		}
		rect = rect.Add(image.Pt(c.props.AnchorRect.Min.X, originY))
	}
```

> The existing clamp block (lines 199-211) already keeps `rect` inside `bounds`; Step 5 extends `bounds` downward so the clamp doesn't pull the top back up.

- [ ] **Step 5: Set the prop + extend Bounds in `openColorPickerForRow`**

In `drumview_context_menu.go`, in `openColorPickerForRow`, replace the `rackBounds` setup + `SetProps` head:

```go
		rackBounds := dv.widgetRects[WidgetRack]
		if rackBounds.Empty() {
			rackBounds = dv.Bounds
		}
		dv.colorWheelComp.SetProps(ColorWheelProps{
			AnchorRect: anchor,
			Bounds:     rackBounds,
```

with:

```go
		rackBounds := dv.widgetRects[WidgetRack]
		if rackBounds.Empty() {
			rackBounds = dv.Bounds
		}
		panelTopY := 0
		if !Profile().UseBottomSheet {
			// Extend the clamp bounds down into the audio panel so the wheel can
			// pin at the panel top and grow into it.
			panelTopY = dv.bottomPanelTopY()
			rackBounds = image.Rect(rackBounds.Min.X, rackBounds.Min.Y, rackBounds.Max.X, dv.Bounds.Max.Y)
		}
		dv.colorWheelComp.SetProps(ColorWheelProps{
			AnchorRect: anchor,
			Bounds:     rackBounds,
			PanelTopY:  panelTopY,
```

- [ ] **Step 6: Run to verify it passes + no regressions**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestColorPicker|TestColorWheel' ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Checkpoint.**

---

### Task 5: Flatten the shared menu chrome

**Files:**
- Modify: `src/go/internal/ui/menu_chrome.go`
- Test: `src/go/internal/ui/menu_chrome_test.go` (create if absent)

**Interfaces:**
- Produces: `drawMenuSeparator(dst *ebiten.Image, r image.Rectangle)` — 1px hairline at the vertical center of `r`.
- Behavior change: `drawMenuItemBackgroundAccent` rest = no-op (flat); hover = flat accent tint; active = accent tint + stripe. `drawMenuHeaderBandAccent` = no gradient, hairline underline only.

- [ ] **Step 1: Write the failing test**

Create `menu_chrome_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Rest state must be flat: drawing it leaves the destination untouched.
func TestMenuItemRestIsFlat(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	before := dst.At(20, 10)
	drawMenuItemBackgroundAccent(dst, image.Rect(0, 0, 40, 20), menuItemRest, colAccent)
	if after := dst.At(20, 10); after != before {
		t.Fatalf("rest menu item must be flat (no fill); pixel changed %v -> %v", before, after)
	}
}

// The separator helper draws a visible hairline.
func TestMenuSeparatorDraws(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	drawMenuSeparator(dst, image.Rect(0, 0, 40, 20))
	_, _, _, a := dst.At(20, 10).RGBA()
	if a == 0 {
		t.Fatal("separator must draw a visible line at the row's vertical center")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMenuItemRestIsFlat|TestMenuSeparatorDraws' ./internal/ui/`
Expected: FAIL — rest currently fills `colSurface2`; `drawMenuSeparator` undefined.

- [ ] **Step 3: Flatten the helpers + add separator**

In `menu_chrome.go`, replace `drawMenuHeaderBandAccent` body:

```go
func drawMenuHeaderBandAccent(dst *ebiten.Image, r image.Rectangle, accent color.Color) {
	if r.Empty() {
		return
	}
	// Flat: no gradient band. A single hairline underline in the accent color
	// is the only chrome; the title text is drawn by the caller over the panel.
	underline := image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y)
	drawRect(dst, underline, WithAlphaFromColor(accent, genAlphaMenuHeaderAccent), true)
}
```

Replace the `menuItemRest` / `menuItemHover` cases in `drawMenuItemBackgroundAccent`:

```go
	switch state {
	case menuItemRest:
		// Flat: rows sit directly on the menu's single surface, no per-row pad.
	case menuItemHover:
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaSubtle), true)
	case menuItemActive:
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaMenuActiveTint), true)
		stripe := image.Rect(r.Min.X, r.Min.Y, r.Min.X+accentStripeW(), r.Max.Y)
		drawRect(dst, stripe, accent, true)
	}
```

Append the separator helper:

```go
// drawMenuSeparator draws a 1px hairline across the vertical center of r — the
// minimalist group divider that replaces the old group-container boxes.
func drawMenuSeparator(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	y := r.Min.Y + r.Dy()/2
	line := image.Rect(r.Min.X+SpaceSM, y, r.Max.X-SpaceSM, y+1)
	drawRect(dst, line, WithAlpha(genColorBorder, AlphaSubtle), true)
}
```

> `drawKeycapBevel` is no longer called from `menu_chrome.go`; leave the function (used elsewhere). If `go vet` reports `fillVerticalGradient`/`drawKeycapBevel` imports unused *here*, none are imports — they are package funcs — so no import edits are needed.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMenuItemRestIsFlat|TestMenuSeparatorDraws' ./internal/ui/`
Expected: PASS. Then `go vet -tags test ./internal/ui/` clean.

- [ ] **Step 5: Checkpoint.**

---

### Task 6: Flatten context-menu rows (drop group boxes + keycap chrome)

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` — `drawContextMenu`: replace the "group container backgrounds" block with separators; drop the per-item `btn.Draw` keycap chrome.
- Test: `src/go/internal/ui/drumview_context_menu_test.go`

**Interfaces:**
- Consumes: `drawMenuSeparator` (Task 5).

- [ ] **Step 1: Replace the group-container block with separators**

In `drawContextMenu`, replace the entire block headed `// Draw group container backgrounds behind items.` (the `if dv.contextMenuRow >= 0 ... { ... }` that builds `drawGroupBG` and walks items emitting group BGs) with:

```go
	// Hairline separators between groups (replaces the old group-container
	// boxes for the flat look). Walk items mirroring the layout pitch.
	if dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		items := dv.contextMenuItems(dv.contextMenuRow)
		isMobile := Profile().IsMobile()
		scrollPx := 0
		if dv.contextMenuScroll != nil {
			scrollPx = dv.contextMenuScroll.VS.First * touchMinTargetPx
		}
		curY := dv.contextMenuRect.Min.Y
		if isMobile {
			curY = dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx
		}
		dividerGap := SpaceSM
		if !isMobile {
			dividerGap = 0
		}
		for _, item := range items {
			if item.divider {
				sep := image.Rect(dv.contextMenuRect.Min.X, curY, dv.contextMenuRect.Max.X, curY+1)
				if sep.Min.Y >= dv.contextMenuRect.Min.Y && sep.Max.Y <= dv.contextMenuRect.Max.Y {
					drawMenuSeparator(dst, image.Rect(sep.Min.X, curY-1, sep.Max.X, curY+1))
				}
				curY += dividerGap
				continue
			}
			curY += touchMinTargetPx
		}
	}
```

- [ ] **Step 2: Drop the per-item keycap chrome**

In `drawContextMenuItem` (inside `drawContextMenu`), remove these three lines:

```go
		// Draw button chrome (shadow/glow) without text — null Text temporarily.
		saved := btn.Text
		btn.Text = ""
		btn.Draw(dst)
		btn.Text = saved
```

and change the label draw that referenced `saved` to use `btn.Text` directly:

```go
		// Label via RoleBody; keep the existing per-item text color (e.g. red for Delete).
		var labelCol color.Color = colTextPrimary
		if btn.TextColor != nil {
			labelCol = btn.TextColor
		}
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(dst, btn.Text, r.Min.X, ty, RoleBody, labelCol)
```

- [ ] **Step 3: Build + run the context-menu suite**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` (must compile — `colMenuGroupBG`/`colMenuGroupDeleteBG` are now unused locals only if they were locals; they are package constants, so no edit needed).
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestContextMenu' ./internal/ui/`
Expected: PASS. If a test asserted on the old group-box fill, update it to assert the separator/flat behavior (cite this task).

- [ ] **Step 4: Checkpoint.**

---

### Task 7: Flatten overflow + subdiv rows (drop keycap chrome)

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` — `drawOverflowMenu` (drop the `btn.Text=""; btn.Draw(clip); restore` block; draw label from `btn.Text`).
- Modify: `src/go/internal/ui/drumview_overlay_subdiv_comp.go` — `drawSubdivRow` (same drop; draw label from `btn.Text`).
- Test: `src/go/internal/ui/subdiv_menu_scroll_test.go` (reuse existing suite for regression)

- [ ] **Step 1: Overflow — drop keycap chrome**

In `drawOverflowMenu`, remove:

```go
		// Draw button chrome (shadow/glow) without text — null Text temporarily.
		saved := btn.Text
		btn.Text = ""
		btn.Draw(clip)
		btn.Text = saved
```

and change the label line to:

```go
		// Label via RoleBody.
		r := btn.Rect()
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(clip, btn.Text, r.Min.X, ty, RoleBody, colTextPrimary)
```

- [ ] **Step 2: Subdiv — drop keycap chrome**

In `drawSubdivRow`, remove:

```go
	// Draw button chrome (glow/shadow/press) without text — blank Text temporarily.
	saved := btn.Text
	btn.Text = ""
	btn.Draw(dst)
	btn.Text = saved
```

and change the centered-label block to use `btn.Text`:

```go
	th := StyledTextHeight(RoleBody)
	tw := StyledTextWidth(btn.Text, RoleBody)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	col := colTextPrimary
	if v == s.props.Current {
		col = colTextAccent
	}
	DrawTextStyled(dst, btn.Text, tx, ty, RoleBody, col)
```

- [ ] **Step 3: Build + run**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestOverflow|TestSubdiv|TestTemplateMenuScroll' ./internal/ui/`
Expected: PASS.

- [ ] **Step 4: Checkpoint.**

---

### Task 8: DESIGN.md narrative + token comments

**Files:**
- Modify: `DESIGN.md`
- Regen: `src/go/internal/ui/design_tokens.gen.go` (via `make gen-design-tokens`)
- Test: `design_md_drift_test.go`, `token_discipline_test.go`, `design_md_lint_test.go` (guards, not edited)

> Token *values* don't change (`menu-active-tint` = 60, `menu-header-accent` = 50 retained), so the drift want-map and gen constants stay numerically identical — only YAML comments + narrative change. Regen is a no-op safety check.

- [ ] **Step 1: Update the alpha-token comment**

In `DESIGN.md` line ~947, change:

```
  menu-header-accent:      50  # 1px gold underline on menu header bands
```

to:

```
  menu-header-accent:      50  # 1px accent hairline under a menu's slim title row
```

- [ ] **Step 2: Update the menu-style narrative**

In `DESIGN.md`, in the menu/components section, replace any "Neon Horizon" gradient-header / keycap-bevel menu description with the flat spec. Add, near the `context-menu-item` component note (~line 1217):

```
  # Menus are minimalist-flat: rows sit directly on the single panel surface
  # (no per-row keycap pad/bevel, no group-container boxes). Group breaks use a
  # 1px hairline separator. The owning instrument's color tints the slim title
  # hairline, the active-row fill+stripe, hover fill, and the title color dot;
  # at-rest rows are neutral. Destructive items keep `error` red text.
```

- [ ] **Step 3: Regenerate tokens + run guards**

Run: `make gen-design-tokens`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS. If `TestDesignMDLintSnapshot` reports a changed warning set from the comment edits, update `testdata/design_md_lint.expected.json` accordingly and cite this task.

- [ ] **Step 4: Checkpoint.**

---

### Task 9: Visual verification (screenshots) + full fast-path

**Files:** none (verification only)

- [ ] **Step 1: Re-capture the menu scenes (desktop + mobile)**

Run:
```
make screenshots-all SCENES=instrument_menu_open,context_menu_open,subdiv_menu_open,color_wheel_open,node_sidebar_open
make screenshots-all MOBILE=1 SCENES=instrument_menu_open,context_menu_open,subdiv_menu_open,color_wheel_open
```
Inspect `screenshots/all/{desktop,mobile}/*.png`: every menu flat (no gradient header / keycap rows / group boxes), per-instrument accent present, row menus pinned to the bottom-panel top and fully on-screen.

- [ ] **Step 2: Run scene/crop guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestScene|Crop' ./internal/ui/`
Expected: PASS.

- [ ] **Step 3: Full menu/placement fast-path sweep**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Menu|ContextMenu|InstMenu|ColorPicker|ColorWheel|Subdiv|Overflow|AnchorPopup|LongPress' ./internal/ui/`
Expected: PASS. (Any failure here is in-scope; whole-suite pre-existing failures are out of scope per Global Constraints.)

- [ ] **Step 4: Final checkpoint — report results to the user (no commit).**

---

## Self-review

- **Spec coverage:** Placement → Tasks 1-4 (context, instrument, color) + node pop-up stays put (no task = unchanged, per spec) + subdiv/overflow stay anchored (only restyled, Task 7). Style → Tasks 5-7 (menu_chrome flat, context rows, overflow/subdiv rows). Per-instrument accent → already implemented (spec §C confirmed; no task needed). DESIGN.md → Task 8. Testing → each task + Task 9. ✓
- **Node pop-up:** spec says "restyle to match" but it uses keycap buttons + hover-accent text, not menu_chrome — it is already flat (no gradient/box) and already per-instrument-tinted (`longPressPopupAccent`). No change required; called out here so a reviewer doesn't flag a gap.
- **Placeholder scan:** none — every code step shows real before/after.
- **Type consistency:** `AnchorPopupPanelTop`, `bottomPanelTopY`, `drawMenuSeparator`, `PanelTopY` (on both `InstrumentMenuProps` and `ColorWheelProps`) used consistently across tasks.
