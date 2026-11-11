# Grid-menu drag handle (grip) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user drag the node pop-up and the node-groups menu anywhere inside the grid pane via a grip glyph in each card's top-left corner, without changing where either card appears by default.

**Architecture:** One new value type, `PopupDragHandle`, owns all drag state (pinned / origin / dragging / grab offset) and all clamping. Both menus embed it, call `Place()` at the end of their existing per-frame layout to divert the auto-anchored origin, and gain one input branch above their existing hit-test ladder that returns `InputCaptured`. No dispatcher, tree, or portal changes.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten 2.9, `internal/ui` package. Tests run under the stubbed-Ebiten fast path (`-tags test -modfile=go.test.mod`). Screenshots via the desktop binary under `xvfb-run`.

**Spec:** `docs/superpowers/specs/2026-07-27-grid-menu-drag-handle-design.md`

## Global Constraints

- **Never stage or commit.** The user reviews and commits their own work. Every task ends with a verification step, not a `git commit`. Do not run `git add`, `git commit`, `git stash`, or `git checkout` on tracked files.
- **Go binary:** always `.tools/go/bin/go` (repo root relative) — never system `go`. Same for `.tools/go/bin/gofmt`.
- **Fast test command** (run from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run '<Regex>'`. The 1800s timeout is mandatory — the full `internal/ui` suite takes ~590s and the 600s default produces timeout-not-failure noise.
- **Two pre-existing red tests** in `internal/ui` are unrelated to this work and must not be "fixed" here: `TestSamplerMobileOverflowMenuHasSaveAsReset` and `TestFocusGraph_RespondsToEveryKnob`. Any *other* red test is caused by this work — prove causation, never dismiss a failure as pre-existing.
- **No new color literals.** `internal/ui` has a per-file `color.RGBA{}` / `color.NRGBA{}` budget enforced by `TestTokenDiscipline` (ratchet: too few also fails). Use existing token accessors only: `WithAlpha(genColorBorder, genAlphaScrollbarThumb)`, `colTextSecondary`, `colAccent`.
- **No forbidden glyphs** (`▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←`) in `internal/ui` source. Chrome art goes through `IconID` + `DrawIcon`.
- **Corner radius by role.** Not touched by this plan; do not introduce raw `RadiusXX` values.
- **UI work is not done without a screenshot you have opened and looked at.** Task 6 is mandatory and gating — green Go tests do not prove the grip renders.
- **Grid coordinates shown to users are beats+subdivisions**, never raw units. Not touched by this plan.

## File Structure

| File | Responsibility |
|---|---|
| `src/go/internal/ui/popup_drag_handle.go` | **new.** The `PopupDragHandle` type: state, geometry, clamping. No knowledge of either menu. |
| `src/go/internal/ui/popup_drag_handle_test.go` | **new.** Pure-geometry unit tests for the type. |
| `src/go/internal/ui/drawing_icons.go` | `drawGripIcon` — the 2×3 dot matrix, alongside the other icon vars. |
| `src/go/internal/ui/icons.go` | `IconGrip` const + `drawIconByID` switch entry. |
| `src/go/internal/ui/game_node_sidebar.go` | Node pop-up: `drag` field, `Place` call, grip rect, title shift, input branch, `Capturing`, `Reset` in Open/Close. |
| `src/go/internal/ui/game_group_menu.go` | Group menu: same, plus skipping auto-placement while pinned. |
| `src/go/internal/ui/game_input_drag.go` | `gridMenuDragging()` helper. |
| `src/go/internal/ui/game_update.go` | `panOK` gains the drag gate. |
| `src/go/internal/ui/popup_drag_menus_test.go` | **new.** Functional tests driving real `g.Update()` frames for both menus + the camera gate. |
| `src/go/internal/ui/scene_catalog.go` | Two screenshot scenes. |

---

### Task 1: `IconGrip`

**Files:**
- Modify: `src/go/internal/ui/drawing_icons.go` (add after `drawOverflowIcon`, ~line 121)
- Modify: `src/go/internal/ui/icons.go` (const block ~line 56, switch ~line 89)
- Test: `src/go/internal/ui/popup_drag_handle_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `IconGrip IconID = "grip"`, drawable via `DrawIcon(dst, IconGrip, r, col) bool`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/popup_drag_handle_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestGripIconRegistered pins IconGrip into the icon dispatch switch. An icon
// const with no switch entry silently draws nothing (DrawIcon returns false).
func TestGripIconRegistered(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	if !DrawIcon(dst, IconGrip, r, colTextPrimary) {
		t.Fatalf("DrawIcon returned false for %q (not registered in drawIconByID switch)", IconGrip)
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

From `src/go`:

```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestGripIconRegistered
```

Expected: build failure — `undefined: IconGrip`.

- [ ] **Step 3: Add the icon const**

In `src/go/internal/ui/icons.go`, add to the const block (right after `IconCalendar`):

```go
	IconGrip     IconID = "grip"
```

- [ ] **Step 4: Add the draw function**

In `src/go/internal/ui/drawing_icons.go`, immediately after `drawOverflowIcon`:

```go
// drawGripIcon paints the drag-handle affordance: a 2×3 dot matrix, the
// conventional "grab me" texture. Mirrors drawOverflowIcon's dot geometry
// (r=1.5 on the shared 24×24 icon canvas) so the two read as one family.
var drawGripIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	for _, x := range []float64{9, 15} {
		for _, y := range []float64{6, 12, 18} {
			iconFillCircle(dst, c, x, y, 1.5, col)
		}
	}
}
```

- [ ] **Step 5: Register it in the dispatch switch**

In `src/go/internal/ui/icons.go`'s `drawIconByID`, next to the `IconOverflow` case:

```go
	case IconGrip:
		drawGripIcon(dst, r, col)
```

- [ ] **Step 6: Run the test and confirm it passes**

```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestGripIconRegistered
```

Expected: PASS.

- [ ] **Step 7: Confirm no token-budget regression**

```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift'
```

Expected: PASS. (The new function takes `col` from its caller and declares no literal — if this fails, you added a color literal; remove it.)

---

### Task 2: The `PopupDragHandle` component

**Files:**
- Create: `src/go/internal/ui/popup_drag_handle.go`
- Test: `src/go/internal/ui/popup_drag_handle_test.go` (extend from Task 1)

**Interfaces:**
- Consumes: `ExpandHitArea(size int) int` (`touch_sizes.go:83`), `SpaceXS`/`SpaceSM` spacing tokens, `sidebarPad` (`game_node_sidebar.go:23`, value 10).
- Produces — the exact API Tasks 3, 4 and 5 call:
  - `type PopupDragHandle struct{ … }` (zero value = auto mode, usable immediately)
  - `func (h *PopupDragHandle) GripRect(panel image.Rectangle) image.Rectangle`
  - `func (h *PopupDragHandle) HitRect(panel image.Rectangle) image.Rectangle`
  - `func (h *PopupDragHandle) Place(auto, pane, reserved image.Rectangle) image.Rectangle`
  - `func (h *PopupDragHandle) Begin(pt image.Point, panel image.Rectangle)`
  - `func (h *PopupDragHandle) MoveTo(pt image.Point, pane, reserved image.Rectangle, size image.Point)`
  - `func (h *PopupDragHandle) End()`
  - `func (h *PopupDragHandle) Dragging() bool`
  - `func (h *PopupDragHandle) Pinned() bool`
  - `func (h *PopupDragHandle) Reset()`
  - `const popupGripSize = 16`

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/popup_drag_handle_test.go`:

```go
// pane/reserved fixtures shared by the geometry tests: a 800×400 grid pane
// with a 120×40 button cluster welded to its top-right corner.
var (
	testDragPane     = image.Rect(0, 0, 800, 400)
	testDragReserved = image.Rect(680, 0, 800, 40)
)

// TestDragHandleAutoModeByDefault: an untouched handle is transparent — Place
// returns the caller's auto-anchored rect verbatim.
func TestDragHandleAutoModeByDefault(t *testing.T) {
	var h PopupDragHandle
	auto := image.Rect(100, 50, 360, 250)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("auto mode should pass the rect through: want %v got %v", auto, got)
	}
	if h.Pinned() {
		t.Fatal("zero-value handle must not be pinned")
	}
}

// TestDragHandleMovesByCursorDelta: the card follows the cursor 1:1 with no
// jump at press time — the grab offset keeps the grip under the finger.
func TestDragHandleMovesByCursorDelta(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 50, 360, 250)
	size := image.Pt(panel.Dx(), panel.Dy())

	press := image.Pt(112, 62) // inside the grip, 12px in from the card corner
	h.Begin(press, panel)
	h.MoveTo(press.Add(image.Pt(60, 30)), testDragPane, testDragReserved, size)
	h.End()

	got := h.Place(panel, testDragPane, testDragReserved)
	want := panel.Add(image.Pt(60, 30))
	if got != want {
		t.Fatalf("pinned rect should track the cursor delta: want %v got %v", want, got)
	}
	if h.Dragging() {
		t.Fatal("End() must clear the dragging flag")
	}
}

// TestDragHandleTapDoesNotPin: a press+release inside the deadzone is a tap,
// not a drag — the card stays in auto mode and keeps following its node.
func TestDragHandleTapDoesNotPin(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 50, 360, 250)
	size := image.Pt(panel.Dx(), panel.Dy())

	press := image.Pt(112, 62)
	h.Begin(press, panel)
	h.MoveTo(press.Add(image.Pt(1, 1)), testDragPane, testDragReserved, size)
	h.End()

	if h.Pinned() {
		t.Fatal("a 1px wobble is a tap, not a drag — must not pin")
	}
	auto := image.Rect(500, 200, 760, 400)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("after a tap the handle must still be transparent: want %v got %v", auto, got)
	}
}

