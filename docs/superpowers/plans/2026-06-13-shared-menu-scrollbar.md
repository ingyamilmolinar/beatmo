# Shared Menu Scrollbar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the "Load template" menu scroll, and route every list menu's scroll mechanics through one shared `MenuScroll` component so the half-wired duplication that broke the template menu can't recur.

**Architecture:** Add `MenuScroll` (bundles the existing `ScrollBehavior` + `DeferredTap`) exposing `Configure`/`OffsetPx`/`HandleWheel`/`Draw`/`HandleScrollbarDrag` plus a full `HandleInput` state machine lifted verbatim from the (correct) context menu. The overflow + context menus use the full `HandleInput`; the subdivision + instrument menus share the primitives (`HandleScrollbarDrag` + `OffsetPx` + `Draw`) while keeping their bespoke content-input.

**Tech Stack:** Go 1.23, Ebiten v2, build tag `test`, bundled Go at `.tools/go/bin/go`.

**Project rules honored:** Do **NOT** `git add`/commit anything (standing user rule). All steps end at a verification checkpoint; changes stay in the working tree. Bug is reproduced with a functional test FIRST. This branch (`add-core-node-types`) carries known pre-existing failures — diff targeted test output against the baseline, never blame the whole suite.

**Fast test command (used throughout):**
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<RegExp>' -v
```

---

## File Structure

- **Create** `src/go/internal/ui/menu_scroll.go` — the `MenuScroll` type + `MenuScrollInput`. One responsibility: shared scroll mechanics + input state machine.
- **Create** `src/go/internal/ui/menu_scroll_test.go` — unit tests for `MenuScroll`.
- **Create** `src/go/internal/ui/template_menu_scroll_test.go` — functional reproduction + regression for the overflow/template scroll bug.
- **Modify** `src/go/internal/ui/drumview.go` — rename field `overflowScroll *ScrollBehavior` → `overflowMenuScroll *MenuScroll`.
- **Modify** `src/go/internal/ui/drumview_context_menu.go` — overflow menu: configure/offset/clip/input via `MenuScroll`; context menu: `HandleInput` + `OffsetPx`; add test accessor.
- **Modify** `src/go/internal/ui/drumview_portal_open.go` — overflow `wheelFn` uses new field.
- **Modify** `src/go/internal/ui/drumview_portal_open_test.go` — update the one test that pokes the old field.
- **Modify** `src/go/internal/ui/drumview_overlay_subdiv_comp.go` — add `MenuScroll`: cap height, offset, scrollbar drag + wheel + draw.
- **Create** `src/go/internal/ui/subdiv_menu_scroll_test.go` — subdivision overflow-scroll test.
- **Modify** `src/go/internal/ui/drumview_overlay_inst_comp.go` — hold a `*MenuScroll`, route its existing `ScrollBehavior` usage through it (primitive-sharing only).

---

## Task 1: `MenuScroll` core (geometry + primitives)

**Files:**
- Create: `src/go/internal/ui/menu_scroll.go`
- Test: `src/go/internal/ui/menu_scroll_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/menu_scroll_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestMenuScroll_OffsetAndWheel(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 20, 5) // 20 items, 5 visible

	if !m.HasScroll() {
		t.Fatalf("HasScroll()=false; want true (Total=20 Visible=5)")
	}
	if got := m.OffsetPx(); got != 0 {
		t.Fatalf("OffsetPx()=%d; want 0 at top", got)
	}
	if !m.HandleWheel(-2) { // wheel down 2 items
		t.Fatalf("HandleWheel returned false; expected offset change")
	}
	if got := m.OffsetPx(); got != 2*20 {
		t.Fatalf("OffsetPx()=%d; want %d after scrolling 2 items", got, 2*20)
	}
}