// TestDragHandleClampsInsidePane: dragged hard past every edge, the card stays
// fully inside the pane (the node card's GridTree layer is clipped to it).
func TestDragHandleClampsInsidePane(t *testing.T) {
	size := image.Pt(260, 200)
	cases := []struct{ name string; to image.Point }{
		{"far left/up", image.Pt(-5000, -5000)},
		{"far right/down", image.Pt(5000, 5000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h PopupDragHandle
			panel := image.Rect(100, 150, 100+size.X, 150+size.Y)
			h.Begin(image.Pt(112, 162), panel)
			h.MoveTo(tc.to, testDragPane, testDragReserved, size)
			h.End()

			got := h.Place(panel, testDragPane, testDragReserved)
			if !got.In(testDragPane) {
				t.Fatalf("card %v escaped the pane %v", got, testDragPane)
			}
			if got.Dx() != size.X || got.Dy() != size.Y {
				t.Fatalf("clamping must not resize the card: want %v got %dx%d", size, got.Dx(), got.Dy())
			}
		})
	}
}

// TestDragHandleGripEscapesReservedZone: the top-right button cluster sits at a
// higher z and swallows presses, so a card parked under it must still expose
// its grip — otherwise the user can never drag it back out.
func TestDragHandleGripEscapesReservedZone(t *testing.T) {
	var h PopupDragHandle
	size := image.Pt(260, 200)
	panel := image.Rect(100, 150, 100+size.X, 150+size.Y)

	h.Begin(image.Pt(112, 162), panel)
	// Aim the grip straight at the middle of the cluster.
	h.MoveTo(image.Pt(740, 20), testDragPane, testDragReserved, size)
	h.End()

	got := h.Place(panel, testDragPane, testDragReserved)
	if hit := h.HitRect(got); hit.Overlaps(testDragReserved) {
		t.Fatalf("grip hit rect %v must clear the reserved cluster %v (card %v)", hit, testDragReserved, got)
	}
	if !got.In(testDragPane) {
		t.Fatalf("escape adjustment pushed the card %v out of the pane %v", got, testDragPane)
	}
}

// TestDragHandleReClampsOnGrowth: expanding a section grows the card; the
// pinned origin must slide just enough to keep it inside the pane.
func TestDragHandleReClampsOnGrowth(t *testing.T) {
	var h PopupDragHandle
	small := image.Rect(100, 150, 360, 250) // 260×100
	h.Begin(image.Pt(112, 162), small)
	h.MoveTo(image.Pt(512, 562), testDragPane, testDragReserved, image.Pt(260, 100))
	h.End()

	grown := image.Rect(0, 0, 260, 300) // same card, 3× taller
	got := h.Place(grown, testDragPane, testDragReserved)
	if !got.In(testDragPane) {
		t.Fatalf("grown card %v must be re-clamped into the pane %v", got, testDragPane)
	}
	if got.Dy() != 300 {
		t.Fatalf("re-clamp must preserve the requested height: want 300 got %d", got.Dy())
	}
}

// TestDragHandleResetRestoresAuto: closing the menu drops the pin, so the next
// open re-anchors beside its node.
func TestDragHandleResetRestoresAuto(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	h.Begin(image.Pt(112, 162), panel)
	h.MoveTo(image.Pt(300, 300), testDragPane, testDragReserved, image.Pt(260, 200))
	h.End()
	h.Reset()

	if h.Pinned() || h.Dragging() {
		t.Fatal("Reset must clear both pinned and dragging")
	}
	auto := image.Rect(500, 100, 760, 300)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("after Reset the handle must be transparent: want %v got %v", auto, got)
	}
}

// TestDragHandleHitRectMeetsTouchMin: the glyph stays small, but the mobile
// touch target must reach the 44px minimum.
func TestDragHandleHitRectMeetsTouchMin(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	grip := h.GripRect(panel)
	hit := h.HitRect(panel)

	if grip.Dx() != popupGripSize || grip.Dy() != popupGripSize {
		t.Fatalf("grip glyph should stay %dpx: got %dx%d", popupGripSize, grip.Dx(), grip.Dy())
	}
	min := TouchMinTarget()
	if hit.Dx() < min || hit.Dy() < min {
		t.Fatalf("hit rect %dx%d below touch min %d", hit.Dx(), hit.Dy(), min)
	}
	if !grip.In(hit) {
		t.Fatalf("hit rect %v must contain the glyph %v", hit, grip)
	}
}