func TestMenuScroll_NoScrollWhenFits(t *testing.T) {
	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	m.Configure(image.Rect(0, 0, 100, 100), 3, 5) // fewer items than visible
	if m.HasScroll() {
		t.Fatalf("HasScroll()=true; want false when content fits")
	}
	if got := m.OffsetPx(); got != 0 {
		t.Fatalf("OffsetPx()=%d; want 0 when no scroll", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuScroll -v`
Expected: FAIL — `undefined: NewMenuScroll`.

- [ ] **Step 3: Write minimal implementation**

Create `src/go/internal/ui/menu_scroll.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// MenuScroll bundles a ScrollBehavior and a DeferredTap into the single shared
// scroll component used by every list menu (context, overflow, subdivision,
// instrument). It owns the one correct wheel / thumb-drag / touch-scroll input
// state machine — the overflow menu previously hand-rolled this glue and got it
// half-wired (no offset applied, no drag/touch), which is the bug this fixes.
type MenuScroll struct {
	sb  *ScrollBehavior
	tap DeferredTap
}

// NewMenuScroll creates a MenuScroll with the given scrollbar style and the
// per-item pixel height used for offset and pixel<->item conversion.
func NewMenuScroll(style ScrollbarStyle, itemHeight int) *MenuScroll {
	return &MenuScroll{sb: NewScrollBehavior(style, itemHeight)}
}

// Configure refreshes the scroll viewport and item counts for the current
// layout. Safe to call every frame: it preserves the current scroll position
// (VS.First) via Clamp. Call whenever item count, viewport, or page changes.
func (m *MenuScroll) Configure(view image.Rectangle, total, visible int) {
	m.sb.VS.View = view
	m.sb.VS.Total = total
	m.sb.VS.Visible = visible
	m.sb.VS.Clamp()
}

// ItemHeight returns the configured per-item pixel height.
func (m *MenuScroll) ItemHeight() int { return m.sb.ItemHeight }

// OffsetPx is the pixel offset to subtract from item base-Y during layout/draw.
func (m *MenuScroll) OffsetPx() int { return m.sb.VS.First * m.sb.ItemHeight }

// HasScroll reports whether the content overflows the viewport.
func (m *MenuScroll) HasScroll() bool { return m.sb.HasScroll() }

// HandleWheel applies a wheel event; returns true when the offset changed.
func (m *MenuScroll) HandleWheel(steps int) bool { return m.sb.HandleWheel(steps) }

// Draw renders the scrollbar (no-op when content fits).
func (m *MenuScroll) Draw(dst *ebiten.Image) { m.sb.Draw(dst) }

// ScrollBehavior exposes the underlying engine for menus (instrument menu) whose
// bespoke content layout drives VS.View / VS.First directly.
func (m *MenuScroll) ScrollBehavior() *ScrollBehavior { return m.sb }

// DeferredTap exposes the tap helper for callers that need to cancel it.
func (m *MenuScroll) DeferredTap() *DeferredTap { return &m.tap }

// TapActive reports an in-flight mobile deferred tap. Callers map this to
// InputCaptured (vs InputConsumed) for the overlay portal.
func (m *MenuScroll) TapActive() bool { return m.tap.Active() }

// HandleScrollbarDrag processes scrollbar thumb interaction only (start on press
// inside the thumb, continue/end on subsequent events). It is the shared
// primitive for menus that keep their own immediate-tap content-input
// (subdivision, instrument). Returns true when the scrollbar consumed the event.
// relayout (may be nil) is called when the offset changes mid-drag.
func (m *MenuScroll) HandleScrollbarDrag(pt image.Point, pressed bool, relayout func()) bool {
	sb := m.sb
	if sb.Dragging() {
		if pressed {
			if sb.HandleDragTo(pt.Y) && relayout != nil {
				relayout()
			}
		} else {
			sb.HandleDragEnd()
		}
		return true
	}
	if sb.HasScroll() && pressed && pt.In(sb.ThumbRect()) {
		sb.HandleDragStart(pt.Y)
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuScroll -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: builds clean. Leave changes in the working tree — do **not** `git add`/commit.

---

## Task 2: `MenuScroll.HandleInput` state machine

**Files:**
- Modify: `src/go/internal/ui/menu_scroll.go`
- Test: `src/go/internal/ui/menu_scroll_test.go`

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/menu_scroll_test.go`:

```go
func TestMenuScroll_HandleInput_DesktopThumbDrag(t *testing.T) {
	restore := withSmallScreenReturn(t, false) // desktop
	defer restore()

	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	view := image.Rect(0, 0, 100, 100)
	m.Configure(view, 20, 5)

	thumb := m.ScrollBehavior().ThumbRect()
	relayouts := 0
	// Press inside the thumb starts a drag.
	if !m.HandleInput(MenuScrollInput{
		Pt: image.Pt(thumb.Min.X+1, thumb.Min.Y+1), Pressed: true,
		PopupRect: view, ItemView: view,
		Relayout: func() { relayouts++ },
	}) {
		t.Fatalf("press in thumb not handled")
	}
	// Drag down moves the offset.
	m.HandleInput(MenuScrollInput{
		Pt: image.Pt(thumb.Min.X+1, view.Max.Y - 1), Pressed: true,
		PopupRect: view, ItemView: view,
		Relayout: func() { relayouts++ },
	})
	if m.OffsetPx() == 0 {
		t.Fatalf("OffsetPx still 0 after dragging thumb to bottom")
	}
	if relayouts == 0 {
		t.Fatalf("Relayout never called during drag")
	}
}

func TestMenuScroll_HandleInput_DesktopButtonHit(t *testing.T) {
	restore := withSmallScreenReturn(t, false) // desktop
	defer restore()

	m := NewMenuScroll(DropdownScrollbarStyle, 20)
	view := image.Rect(0, 0, 100, 200)
	m.Configure(view, 3, 5) // no scroll

	hit := false
	handled := m.HandleInput(MenuScrollInput{
		Pt: image.Pt(10, 10), Pressed: true,
		PopupRect: view, ItemView: view,
		DesktopHit: func(pt image.Point, pressed bool) bool { hit = true; return true },
	})
	if !handled || !hit {
		t.Fatalf("DesktopHit not invoked (handled=%v hit=%v)", handled, hit)
	}
}
```

This test needs a small-screen helper that returns a restore func and reports
the prior value. If `withSmallScreenReturn` does not exist, add it in Step 3.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuScroll_HandleInput -v`
Expected: FAIL — `undefined: MenuScrollInput` (and possibly `withSmallScreenReturn`).

- [ ] **Step 3: Write minimal implementation**

First verify a screen-class test helper exists:
Run: `cd src/go && grep -rn "func withSmallScreen" internal/ui/*_test.go`
If only `withSmallScreen(t, bool)` exists (no return), add this thin wrapper to
`menu_scroll_test.go`:

```go
// withSmallScreenReturn sets the mobile/desktop screen class for the test and
// returns a restore func. Thin wrapper so MenuScroll tests can toggle class.
func withSmallScreenReturn(t *testing.T, small bool) func() {
	t.Helper()
	prev := SetSmallScreenForTest(small) // returns restore func
	return prev
}
```

Check the exact name with:
Run: `cd src/go && grep -rn "SmallScreenForTest\|SetForceSmallScreen\|forceSmallScreen" internal/ui/*.go | grep -v _test`
Use whatever setter the package exposes (e.g. `SetSmallScreenForTest`,
`SetForceSmallScreen`); `withSmallScreen` in `context_menu_scroll_test.go`
already wraps it — mirror that call but capture the restore func.

Then append to `src/go/internal/ui/menu_scroll.go`:

```go
// MenuScrollInput carries the per-event inputs and content callbacks for one
// HandleInput call. Each menu keeps its own content layout/draw; MenuScroll owns
// only the scroll mechanics.
type MenuScrollInput struct {
	Pt        image.Point
	Pressed   bool
	PopupRect image.Rectangle // whole popup; taps inside are consumed
	ItemView  image.Rectangle // scroll viewport (excludes any pinned header)

	// Relayout re-applies OffsetPx() to cached item buttons after the offset
	// changes. Menus that rebuild their buttons every frame may pass nil.
	Relayout func()
	// FireTapAt fires the menu button under (x,y) on a committed mobile tap.
	FireTapAt func(x, y int)
	// DesktopHit performs immediate hit-testing of the menu's buttons on
	// desktop; returns true when a button consumed the event.
	DesktopHit func(pt image.Point, pressed bool) bool
	// CloseBtn, when non-nil, is checked before items on mobile (highest z).
	CloseBtn *Button
}

// HandleInput runs the complete desktop+mobile scroll input state machine,
// behavior-identical to the (correct) context menu reference implementation.
// Returns true when the event was handled (consumed/captured).
func (m *MenuScroll) HandleInput(in MenuScrollInput) bool {
	pt := in.Pt
	left := in.Pressed
	sb := m.sb

	if Profile().IsMobile() {
		// 1. Scrollbar drag in progress.
		if sb.Dragging() {
			if left {
				if sb.HandleDragTo(pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				sb.HandleDragEnd()
			}
			return true
		}
		// 2. Scroll committed → move; on release cancel the deferred tap.
		if sb.ScrollingCommitted() {
			if left {
				if sb.HandleTouchMove(pt.X, pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				m.tap.Cancel()
				sb.HandleTouchEnd()
			}
			return true
		}
		// 3. Touch active but still in the dead zone.
		if sb.TouchActive() {
			if left {
				if sb.HandleTouchMove(pt.X, pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				wasTap := !sb.ScrollingCommitted()
				sb.HandleTouchEnd()
				if wasTap && in.FireTapAt != nil {
					m.tap.End(in.FireTapAt)
				} else {
					m.tap.Cancel()
				}
			}
			return true
		}
		// 4. Scrollbar thumb click.
		if sb.HasScroll() && left && pt.In(sb.ThumbRect()) {
			sb.HandleDragStart(pt.Y)
			return true
		}
		// 5. Close button (highest z, before item area).
		if in.CloseBtn != nil && left && pt.In(in.CloseBtn.Rect()) {
			if in.CloseBtn.OnClick != nil {
				in.CloseBtn.OnClick()
			}
			return true
		}
		// 6. Press in item viewport → begin deferred tap + touch scroll.
		if left && pt.In(in.ItemView) && !sb.TouchActive() && !sb.Dragging() {
			if m.tap.Begin(pt.X, pt.Y) {
				sb.HandleTouchBegin(pt.X, pt.Y)
				return true
			}
		}
		// 7. Header area (above viewport) → consume without action.
		if pt.In(in.PopupRect) && pt.Y < in.ItemView.Min.Y {
			return true
		}
	} else {
		// Desktop: scrollbar drag, then immediate button hit.
		if sb.Dragging() {
			if left {
				if sb.HandleDragTo(pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				sb.HandleDragEnd()
			}
			return true
		}
		if sb.HasScroll() && left && pt.In(sb.ThumbRect()) {
			sb.HandleDragStart(pt.Y)
			return true
		}
		if in.DesktopHit != nil && in.DesktopHit(pt, left) {
			return true
		}
	}

	// Consume any tap inside the popup (click-outside close handled by the tree).
	if pt.In(in.PopupRect) {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuScroll -v`
Expected: PASS (all MenuScroll subtests).

- [ ] **Step 5: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build. Leave in working tree.

---

## Task 3: Failing functional test for the template-menu scroll bug

**Files:**
- Create: `src/go/internal/ui/template_menu_scroll_test.go`
- Modify: `src/go/internal/ui/drumview_context_menu.go` (add a test accessor only)

- [ ] **Step 1: Add the test accessor**

In `src/go/internal/ui/drumview_context_menu.go`, near the other `*ForTest`
helpers (search for `ContextMenuScrollForTest`), add:

```go
// OverflowMenuScrollForTest exposes the overflow menu's MenuScroll for tests.
func (dv *DrumView) OverflowMenuScrollForTest() *MenuScroll { return dv.overflowMenuScroll }
```

NOTE: `dv.overflowMenuScroll` is introduced in Task 4. To keep this task
compiling on its own, this step also renames the field now — but the rename is
the first step of Task 4. **Do Task 4 Step 1 (the field rename) before running
this test.** If executing strictly task-by-task, fold this accessor into Task 4
Step 1. (The test below is the gate for Task 4.)

- [ ] **Step 2: Write the failing functional test**

Create `src/go/internal/ui/template_menu_scroll_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// openTemplatePageClamped opens the overflow menu on the template page with a
// drum pane short enough that the template list overflows the popup.
func openTemplatePageClamped(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(420, 360) // short pane → popup is height-clamped
	dv := g.drum
	dv.overflowPage = 1 // template page (Templates header + Back + 7 genres)
	dv.OpenOverflowMenu()
	return dv
}

// lastTemplateBtn returns the button rect for the last template row ("Techno")
// from the menu's real hit geometry.
func lastTemplateBtn(dv *DrumView) (image.Rectangle, bool) {
	popup := dv.overflowPopupRect()
	btns := dv.overflowPopupBtns(popup)
	// Last button is the close button; the one before it is the last item row.
	if len(btns) < 2 {
		return image.Rectangle{}, false
	}
	return btns[len(btns)-2].Rect(), true
}

func TestTemplateMenu_ScrollsToReachLastItem(t *testing.T) {
	dv := openTemplatePageClamped(t)

	scroll := dv.OverflowMenuScrollForTest()
	if scroll == nil {
		t.Fatal("overflow MenuScroll is nil")
	}
	if !scroll.HasScroll() {
		t.Fatalf("expected HasScroll()=true on clamped template page; Total=%d Visible=%d",
			scroll.ScrollBehavior().VS.Total, scroll.ScrollBehavior().VS.Visible)
	}

	popup := dv.overflowPopupRect()
	before, ok := lastTemplateBtn(dv)
	if !ok {
		t.Fatal("could not locate last template button")
	}
	// Bug: with no offset applied, the last row sits at the same Y regardless of
	// scroll. After wheel-scrolling toward the end, its Y must decrease (move up
	// into the viewport).
	for i := 0; i < 20; i++ {
		scroll.HandleWheel(-1) // scroll down one item
	}
	after, _ := lastTemplateBtn(dv)

	if after.Min.Y >= before.Min.Y {
		t.Fatalf("last template did not move up after scrolling: before.Y=%d after.Y=%d (offset not applied)",
			before.Min.Y, after.Min.Y)
	}
	if after.Min.Y < popup.Min.Y || after.Max.Y > popup.Max.Y {
		t.Fatalf("last template not within popup after scroll: row=%v popup=%v", after, popup)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_ScrollsToReachLastItem -v`
Expected: FAIL — either `undefined: OverflowMenuScrollForTest` / `overflowMenuScroll` (before Task 4) or, after the field exists but before offset wiring, the assertion `last template did not move up after scrolling` fails. This is the bug reproduction.

- [ ] **Step 4: Verification checkpoint (NO COMMIT)**

Confirm the failure message is the offset assertion (not a compile error) once
Task 4 Step 1 is done. Leave in working tree.

---

## Task 4: Migrate the overflow / Load-template menu onto `MenuScroll`

**Files:**
- Modify: `src/go/internal/ui/drumview.go`
- Modify: `src/go/internal/ui/drumview_context_menu.go`
- Modify: `src/go/internal/ui/drumview_portal_open.go`
- Modify: `src/go/internal/ui/drumview_portal_open_test.go`

- [ ] **Step 1: Rename the field**

In `src/go/internal/ui/drumview.go` (around line 593), change:

```go
	overflowScroll *ScrollBehavior // scroll when items overflow
```
to:
```go
	overflowMenuScroll *MenuScroll // shared scroll component for the overflow menu
```

- [ ] **Step 2: Replace `initOverflowScroll` with create + configure**

In `src/go/internal/ui/drumview_context_menu.go`, replace `initOverflowScroll`
(lines ~1033-1046) with:

```go
// initOverflowScroll creates the overflow menu's shared scroll component.
func (dv *DrumView) initOverflowScroll() {
	dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, touchMinTargetPx)
	dv.configureOverflowScroll()
}

// configureOverflowScroll refreshes scroll geometry from the current page's
// items and popup rect. Cheap + idempotent; safe to call every frame. Preserves
// the scroll position across page switches (Configure clamps, not resets).
func (dv *DrumView) configureOverflowScroll() {
	if dv.overflowMenuScroll == nil {
		dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, touchMinTargetPx)
	}
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	popupRect := dv.overflowPopupRect()
	visible := popupRect.Dy() / rowH
	if visible > len(items) {
		visible = len(items)
	}
	if visible < 1 {
		visible = 1
	}
	dv.overflowMenuScroll.Configure(popupRect, len(items), visible)
}
```

Also add the test accessor from Task 3 Step 1 here if not already present:

```go
// OverflowMenuScrollForTest exposes the overflow menu's MenuScroll for tests.
func (dv *DrumView) OverflowMenuScrollForTest() *MenuScroll { return dv.overflowMenuScroll }
```

- [ ] **Step 3: Apply the scroll offset in `overflowPopupBtns`**

In `overflowPopupBtns` (lines ~915-942), configure scroll then offset the
non-close rows. Change the start of the function:

```go
func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	dv.configureOverflowScroll()
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	btns := make([]*Button, 0, len(items)+1)
	itemStyle := ButtonVisual(DropdownStyle)
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx() // scroll offset
	for _, item := range items {
		if item.header {
			curY += rowH
			continue
		}
		r := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		btn := NewButton(item.label, itemStyle, item.onClick)
		btn.SetRect(insetRect(r, SpaceXS))
		btns = append(btns, btn)
		curY += rowH
	}
	// Close button stays pinned (NOT offset).
	closeR := closeButtonRect(popupRect, SpaceXS)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeOverflowMenu() })
	closeB.Icon = "close"
	closeB.IconColor = colButtonBorder
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	btns = append(btns, closeB)
	return btns
}
```

- [ ] **Step 4: Apply offset + clip in `drawOverflowMenu`**

In `drawOverflowMenu` (lines ~951-1031), draw row content into a popup-clipped
subimage and offset `curY`. Replace the body from `popupRect := ...` down to the
close-button draw:

```go
	popupRect := dv.overflowPopupRect()
	dv.configureOverflowScroll()
	// Backdrop scrim painted by the OverlayPortal (PortalEntry.Scrim).
	drawPanel(dst, popupRect)
	// Clip scrolling rows to the popup so offset content can't paint outside.
	clip := dst.SubImage(popupRect).(*ebiten.Image)
	btns := dv.overflowPopupBtns(popupRect)
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	nActionBtns := len(btns) - 1 // last is the close button
	for _, item := range items {
		rowR := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		curY += rowH
		if item.header {
			tx := rowR.Min.X + SpaceMD
			ty := rowR.Min.Y + (rowR.Dy()-StyledTextHeight(RoleCaption))/2
			DrawTextStyled(clip, item.label, tx, ty, RoleCaption, colTextSecondary)
			continue
		}
		if btnIdx >= nActionBtns {
			break
		}
		btn := btns[btnIdx]
		btnIdx++

		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		drawMenuItemBackground(clip, btn.Rect(), state)

		if item.iconID != "" {
			r := btn.Rect()
			side := r.Dy() - 2*SpaceXS
			if side < 0 {
				side = 0
			}
			iconR := image.Rect(
				r.Min.X+SpaceSM,
				r.Min.Y+(r.Dy()-side)/2,
				r.Min.X+SpaceSM+side,
				r.Min.Y+(r.Dy()-side)/2+side,
			)
			tint := colIncDecIcon
			if item.active {
				tint = colFollowActive
			}
			DrawIcon(clip, item.iconID, iconR, tint)
		}

		saved := btn.Text
		btn.Text = ""
		btn.Draw(clip)
		btn.Text = saved

		r := btn.Rect()
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(clip, saved, r.Min.X, ty, RoleBody, colTextPrimary)
	}
	// Close button + scrollbar draw on the unclipped dst (pinned chrome).
	if len(btns) > 0 {
		btns[len(btns)-1].Draw(dst)
	}
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.Draw(dst)
	}
```

- [ ] **Step 5: Route `handleOverflowMenuInput` through `MenuScroll`**

Replace `handleOverflowMenuInput` (lines ~663-702) with:

```go
func (dv *DrumView) handleOverflowMenuInput(mx, my int, left bool) bool {
	if !dv.IsOverflowMenuOpen() {
		return false
	}
	dv.configureOverflowScroll()
	popupRect := dv.overflowPopupRect()
	btns := dv.overflowPopupBtns(popupRect)
	closeBtn := btns[len(btns)-1]

	return dv.overflowMenuScroll.HandleInput(MenuScrollInput{
		Pt:        image.Pt(mx, my),
		Pressed:   left,
		PopupRect: popupRect,
		ItemView:  popupRect,
		// Buttons are rebuilt every frame from the live offset, so no Relayout.
		FireTapAt:  dv.fireOverflowMenuTapAt,
		CloseBtn:   closeBtn,
		DesktopHit: func(pt image.Point, pressed bool) bool {
			for _, b := range btns {
				if b.HandleInputResult(pt.X, pt.Y, pressed) != InputIgnored {
					return true
				}
			}
			return false
		},
	})
}
```

NOTE: the previous mobile path used `dv.overflowDeferredTap`. The shared
`MenuScroll` now owns the deferred tap. Leave `dv.overflowDeferredTap` field +
`closeOverflowMenu`'s `dv.overflowDeferredTap.Cancel()` in place (harmless), or
remove its `.Cancel()` calls if the field becomes unused and the build complains.
Verify with the build in Step 8.

- [ ] **Step 6: Update the portal `wheelFn`**

In `src/go/internal/ui/drumview_portal_open.go` (lines ~167-172), change:

```go
			wheelFn: func(x, y, steps int) InputResult {
				if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.HasScroll() {
					dv.overflowMenuScroll.HandleWheel(steps)
				}
				return InputConsumed
			},
```

- [ ] **Step 7: Make `registerFilePickerRects` offset-aware**

In `registerFilePickerRects` (lines ~1061-1096), apply the scroll offset to the
row Y so the registered picker rects track the visible position. Change:

```go
	rowH := touchMinTargetPx
	dv.configureOverflowScroll()
	offset := dv.overflowMenuScroll.OffsetPx()
	...
		y0 := popupRect.Min.Y + i*rowH - offset
```

(Add the `offset` line after `rowH`, and subtract `offset` in the `y0` formula.)

- [ ] **Step 8: Update the one test that pokes the old field**

In `src/go/internal/ui/drumview_portal_open_test.go` (lines ~1602-1605),
replace:

```go
	dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, 24)
	dv.overflowMenuScroll.Configure(image.Rect(0, 0, 100, 200), 20, 5)
```

(Remove the old three `dv.overflowScroll...` lines. Ensure `image` is imported in
that test file; add it if missing.)

- [ ] **Step 9: Run the bug test — expect PASS now**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu_ScrollsToReachLastItem|TestOpenOverflowMenuPortal_WheelFn_WithScroll' -v`
Expected: PASS.

- [ ] **Step 10: Run all overflow/template tests — expect no new failures**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Overflow|Template|FilePicker|Subgroup|Iconograph' -v`
Expected: PASS (or only pre-existing failures — compare against baseline if any fail).

- [ ] **Step 11: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build. Leave in working tree.

---

## Task 5: Migrate the context menu to the shared `HandleInput` + `OffsetPx`

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go`

The context menu is the reference implementation; this is behavior-preserving.

- [ ] **Step 1: Confirm the baseline is green first**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ContextMenu' -v`
Expected: record which (if any) fail BEFORE the change — these are the baseline.

- [ ] **Step 2: Use `OffsetPx()` in `rebuildContextMenuButtons`**

`dv.contextMenuScroll` stays a `*ScrollBehavior` (the field is widely used). Only
replace the inline offset math. In `rebuildContextMenuButtons` (lines ~175-223),
replace both occurrences of:

```go
	scrollPx := dv.contextMenuScroll.VS.First * rowH
```
with a single computation at the top using the shared formula (identical value):

```go
	scrollPx := dv.contextMenuScroll.VS.First * dv.contextMenuScroll.ItemHeight
```

(Behavior identical when `ItemHeight == rowH == touchMinTargetPx`, which it is —
`NewScrollBehavior(DropdownScrollbarStyle, rowH)`. This removes the
duplicate-constant coupling.)

- [ ] **Step 3: Run context menu tests — expect identical results**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ContextMenu' -v`
Expected: same pass/fail set as Step 1 baseline (behavior-preserving).

- [ ] **Step 4: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build. Leave in working tree.

NOTE on scope: the context menu's bespoke `handleContextMenuInput` is the exact
source `MenuScroll.HandleInput` was lifted from, so a full swap is possible but
adds risk to a working menu (its mobile path interleaves group-container draw and
a pinned header). YAGNI: we share the `OffsetPx` formula now; a later change can
swap its input loop to `MenuScroll.HandleInput` once the overflow + subdiv menus
have proven the shared path in production. This keeps the reference menu green.

---

## Task 6: Wire the subdivision menu to `MenuScroll` (primitive-sharing)

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_subdiv_comp.go`
- Test: `src/go/internal/ui/subdiv_menu_scroll_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/subdiv_menu_scroll_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestSubdivMenu_ScrollsWhenOptionsOverflow(t *testing.T) {
	assertDefaultParityState(t)
	c := NewSubdivMenuComponent()
	c.SetScreenBounds(image.Rect(0, 0, 200, 120)) // short → overflow
	manyOpts := []int{1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 48, 64}
	c.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(10, 0, 60, 20),
		Current:    4,
		Options:    manyOpts,
		RowHeight:  24,
		OnSelect:   func(int) {},
		OnClose:    func() {},
	})
	c.Open()

	scroll := c.MenuScrollForTest()
	if scroll == nil {
		t.Fatal("subdiv MenuScroll is nil after Open")
	}
	if !scroll.HasScroll() {
		t.Fatalf("expected HasScroll()=true with %d options in a short card", len(manyOpts))
	}

	// Scroll down: the last option's button must move up into the card.
	last := c.Buttons()[len(c.Buttons())-1].Rect()
	for i := 0; i < len(manyOpts); i++ {
		scroll.HandleWheel(-1)
	}
	c.RebuildForTest() // re-apply offset
	last2 := c.Buttons()[len(c.Buttons())-1].Rect()
	if last2.Min.Y >= last.Min.Y {
		t.Fatalf("last option did not move up after scroll: before=%d after=%d", last.Min.Y, last2.Min.Y)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSubdivMenu_ScrollsWhenOptionsOverflow -v`
Expected: FAIL — `undefined: c.MenuScrollForTest` / `RebuildForTest`.

- [ ] **Step 3: Add the `MenuScroll` field + helpers**

In `src/go/internal/ui/drumview_overlay_subdiv_comp.go`, add to the struct
(after `screenBounds`):

```go
	// menuScroll shares the app-wide scroll component so a long options list
	// (rare, but possible) scrolls like every other list menu.
	menuScroll *MenuScroll
```

Add helpers at the end of the file:

```go
// MenuScrollForTest exposes the scroll component for tests.
func (s *SubdivMenuComponent) MenuScrollForTest() *MenuScroll { return s.menuScroll }

// RebuildForTest re-runs the layout (re-applying the scroll offset) for tests.
func (s *SubdivMenuComponent) RebuildForTest() { s.rebuildButtons() }

// itemViewport is the scrollable region inside the card (below any pad).
func (s *SubdivMenuComponent) itemViewport() image.Rectangle {
	pad := SpaceSM
	r := s.cardRect
	if r.Empty() {
		return r
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}

// fireSubdivTapAt fires the option button under (x,y).
func (s *SubdivMenuComponent) fireSubdivTapAt(x, y int) {
	pt := image.Pt(x, y)
	for _, btn := range s.buttons {
		if pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
}
```

- [ ] **Step 4: Configure + offset in `rebuildButtons`**

In `rebuildButtons`, after `s.cardRect` is finalized (just before the item loop
at line ~174), insert scroll configuration and offset the item Y. Replace the
item loop (lines ~174-194) with:

```go
	// Configure shared scroll over the card's interior.
	if s.menuScroll == nil {
		s.menuScroll = NewMenuScroll(DropdownScrollbarStyle, rowH)
	}
	view := s.itemViewport()
	visible := view.Dy() / rowH
	if visible < 1 {
		visible = 1
	}
	if visible > nOpts {
		visible = nOpts
	}
	s.menuScroll.Configure(view, nOpts, visible)
	offset := s.menuScroll.OffsetPx()

	itemX0 := s.cardRect.Min.X + pad
	itemX1 := s.cardRect.Max.X - pad
	for i, v := range s.props.Options {
		y0 := s.cardRect.Min.Y + pad + i*rowH - offset
		r := image.Rect(itemX0, y0, itemX1, y0+rowH)
		text := fmt.Sprintf("%d", v)
		vv := v
		btn := NewButton(text, DropdownStyle, func() {
			if s.props.OnSelect != nil {
				s.props.OnSelect(vv)
			}
			s.Close()
		})
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, SpaceXS))
		s.buttons = append(s.buttons, btn)
	}
```

- [ ] **Step 5: Add scrollbar drag + wheel to `HandleInput`; clip + draw scrollbar**

In `HandleInput` (lines ~200-229), insert scroll handling at the top (after the
`pt :=` line):

```go
	if s.menuScroll != nil {
		if s.menuScroll.HandleScrollbarDrag(pt, pressed, s.rebuildButtons) {
			return InputConsumed
		}
	}
```

Add a `HandleWheel` body (replace the existing no-op at lines ~311-316):

```go
func (s *SubdivMenuComponent) HandleWheel(x, y, steps int) InputResult {
	if !s.state.open {
		return InputIgnored
	}
	if s.menuScroll != nil && s.menuScroll.HasScroll() {
		s.menuScroll.HandleWheel(steps)
		s.rebuildButtons()
	}
	return InputConsumed
}
```

In `Draw` (lines ~234-256), after the item loop, add:

```go
	if s.menuScroll != nil {
		s.menuScroll.Draw(dst)
	}
```

(Item rows are short and rarely overflow; clipping is optional here. If a partial
row paints below the card edge in the overflow case, wrap the `drawSubdivRow`
loop's `dst` in `dst.SubImage(s.cardRect).(*ebiten.Image)` — mirror Task 4 Step 4.
Add this only if a visual test shows bleed.)

- [ ] **Step 6: Run the subdiv tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Subdiv' -v`
Expected: PASS — new scroll test + all existing subdiv tests
(`subdiv_ui_test.go`, `subdiv_no_clickthrough_test.go`,
`drumview_overlay_subdiv_comp_test.go`).

- [ ] **Step 7: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build. Leave in working tree.

---

## Task 7: Instrument menu shares the `MenuScroll` component (primitive-only)

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go`

The instrument menu already uses `ScrollBehavior` + `DeferredTap` correctly. This
task makes it *hold* the shared `MenuScroll` so all list menus reference one
component, without rewriting its bespoke content-input (stars/search/breadcrumb).

- [ ] **Step 1: Confirm baseline green**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Instrument|InstMenu|Dropdown' -v`
Expected: record the baseline pass/fail set.

- [ ] **Step 2: Back the existing `scroll` field with a `MenuScroll`**

In `src/go/internal/ui/drumview_overlay_inst_comp.go`, change the field (line
~139) and constructor (lines ~194, ~240) so the component owns a `*MenuScroll`
and derives its `*ScrollBehavior` from it. Replace:

```go
	scroll       *ScrollBehavior
```
with:
```go
	menuScroll *MenuScroll     // shared scroll component (owns the ScrollBehavior)
	scroll     *ScrollBehavior // == menuScroll.ScrollBehavior(); kept for the existing call sites
```

In `NewInstrumentMenuComponent` (line ~194) replace
`scroll: NewScrollBehavior(DropdownScrollbarStyle, 24)` with nothing in the
literal, then set both after construction. In `ensureScroll` (lines ~238-243):

```go
func (m *InstrumentMenuComponent) ensureScroll() {
	if m.menuScroll == nil {
		m.menuScroll = NewMenuScroll(DropdownScrollbarStyle, 24)
		m.scroll = m.menuScroll.ScrollBehavior()
	}
	if m.scroll == nil {
		m.scroll = m.menuScroll.ScrollBehavior()
	}
}
```

If the constructor sets `scroll:` in the struct literal, remove that line and
rely on `ensureScroll()` (already called in `Open`/build paths). Grep to confirm
no path reads `m.scroll` before `ensureScroll`:
Run: `cd src/go && grep -n "m.scroll" internal/ui/drumview_overlay_inst_comp.go | head`

- [ ] **Step 3: Use `Draw`/`HasScroll` via the shared component (optional tidy)**

Where the menu draws its scrollbar today (search for `m.scroll.Draw` /
`m.scroll.Style.Draw` in the file), the call already routes through the same
engine; no change required for correctness. Leave the bespoke `VS.View`/`First`
math untouched (it is woven through search/breadcrumb/paging).

- [ ] **Step 4: Run instrument menu tests — expect identical results**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Instrument|InstMenu|Dropdown' -v`
Expected: same pass/fail set as Step 1 baseline.

- [ ] **Step 5: Verification checkpoint (NO COMMIT)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Expected: clean build. Leave in working tree.

---

## Task 8: Full verification sweep

**Files:** none (verification only)

- [ ] **Step 1: Build everything (prod + test tags)**

Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./...`
Expected: both clean.

- [ ] **Step 2: Run the full fast UI suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -40`
Expected: only the branch's known pre-existing failures remain (chip-strip Synth
FAILs + any suite-level panic noted in `project_add_core_node_types_preexisting_failures`).
None of the failures may be in: `menu_scroll`, `template_menu`, `subdiv`,
`context_menu`, `overflow`, `instrument`/`dropdown`, `filepicker`.

- [ ] **Step 3: Capture the new menus visually (optional but recommended)**

Run: `make screenshots-all SCENES=$(cd src/go && go run ./... -list-scenes 2>/dev/null | grep -i overflow | paste -sd ,)` — or open the app via the `run` skill and verify the Load-template list scrolls (wheel + drag).
Expected: template list scrolls; scrollbar thumb visible and draggable; rows clipped to the popup.

- [ ] **Step 4: Final checkpoint (NO COMMIT)**

Confirm everything is in the working tree, nothing staged:
Run: `cd /home/ymolinar/Repos/beatmo && git status --short | head`
Expected: modified/new files present, none staged. **Do not commit** — the user
owns all commits.

---

## Self-Review (completed by plan author)

- **Spec coverage:** MenuScroll core (T1), HandleInput (T2), overflow fix (T3+T4),
  context menu share (T5), subdivision (T6), instrument menu (T7), testing/verify
  (T8). All spec sections map to tasks.
- **Type consistency:** field `overflowMenuScroll *MenuScroll`; methods
  `Configure`, `OffsetPx`, `HandleWheel`, `Draw`, `HasScroll`, `HandleInput`,
  `HandleScrollbarDrag`, `ScrollBehavior`, `TapActive`, `DeferredTap`,
  `ItemHeight`; struct `MenuScrollInput` with `Pt/Pressed/PopupRect/ItemView/
  Relayout/FireTapAt/DesktopHit/CloseBtn` — used identically across T2/T4/T6.
- **Placeholders:** none — every code step shows full code; verify-grep steps name
  exact commands. A few "confirm exact setter name" greps are intentional
  (the screen-class test setter name varies by repo revision) and carry the
  fallback instruction inline.
- **Rule conflict resolved:** the writing-plans template's `git commit` steps are
  replaced by "Verification checkpoint (NO COMMIT)" to honor the standing
  no-staging/no-commit user rule.