// TestDragHandleGripSitsTopLeft: the affordance the user was promised is in
// the card's top-left corner.
func TestDragHandleGripSitsTopLeft(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	grip := h.GripRect(panel)
	if !grip.In(panel) {
		t.Fatalf("grip %v must sit inside the card %v", grip, panel)
	}
	if grip.Min.X > panel.Min.X+panel.Dx()/4 || grip.Min.Y > panel.Min.Y+panel.Dy()/4 {
		t.Fatalf("grip %v is not in the top-left quadrant of %v", grip, panel)
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestDragHandle
```

Expected: build failure — `undefined: PopupDragHandle`.

- [ ] **Step 3: Write the component**

Create `src/go/internal/ui/popup_drag_handle.go`:

```go
package ui

import "image"

const (
	// popupGripSize is the drag-grip glyph box. The visible affordance stays
	// this size on every platform; only the HIT rect grows on touch (HitRect),
	// mirroring the HitArea.Touch/ClipRect pattern used by the zone tree —
	// enlarging the visible box instead would push the card title around.
	popupGripSize = 16
	// popupGripInset is the grip's offset from the card's top-left corner. It
	// matches sidebarPad so the glyph lines up with the header band both menus
	// already inset by that amount.
	popupGripInset = sidebarPad
	// popupGripDragDeadzone is how far the pointer must travel before a press
	// on the grip counts as a drag. Below it the gesture is a tap and the card
	// stays in auto mode — otherwise a stray tap (or a mobile touch wobble)
	// would silently pin a card the user never meant to move.
	popupGripDragDeadzone = 3
)

// PopupDragHandle is the shared drag state for a grid-pane menu card whose
// position is otherwise derived every frame (node pop-up, group menu). The
// zero value is "auto": Place passes the caller's auto-anchored rect through
// untouched, so embedding the handle changes nothing until the user drags it.
//
// Once dragged, the handle is PINNED: it holds an absolute grid-pane origin
// and stops tracking the node/group/camera. Reset() (called from each menu's
// Open/Close) returns it to auto, so a pin never survives a reopen.
type PopupDragHandle struct {
	pinned   bool
	origin   image.Point // pinned top-left, grid-pane space
	dragging bool
	grab     image.Point // cursor − panel.Min at press: keeps the grip under the finger
	pressPt  image.Point // press origin, for the deadzone test
}

// GripRect is the glyph box in the card's top-left corner.
func (h *PopupDragHandle) GripRect(panel image.Rectangle) image.Rectangle {
	if panel.Empty() {
		return image.Rectangle{}
	}
	x := panel.Min.X + popupGripInset
	y := panel.Min.Y + popupGripInset
	return image.Rect(x, y, x+popupGripSize, y+popupGripSize)
}

// HitRect is the pressable area: the glyph on desktop, grown around the same
// center to the platform touch minimum on mobile (ExpandHitArea). Clamped so
// the growth can never reach outside the card it belongs to.
func (h *PopupDragHandle) HitRect(panel image.Rectangle) image.Rectangle {
	grip := h.GripRect(panel)
	if grip.Empty() {
		return image.Rectangle{}
	}
	want := ExpandHitArea(popupGripSize)
	grow := (want - popupGripSize) / 2
	if grow <= 0 {
		return grip
	}
	hit := grip.Inset(-grow)
	return hit.Intersect(panel)
}

// Dragging reports whether a grip drag is in flight (feeds Capturing() and the
// camera-pan gate).
func (h *PopupDragHandle) Dragging() bool { return h.dragging }

// Pinned reports whether the user has moved this card off its auto position.
func (h *PopupDragHandle) Pinned() bool { return h.pinned }

// Reset returns the handle to auto mode.
func (h *PopupDragHandle) Reset() {
	h.pinned = false
	h.dragging = false
	h.origin = image.Point{}
	h.grab = image.Point{}
	h.pressPt = image.Point{}
}

// Begin starts a drag from a press at pt inside panel. It records the grab
// offset so the card does not jump to put its corner under the cursor. It does
// NOT pin — see MoveTo's deadzone.
func (h *PopupDragHandle) Begin(pt image.Point, panel image.Rectangle) {
	h.dragging = true
	h.grab = pt.Sub(panel.Min)
	h.pressPt = pt
}

// MoveTo advances an in-flight drag to pt. Past the deadzone it pins the card
// at pt−grab, clamped by clampPopupOrigin. size is the card's CURRENT size.
func (h *PopupDragHandle) MoveTo(pt image.Point, pane, reserved image.Rectangle, size image.Point) {
	if !h.dragging {
		return
	}
	d := pt.Sub(h.pressPt)
	if !h.pinned && abs(d.X) < popupGripDragDeadzone && abs(d.Y) < popupGripDragDeadzone {
		return
	}
	h.pinned = true
	h.origin = clampPopupOrigin(pt.Sub(h.grab), size, pane, reserved)
}

// End finishes the drag, keeping the pin.
func (h *PopupDragHandle) End() { h.dragging = false }

// Place is the chokepoint every consumer calls at the end of its layout: auto
// is the rect the menu's own anchoring produced this frame. In auto mode it is
// returned verbatim (zero behavior change); pinned, the stored origin is
// re-clamped against the CURRENT size — so a card that grew a section still
// ends up fully inside the pane.
func (h *PopupDragHandle) Place(auto, pane, reserved image.Rectangle) image.Rectangle {
	if !h.pinned {
		return auto
	}
	size := image.Pt(auto.Dx(), auto.Dy())
	o := clampPopupOrigin(h.origin, size, pane, reserved)
	h.origin = o
	return image.Rect(o.X, o.Y, o.X+size.X, o.Y+size.Y)
}

// clampPopupOrigin enforces the two placement rules from the design:
//  1. a size-sized card at the returned origin sits fully inside pane;
//  2. its grip hit rect does not overlap reserved (the gear/file/logs cluster,
//     which sits at a higher z and would swallow presses) — the escape
//     guarantee that keeps a parked card draggable.
//
// Rule 2 resolves by the SMALLEST axis-aligned push that clears the cluster
// and still satisfies rule 1, so the card slides the short way out rather than
// teleporting across the pane.
func clampPopupOrigin(origin, size image.Point, pane, reserved image.Rectangle) image.Point {
	o := clampOriginInPane(origin, size, pane)
	if reserved.Empty() {
		return o
	}
	var h PopupDragHandle
	panelAt := func(p image.Point) image.Rectangle {
		return image.Rect(p.X, p.Y, p.X+size.X, p.Y+size.Y)
	}
	hit := h.HitRect(panelAt(o))
	if hit.Empty() || !hit.Overlaps(reserved) {
		return o
	}
	// Four candidate pushes: out the left, right, top, bottom of the cluster.
	deltas := []image.Point{
		{X: reserved.Min.X - hit.Max.X},
		{X: reserved.Max.X - hit.Min.X},
		{Y: reserved.Min.Y - hit.Max.Y},
		{Y: reserved.Max.Y - hit.Min.Y},
	}
	best, haveBest := o, false
	bestCost := 0
	for _, d := range deltas {
		cand := clampOriginInPane(o.Add(d), size, pane)
		if ch := h.HitRect(panelAt(cand)); ch.Empty() || ch.Overlaps(reserved) {
			continue // clamping undid the push — this direction has no room
		}
		cost := abs(cand.X-o.X) + abs(cand.Y-o.Y)
		if !haveBest || cost < bestCost {
			best, bestCost, haveBest = cand, cost, true
		}
	}
	if !haveBest {
		// Pane too small for any escape (tiny window): keep rule 1 and accept
		// the overlap — Esc and tap-away still dismiss the card.
		return o
	}
	return best
}

// clampOriginInPane slides a size-sized card so it lies inside pane. A card
// larger than the pane on an axis is pinned to the pane's min on that axis
// (the menus already cap their own size to the pane, so this is a guard).
func clampOriginInPane(origin, size image.Point, pane image.Rectangle) image.Point {
	o := origin
	if max := pane.Max.X - size.X; o.X > max {
		o.X = max
	}
	if o.X < pane.Min.X {
		o.X = pane.Min.X
	}
	if max := pane.Max.Y - size.Y; o.Y > max {
		o.Y = max
	}
	if o.Y < pane.Min.Y {
		o.Y = pane.Min.Y
	}
	return o
}
```

Note: `abs(int) int` — check whether the package already has it before adding one:

```bash
grep -rn "^func abs(" src/go/internal/ui/*.go
```

If it does not exist, add it to `popup_drag_handle.go`:

```go
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

If it exists with a different name (e.g. `absInt`), use the existing one and drop this helper — do not add a duplicate.

- [ ] **Step 4: Format and run the tests**

```bash
../../.tools/go/bin/gofmt -w src/go/internal/ui/popup_drag_handle.go src/go/internal/ui/popup_drag_handle_test.go
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'TestDragHandle|TestGripIcon'
```

Expected: all PASS.

---

### Task 3: Node pop-up integration

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` — struct (~line 52), `Open` (~112), `Close` (~132), `layout` (~407-434), `drawHeader` (~1870), `HandleInput` (~1100), `Capturing` (~1407)
- Test: `src/go/internal/ui/popup_drag_menus_test.go` (create)

**Interfaces:**
- Consumes: `PopupDragHandle` and every method from Task 2; `IconGrip` from Task 1; `(*Game).gridTopButtonsReservedRect() image.Rectangle` (`grid_reserved_zone.go:36`); `AnchorPopupRect`, `avoidReservedRect`.
- Produces: `sb.drag` reachable from tests as `g.sidebar.drag`; `sb.rects["grip"]` published in grid-pane screen space.

- [ ] **Step 1: Write the failing functional test**

Create `src/go/internal/ui/popup_drag_menus_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// dragRig drives real g.Update() frames with scriptable pointer state — the
// project's functional-over-isolation rule: the grip must work through the
// actual dispatcher, not a hand-called HandleInput.
type dragRig struct {
	g    *Game
	x, y int
	down bool
}

func newDragRig(t *testing.T, w, h int) *dragRig {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)

	r := &dragRig{g: g}
	restore := SetInputForTest(
		func() (int, int) { return r.x, r.y },
		func(b ebiten.MouseButton) bool { return r.down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	return r
}

// moveTo positions the pointer and runs one frame.
func (r *dragRig) moveTo(x, y int) {
	r.x, r.y = x, y
	_ = r.g.Update()
}

// dragGrip presses the grip at from, walks to to in steps, and releases.
func (r *dragRig) dragGrip(from, to image.Point) {
	r.x, r.y = from.X, from.Y
	r.down = true
	_ = r.g.Update() // press frame
	steps := 5
	for i := 1; i <= steps; i++ {
		r.moveTo(
			from.X+(to.X-from.X)*i/steps,
			from.Y+(to.Y-from.Y)*i/steps,
		)
	}
	r.down = false
	_ = r.g.Update() // release frame
}

func openNodeMenuForDrag(t *testing.T, r *dragRig) *uiNode {
	t.Helper()
	n := r.g.tryAddNode(2, 1, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	r.g.updateBeatInfos()
	r.g.sel = n
	n.Selected = true
	r.g.sidebar.Open(n)
	_ = r.g.Update()
	return n
}

// TestNodeMenuGripDragMovesCard is the headline behavior: press the grip, move,
// release — the card is where it was dropped.
func TestNodeMenuGripDragMovesCard(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	before := r.g.sidebar.rects["panel"]
	grip := r.g.sidebar.rects["grip"]
	if grip.Empty() {
		t.Fatal(`sidebar must publish rects["grip"]`)
	}
	if !grip.In(before) {
		t.Fatalf("grip %v must sit inside the card %v", grip, before)
	}

	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	to := from.Add(image.Pt(120, 60))
	r.dragGrip(from, to)

	after := r.g.sidebar.rects["panel"]
	if after == before {
		t.Fatalf("card did not move: still %v", before)
	}
	if got, want := after.Min.Sub(before.Min), image.Pt(120, 60); got != want {
		t.Fatalf("card should track the cursor delta: want %v got %v", want, got)
	}
	if after.Dx() != before.Dx() || after.Dy() != before.Dy() {
		t.Fatalf("drag must not resize the card: %v → %v", before, after)
	}
}

// TestNodeMenuGripDragPinSurvivesFrames: the pin is not a one-frame nudge that
// the next layout re-anchors away.
func TestNodeMenuGripDragPinSurvivesFrames(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(140, 40)))
	dropped := r.g.sidebar.rects["panel"]

	for i := 0; i < 30; i++ {
		_ = r.g.Update()
	}
	if got := r.g.sidebar.rects["panel"]; got != dropped {
		t.Fatalf("pinned card drifted over 30 frames: %v → %v", dropped, got)
	}
}

// TestNodeMenuGripDragSurvivesCameraPan: pinned means pinned to the SCREEN —
// panning the grid must not drag the card along with its node.
func TestNodeMenuGripDragSurvivesCameraPan(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(100, 50)))
	dropped := r.g.sidebar.rects["panel"]

	r.g.cam.OffsetX += 90
	r.g.cam.OffsetY += 45
	_ = r.g.Update()

	if got := r.g.sidebar.rects["panel"]; got != dropped {
		t.Fatalf("pinned card followed the camera: %v → %v", dropped, got)
	}
}

// TestNodeMenuReopenResetsToDefault: the pin is per-open, per the design.
func TestNodeMenuReopenResetsToDefault(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	n := openNodeMenuForDrag(t, r)
	def := r.g.sidebar.rects["panel"]

	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(130, 70)))
	if r.g.sidebar.rects["panel"] == def {
		t.Fatal("precondition failed: card never moved")
	}

	r.g.sidebar.Close()
	_ = r.g.Update()
	r.g.sidebar.Open(n)
	_ = r.g.Update()

	if got := r.g.sidebar.rects["panel"]; got != def {
		t.Fatalf("reopen should restore the default anchored position: want %v got %v", def, got)
	}
}

// TestNodeMenuGripPressCreatesNoNode: the grip press must be consumed, never
// fall through to the grid editor.
func TestNodeMenuGripPressCreatesNoNode(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)
	before := len(r.g.nodes) // the UI-side node slice, as eq_wheel_popup_isolation_test.go does

	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(110, 55)))

	if after := len(r.g.nodes); after != before {
		t.Fatalf("grip drag changed the node count: %d → %d", before, after)
	}
}

// TestNodeMenuGripDragDoesNotScroll: the grip lives in the header band, above
// the scroll viewport — a drag on it must not scroll the card's content.
func TestNodeMenuGripDragDoesNotScroll(t *testing.T) {
	r := newDragRig(t, 640, 250) // short pane → content overflows
	openNodeMenuForDrag(t, r)
	r.g.sidebar.ExpandAllSections()
	_ = r.g.Update()

	before := r.g.sidebar.scroll.VS.First
	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(20, 60)))

	if after := r.g.sidebar.scroll.VS.First; after != before {
		t.Fatalf("grip drag scrolled the content: %d → %d", before, after)
	}
}

// TestNodeMenuStaysInGridPaneWhenDragged: the card renders through the
// grid-sidebar GridTree layer (clip:true) — anything outside the pane is
// scissored away, so the drag must never put it there.
func TestNodeMenuStaysInGridPaneWhenDragged(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	gridPane := image.Rect(0, 0, r.g.split.GridW(r.g.winW), r.g.split.GridH(r.g.winH))
	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, image.Pt(4000, 4000))

	if got := r.g.sidebar.rects["panel"]; !got.In(gridPane) {
		t.Fatalf("dragged card %v escaped the grid pane %v", got, gridPane)
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestNodeMenu
```

Expected: FAIL — `sidebar must publish rects["grip"]`.

- [ ] **Step 3: Add the drag field**

In `game_node_sidebar.go`'s `NodeSidebar` struct, after the `hover` field:

```go
	// drag is the top-left grip handle. Zero value = auto mode: layout's
	// node-anchored placement passes through untouched. Once the user drags
	// the grip the card is PINNED to the grid pane and stops tracking the
	// node/camera; Open/Close reset it so a pin never survives a reopen.
	drag PopupDragHandle
```

- [ ] **Step 4: Reset the handle on Open and Close**

In `Open`, alongside the other per-open resets (after `sb.deferredTap.Cancel()`):

```go
	sb.drag.Reset()
```

In `Close`, after `sb.closedGuard = 2`:

```go
	sb.drag.Reset()
```

- [ ] **Step 5: Divert the origin in `layout` and publish the grip rect**

In `layout()`, replace the origin derivation (currently `origin := anchored.Min` after the `avoidReservedRect` block) with:

```go
	// The grip handle gets the last word: in auto mode Place returns `anchored`
	// verbatim, so default placement is unchanged; once the user has dragged,
	// it returns the pinned origin re-clamped against the CURRENT card size.
	gridPaneRect := image.Rect(0, 0, g.split.GridW(g.winW), gridH)
	placed := sb.drag.Place(anchored, gridPaneRect, g.gridTopButtonsReservedRect())
	origin := placed.Min
```

Then, immediately after the existing translate block (`if origin.X != 0 || origin.Y != 0 { … }`) and before `sb.wireButtons()`:

```go
	// Grip rect is published in FINAL screen space — after the translate — so
	// draw and hit-test agree without either re-deriving the origin.
	sb.rects["grip"] = sb.drag.GripRect(sb.rects["panel"])
```

- [ ] **Step 6: Shift the title clear of the grip and draw the glyph**

In `drawHeader`, replace the `titleRect` line:

```go
	titleRect := image.Rect(headerRect.Min.X+2, headerRect.Min.Y, headerRect.Max.X, titleBottom)
```

with:

```go
	// Title starts clear of the grip glyph so the two never overlap.
	titleX := headerRect.Min.X + 2
	if gr, ok := sb.rects["grip"]; ok && !gr.Empty() {
		if x := gr.Max.X + sidebarGap; x > titleX {
			titleX = x
		}
	}
	titleRect := image.Rect(titleX, headerRect.Min.Y, headerRect.Max.X, titleBottom)
```

And after `drawMenuTitle(dst, titleRect, label, swatchCol)`:

```go
	// Drag grip: dim by default, brighter on hover, accent while dragging —
	// the standard three-state affordance tint.
	if gr, ok := sb.rects["grip"]; ok && !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(sb.drag.Dragging(), sb.gripHovered()))
	}
```

Add the two small helpers at the bottom of `popup_drag_handle.go`:

```go
// popupGripTint is the grip's three-state tint: resting (dim, same treatment
// as the bottom-sheet drag handle), hovered, and dragging.
func popupGripTint(dragging, hovered bool) color.Color {
	switch {
	case dragging:
		return colAccent
	case hovered:
		return colTextSecondary
	default:
		return WithAlpha(genColorBorder, genAlphaScrollbarThumb)
	}
}
```

…which needs `"image/color"` added to that file's imports. And in `game_node_sidebar.go`, next to the other small helpers:

```go
// gripHovered reports whether the desktop cursor is over the drag grip. Mobile
// has no hover state, so it always reports false.
func (sb *NodeSidebar) gripHovered() bool {
	if Profile().IsMobile() {
		return false
	}
	gr, ok := sb.rects["grip"]
	if !ok || gr.Empty() {
		return false
	}
	mx, my := cursorPosition()
	return image.Pt(mx, my).In(gr)
}
```

- [ ] **Step 7: Handle the drag in `HandleInput`**

In `HandleInput`, insert immediately after the `sb.layout()` call and the `prevSecHeld` bookkeeping, **before** the `panelR := sb.rects["panel"]` scroll ladder (the grip sits in the header band, above the scroll viewport, so ordering it first cannot starve scrolling):

```go
	// Grip drag — highest precedence inside the card. Returning InputCaptured
	// makes the dispatcher route every later frame straight here without
	// re-hit-testing (input_dispatch.go), so the drag survives the cursor
	// leaving the card.
	gridPaneR := image.Rect(0, 0, sb.game.split.GridW(sb.game.winW), sb.game.split.GridH(sb.game.winH))
	if sb.drag.Dragging() {
		panel := sb.rects["panel"]
		if pressed {
			sb.drag.MoveTo(image.Pt(x, y), gridPaneR, sb.game.gridTopButtonsReservedRect(),
				image.Pt(panel.Dx(), panel.Dy()))
			sb.layout()
		} else {
			sb.drag.End()
		}
		return InputCaptured
	}
	if pressed {
		if hit := sb.drag.HitRect(sb.rects["panel"]); !hit.Empty() && image.Pt(x, y).In(hit) {
			sb.drag.Begin(image.Pt(x, y), sb.rects["panel"])
			return InputCaptured
		}
	}
```

- [ ] **Step 8: Report the capture**

`Capturing()` (`game_node_sidebar.go:1407`) currently reads
`return sb.scroll.TouchActive() || sb.scroll.Dragging()`. Add the drag term at the front, preserving both existing terms:

```go
func (sb *NodeSidebar) Capturing() bool {
	return sb.drag.Dragging() || sb.scroll.TouchActive() || sb.scroll.Dragging()
}
```

- [ ] **Step 9: Run the tests and confirm they pass**

```bash
../../.tools/go/bin/gofmt -w src/go/internal/ui/game_node_sidebar.go src/go/internal/ui/popup_drag_handle.go
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestNodeMenu
```

Expected: all PASS. `TestNodeMenuGripDragSurvivesCameraPan` may still fail here if the camera panned during the drag — that is Task 5's gate. If it fails for that reason, note it and continue; it must be green by the end of Task 5.

- [ ] **Step 10: Check for regressions in the node-menu suite**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'Sidebar|NodePopup|NodeMenu|NodeSidebar'
```

Expected: PASS. Existing tests that assert exact header/title x-positions may need updating for the grip shift — update the assertion, do not shrink the grip.

---

### Task 4: Group menu integration

**Files:**
- Modify: `src/go/internal/ui/game_group_menu.go` — struct (~line 30), `openAt` (~121), `Close` (~227), `layoutAt` (~812-832), `Draw` (~1097), `HandleInput` (~260), `Capturing` (~257)
- Test: `src/go/internal/ui/popup_drag_menus_test.go` (extend)

**Interfaces:**
- Consumes: everything Task 2 produces; `dragRig`, `newDragRig` from Task 3's test file; `placeGroupMenu(anchor, size image.Point, members []image.Rectangle, gearZone image.Rectangle, gridW, gridH int) image.Rectangle` (`grid_reserved_zone.go:131`).
- Produces: `m.drag`; `m.rects["grip"]`.

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/popup_drag_menus_test.go`:

```go
// openGroupMenuForDrag builds a three-node group and opens its menu, mirroring
// the group_menu_open screenshot scene.
func openGroupMenuForDrag(t *testing.T, r *dragRig) model.GroupID {
	t.Helper()
	a := r.g.tryAddNode(2, 1, model.NodeTypeRegular)
	b := r.g.tryAddNode(3, 1, model.NodeTypeRegular)
	c := r.g.tryAddNode(4, 1, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil {
		t.Fatal("could not build the group's nodes")
	}
	r.g.updateBeatInfos()
	gid, err := r.g.graph.CreateGroup("Drums", []model.NodeID{a.ID, b.ID, c.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	_, sy1, sx2, _ := r.g.nodeScreenRect(c)
	r.g.groupMenu.OpenAt(gid, int(sx2)+24, int(sy1))
	_ = r.g.Update()
	return gid
}

// TestGroupMenuGripDragMovesCard: the headline behavior for the second menu.
func TestGroupMenuGripDragMovesCard(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	before := r.g.groupMenu.panel
	grip := r.g.groupMenu.rects["grip"]
	if grip.Empty() {
		t.Fatal(`group menu must publish rects["grip"]`)
	}
	if !grip.In(before) {
		t.Fatalf("grip %v must sit inside the card %v", grip, before)
	}

	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(-90, 100)))

	after := r.g.groupMenu.panel
	if got, want := after.Min.Sub(before.Min), image.Pt(-90, 100); got != want {
		t.Fatalf("card should track the cursor delta: want %v got %v", want, got)
	}
}

// TestGroupMenuPinnedIgnoresMemberAvoidance: auto placement dodges member
// rects; once pinned, the card must stay where it was dropped even if a member
// moves under it (camera pan), not get shoved aside.
func TestGroupMenuPinnedIgnoresMemberAvoidance(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	grip := r.g.groupMenu.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(-120, 60)))
	dropped := r.g.groupMenu.panel

	// Pan so the members slide under the dropped card.
	r.g.cam.OffsetX -= 140
	for i := 0; i < 5; i++ {
		_ = r.g.Update()
	}
	if got := r.g.groupMenu.panel; got != dropped {
		t.Fatalf("pinned group card was displaced by member avoidance: %v → %v", dropped, got)
	}
}

// TestGroupMenuReopenResetsToDefault: pin is per-open here too.
func TestGroupMenuReopenResetsToDefault(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	gid := openGroupMenuForDrag(t, r)
	def := r.g.groupMenu.panel
	anchorX, anchorY := r.g.groupMenu.anchorX, r.g.groupMenu.anchorY

	grip := r.g.groupMenu.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(-100, 80)))
	if r.g.groupMenu.panel == def {
		t.Fatal("precondition failed: card never moved")
	}

	r.g.groupMenu.Close()
	_ = r.g.Update()
	r.g.groupMenu.OpenAt(gid, anchorX, anchorY)
	_ = r.g.Update()

	if got := r.g.groupMenu.panel; got != def {
		t.Fatalf("reopen should restore the default placement: want %v got %v", def, got)
	}
}

// TestGroupMenuCapturingDuringDrag: without this the dispatcher drops the
// capture mid-gesture and the card stops following the cursor.
func TestGroupMenuCapturingDuringDrag(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	grip := r.g.groupMenu.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.x, r.y = from.X, from.Y
	r.down = true
	_ = r.g.Update()
	r.moveTo(from.X-40, from.Y+30)

	if !r.g.groupMenu.Capturing() {
		t.Fatal("group menu must report Capturing() while a grip drag is in flight")
	}
	r.down = false
	_ = r.g.Update()
	if r.g.groupMenu.Capturing() {
		t.Fatal("Capturing() must clear on release")
	}
}

// TestGroupMenuStaysInGridPaneWhenDragged mirrors the node-card containment
// guarantee.
func TestGroupMenuStaysInGridPaneWhenDragged(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	gridPane := image.Rect(0, 0, r.g.split.GridW(r.g.winW), r.g.split.GridH(r.g.winH))
	grip := r.g.groupMenu.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, image.Pt(-4000, 4000))

	if got := r.g.groupMenu.panel; !got.In(gridPane) {
		t.Fatalf("dragged group card %v escaped the grid pane %v", got, gridPane)
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run TestGroupMenuGrip
```

Expected: FAIL — `group menu must publish rects["grip"]`.

- [ ] **Step 3: Add the drag field**

In `game_group_menu.go`'s `GroupMenu` struct, after `btns`:

```go
	// drag is the top-left grip handle — same contract as NodeSidebar.drag:
	// zero value passes placement through, a drag pins the card to the grid
	// pane, openAt/Close reset it.
	drag PopupDragHandle
```

- [ ] **Step 4: Reset on open and close**

In `openAt`, right after `m.provisional = provisional`:

```go
	m.drag.Reset()
```

In `Close`, alongside the other state teardown:

```go
	m.drag.Reset()
```

- [ ] **Step 5: Divert placement in `layoutAt` and publish the grip rect**

In `layoutAt`, replace the placement block:

```go
	panel := placeGroupMenu(image.Pt(x, y), image.Pt(w, h), members, reserved, gridW, gridH)
	m.panel = panel
	m.rects["panel"] = panel
```

with:

```go
	// While PINNED, skip auto-placement entirely rather than computing it and
	// overriding: placeGroupMenu's member-overlap pass would otherwise re-run
	// every frame against a result it can't affect, and a member drifting under
	// the card must not shove it off the user's drop point.
	gridPaneRect := image.Rect(0, 0, gridW, gridH)
	panel := image.Rect(0, 0, w, h)
	if !m.drag.Pinned() {
		panel = placeGroupMenu(image.Pt(x, y), image.Pt(w, h), members, reserved, gridW, gridH)
	}
	panel = m.drag.Place(panel, gridPaneRect, g.gridTopButtonsReservedRect())
	m.panel = panel
	m.rects["panel"] = panel
	m.rects["grip"] = m.drag.GripRect(panel)
```

- [ ] **Step 6: Shift the title clear of the grip**

Still in `layoutAt`, replace the `headerR` line:

```go
	headerR := image.Rect(panel.Min.X+sidebarPad, panel.Min.Y+sidebarPad, panel.Max.X-sidebarPad, panel.Min.Y+sidebarPad+sidebarHeaderH)
	m.rects["title"] = headerR
```

with:

```go
	headerR := image.Rect(panel.Min.X+sidebarPad, panel.Min.Y+sidebarPad, panel.Max.X-sidebarPad, panel.Min.Y+sidebarPad+sidebarHeaderH)
	// Title starts clear of the grip glyph; Save/close keep their right-aligned
	// header slots, so only the left edge moves.
	titleR := headerR
	if gr := m.rects["grip"]; !gr.Empty() {
		if x := gr.Max.X + sidebarGap; x > titleR.Min.X {
			titleR.Min.X = x
		}
	}
	m.rects["title"] = titleR
```

Note `headerR` (not `titleR`) stays the basis for the close/Save rects below — do not change those lines.

- [ ] **Step 7: Draw the glyph**

In `Draw`, after the panel border is painted (`drawRoundedRect(dst, panel, colPanelBorder, RadiusContainer, false)`):

```go
	if gr := m.rects["grip"]; !gr.Empty() {
		DrawIcon(dst, IconGrip, gr, popupGripTint(m.drag.Dragging(), m.gripHovered()))
	}
```

And add the hover helper next to the other `GroupMenu` helpers:

```go
// gripHovered reports whether the desktop cursor is over the drag grip.
func (m *GroupMenu) gripHovered() bool {
	if Profile().IsMobile() {
		return false
	}
	gr := m.rects["grip"]
	if gr.Empty() {
		return false
	}
	mx, my := cursorPosition()
	return image.Pt(mx, my).In(gr)
}
```

- [ ] **Step 8: Handle the drag in `HandleInput`**

In `HandleInput`, insert immediately after the `m.layoutAt(m.anchorX, m.anchorY)` refresh and **before** the `pt := image.Pt(x, y)` / outside-close block:

```go
	gridPaneR := image.Rect(0, 0, m.game.split.GridW(m.game.winW), m.game.split.GridH(m.game.winH))
	if m.drag.Dragging() {
		if pressed {
			m.drag.MoveTo(image.Pt(x, y), gridPaneR, m.game.gridTopButtonsReservedRect(),
				image.Pt(m.panel.Dx(), m.panel.Dy()))
			m.layoutAt(m.anchorX, m.anchorY)
		} else {
			m.drag.End()
		}
		return InputCaptured
	}
	if pressed {
		if hit := m.drag.HitRect(m.panel); !hit.Empty() && image.Pt(x, y).In(hit) {
			m.drag.Begin(image.Pt(x, y), m.panel)
			return InputCaptured
		}
	}
```

- [ ] **Step 9: Report the capture**

Replace `Capturing`:

```go
// Capturing: true only while a grip drag is in flight (the menu has no
// scrollable content, so this is its sole capture state).
func (m *GroupMenu) Capturing() bool { return m.drag.Dragging() }
```

- [ ] **Step 10: Run the tests and confirm they pass**

```bash
../../.tools/go/bin/gofmt -w src/go/internal/ui/game_group_menu.go
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'TestGroupMenu|TestGroupMenuGrip'
```

Expected: all PASS. As in Task 3, a camera-pan-related failure is Task 5's job.

- [ ] **Step 11: Check the placement suite for regressions**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'GroupMenu|GroupBadge|MenuOverlaps'
```

Expected: PASS. `TestMenuOverlapsOnlyWhenNoRoom` is the sensitive one — auto placement must be byte-identical to before, since `Place` is a pass-through in auto mode. If it fails, the pinned branch is leaking into the auto path.

---

### Task 5: Camera-pan gate

**Files:**
- Modify: `src/go/internal/ui/game_input_drag.go` (add after `menuHit`, ~line 72)
- Modify: `src/go/internal/ui/game_update.go:692` (`panOK`)
- Test: `src/go/internal/ui/popup_drag_menus_test.go` (extend)

**Interfaces:**
- Consumes: `(*PopupDragHandle).Dragging()`; `g.sidebar.drag`, `g.groupMenu.drag`.
- Produces: `func (g *Game) gridMenuDragging() bool`.

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/popup_drag_menus_test.go`:

```go
// TestGripDragDoesNotPanCamera: panOK gates on menuHit(cursor), which is
// position-based — once the drag pulls the cursor off the card the grid would
// start panning under the moving menu. The drag state must gate it instead.
func TestGripDragDoesNotPanCamera(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	beforeX, beforeY := r.g.cam.OffsetX, r.g.cam.OffsetY

	// Drag far enough that the cursor leaves the card entirely.
	grip := r.g.sidebar.rects["grip"]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(300, 120)))

	if r.g.cam.OffsetX != beforeX || r.g.cam.OffsetY != beforeY {
		t.Fatalf("camera panned during a grip drag: (%v,%v) → (%v,%v)",
			beforeX, beforeY, r.g.cam.OffsetX, r.g.cam.OffsetY)
	}
}

// TestGridMenuDraggingReportsBothMenus keeps the gate honest for the group
// menu too — a helper that only checks the sidebar would silently regress.
func TestGridMenuDraggingReportsBothMenus(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	if r.g.gridMenuDragging() {
		t.Fatal("no drag in flight yet")
	}
	grip := r.g.groupMenu.rects["grip"]
	r.x, r.y = grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2
	r.down = true
	_ = r.g.Update()
	r.moveTo(r.x-60, r.y+40)

	if !r.g.gridMenuDragging() {
		t.Fatal("gridMenuDragging must report the group menu's drag")
	}
	r.down = false
	_ = r.g.Update()
	if r.g.gridMenuDragging() {
		t.Fatal("gridMenuDragging must clear on release")
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'TestGripDragDoesNotPanCamera|TestGridMenuDragging'
```

Expected: build failure — `g.gridMenuDragging undefined`.

- [ ] **Step 3: Add the helper**

In `game_input_drag.go`, immediately after `menuHit`:

```go
// gridMenuDragging reports whether either grid-pane menu card is being dragged
// by its grip. menuHit (above) is cursor-POSITION based, so once a grip drag
// pulls the pointer off the card it stops suppressing the camera — the grid
// would then pan under the moving menu. The camera gate consults this instead.
func (g *Game) gridMenuDragging() bool {
	return (g.sidebar != nil && g.sidebar.drag.Dragging()) ||
		(g.groupMenu != nil && g.groupMenu.drag.Dragging())
}
```

- [ ] **Step 4: Gate `panOK`**

In `game_update.go:692`, add the term to the existing chain (keep every other term exactly as-is):

```go
	panOK := !g.linkDrag.active && !g.marquee.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds) && !g.drum.Capturing() && !g.menuHit(mx, my) && !g.gridMenuDragging() && !g.longPressPopup && !g.modalOverlayActive() && !blockLandscapeInput
```

- [ ] **Step 5: Run the tests and confirm they pass**

```bash
../../.tools/go/bin/gofmt -w src/go/internal/ui/game_input_drag.go src/go/internal/ui/game_update.go
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'TestGripDrag|TestGridMenuDragging|TestNodeMenu|TestGroupMenuGrip|TestDragHandle'
```

Expected: all PASS, including the two camera-pan tests deferred from Tasks 3 and 4.

- [ ] **Step 6: Run the full `internal/ui` suite**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/
```

Expected: green except the two known-unrelated reds (`TestSamplerMobileOverflowMenuHasSaveAsReset`, `TestFocusGraph_RespondsToEveryKnob`). Any other failure belongs to this work — prove causation before touching it.

---

### Task 6: Screenshot scenes + visual verification (MANDATORY, GATING)

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` (near `node_sidebar_open` ~line 331 and `group_menu_open` ~line 362)

**Interfaces:**
- Consumes: everything from Tasks 1-5; `sceneGroupMenuSetup` (`scene_catalog.go:2247`).
- Produces: scenes `node_menu_dragged`, `group_menu_dragged`.

- [ ] **Step 1: Add the scenes**

In `scene_catalog.go`, after the `node_sidebar_low_node` entry:

```go
	{Name: "node_menu_dragged", Description: "node pop-up moved by its top-left drag grip — card sits at the dropped position (not node-anchored), grip glyph visible, title clear of it", Mobile: true,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n == nil || g.sidebar == nil {
				return
			}
			g.sidebar.Open(n)
			g.sidebar.layout()
			sceneDragMenuGrip(g, g.sidebar.rects["panel"], &g.sidebar.drag, image.Pt(140, 90))
			g.sidebar.layout()
		}},
```

…and after the `group_menu_open` entry:

```go
	{Name: "group_menu_dragged", Description: "group menu moved by its top-left drag grip — card sits at the dropped position, member avoidance no longer applies, grip glyph visible", Mobile: true,
		Setup: func(g *Game) {
			sceneGroupMenuSetup(g)
			if g.groupMenu == nil || !g.groupMenu.IsOpen() {
				return
			}
			sceneDragMenuGrip(g, g.groupMenu.panel, &g.groupMenu.drag, image.Pt(-120, 110))
			g.groupMenu.layoutAt(g.groupMenu.anchorX, g.groupMenu.anchorY)
		}},
```

…plus the shared helper next to `sceneGroupMenuSetup`:

```go
// sceneDragMenuGrip drives a grid-pane menu card through a grip drag by delta,
// using the same PopupDragHandle entry points the real press/move/release path
// uses — so the captured scene reflects genuine drag state, not a hand-set
// origin. Mirrors the SetFrozen/SetAutoGain scene-setter pattern.
func sceneDragMenuGrip(g *Game, panel image.Rectangle, h *PopupDragHandle, delta image.Point) {
	if panel.Empty() {
		return
	}
	grip := h.GripRect(panel)
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	pane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	h.Begin(from, panel)
	h.MoveTo(from.Add(delta), pane, g.gridTopButtonsReservedRect(), image.Pt(panel.Dx(), panel.Dy()))
	h.End()
}
```

- [ ] **Step 2: Verify the scenes are registered and their setups are sane**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/ -run 'Scene'
```

Expected: PASS (the catalog tests exercise every scene's Setup).

- [ ] **Step 3: Build the screenshot binary**

From the repo root:

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
```

- [ ] **Step 4: Capture desktop**

```bash
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene node_menu_dragged  -screenshot /tmp/shot_node_dragged.png
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene group_menu_dragged -screenshot /tmp/shot_group_dragged.png
```

- [ ] **Step 5: Capture mobile (portrait is REQUIRED — landscape renders "rotate device")**

```bash
xvfb-run -a -s "-screen 0 390x844x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene node_menu_dragged  -screenshot /tmp/shot_node_dragged_mobile.png
xvfb-run -a -s "-screen 0 390x844x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene group_menu_dragged -screenshot /tmp/shot_group_dragged_mobile.png
```

Note: `MOBILE=1` as an env var does nothing to this binary — the `-mobile -win-w -win-h` flags are what switch the profile.

- [ ] **Step 6: LOOK at all four PNGs**

Open each with the Read tool and confirm, explicitly, for each:

1. the grip glyph (2×3 dots) is present in the card's top-left corner and legible;
2. the card title does not collide with or run under the glyph;
3. the card is intact at the dropped position — no clipped edge, no spill past the split into the drum-view panel;
4. the close × (desktop) and Save (group menu) still sit in their right-aligned header slots;
5. nothing else in the frame regressed.

If any check fails: fix, rebuild, re-capture, look again. Do not proceed on a PNG you have not opened.

To zoom a region for inspection (ffmpeg is the available tool — no imagemagick/PIL):

```bash
ffmpeg -y -i /tmp/shot_node_dragged.png -vf "crop=W:H:X:Y,scale=iw*3:ih*3:flags=neighbor" /tmp/crop.png
```

- [ ] **Step 7: Final full-suite verification**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1800s ./internal/ui/
```

Expected: green except the two known-unrelated reds. Report the result honestly, including anything left red and why.

- [ ] **Step 8: Report, do not commit**

Summarize: files changed, tests added, screenshot paths inspected. Leave everything unstaged for the user's review.

---

## Self-Review

**Spec coverage:** auto/pinned behavior → Tasks 3-4 (`Place`) and their reopen tests; `PopupDragHandle` API → Task 2, every method exercised; clamp rules 1 and 2 → Task 2 tests `ClampsInsidePane` / `GripEscapesReservedZone`, re-clamp on growth → `ReClampsOnGrowth`; `IconGrip` → Task 1; node-menu integration incl. title shift → Task 3 steps 5-6; group-menu integration incl. skipping auto-placement when pinned → Task 4 step 5 + `PinnedIgnoresMemberAvoidance`; input branch + `InputCaptured` + `Capturing` → Tasks 3 step 7-8, 4 step 8-9, `TestGroupMenuCapturingDuringDrag`; edge case 1 (camera pan) → Task 5; edge case 2 (growth) → Task 2; edge case 3 (mobile touch min) → Task 2 `HitRectMeetsTouchMin` + Task 6 mobile captures; edge case 4 (shift+press) → the grip branch is placed *after* the existing shift check in Task 3 step 7, which is called out in the step; telemetry (none) → no task, by design; testing → Tasks 2-6.

**Placeholders:** none — every code step carries the literal code; no "similar to Task N" back-references (Task 4 repeats its code in full).

**Type consistency:** `Place(auto, pane, reserved)`, `MoveTo(pt, pane, reserved, size)`, `HitRect(panel)`, `GripRect(panel)`, `Begin(pt, panel)`, `End()`, `Dragging()`, `Pinned()`, `Reset()` are used with identical signatures in Tasks 2, 3, 4, 5 and 6. `popupGripTint(dragging, hovered bool)` is defined once in Task 3 and reused in Task 4. `sb.rects["grip"]` / `m.rects["grip"]` is the single published key both draw and test read.

**Known risk flagged for the implementer:** Task 3 step 10 and Task 4 step 11 may surface existing tests that assert exact header/title geometry. Update those assertions to account for the grip shift; do not shrink the grip to fit an old assertion.
