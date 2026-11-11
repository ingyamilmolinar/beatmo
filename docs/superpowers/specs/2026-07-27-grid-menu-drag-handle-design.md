# Grid-menu drag handle (grip) — design

**Date:** 2026-07-27
**Status:** approved, not yet implemented
**Scope:** the two grid-pane menus — the node pop-up (`NodeSidebar`) and the node-groups menu (`GroupMenu`).

## Problem

Both grid-pane menus are auto-placed: they anchor to their target (tapped node / group
badge), dodge obstacles (the top-right button cluster, group-member rects), and re-derive
that placement every frame. The placement is good by default but it is the *only* placement
available — a card that lands over the nodes the user wants to see cannot be moved, so the
user must close it, pan the grid, and reopen.

## Goal

Let the user move either card wherever they want, without changing where it appears by
default. A grip affordance in the card's top-left corner starts the move; the card is then
pinned where it is dropped.

## Non-goals

- The top-right menus (Settings / Logs / File) and the node long-press pop-up are **out of
  scope**. The first three are global-portal overlays on a different tree with a different
  input path; the last is a transient slide-release gesture whose model a drag would fight.
- No undo entry and no new `hooks.Kind` for the move — see "Telemetry" below.
- No change to either menu's default position, size, content, or dismissal behavior.

## Behavior

| State | Placement |
|---|---|
| **auto** (default, on every fresh open) | existing anchored placement, re-derived every frame; follows camera pan, dodges obstacles |
| **pinned** (after a grip drag) | fixed position in grid-pane space; does not follow the camera, the node, or group members |

Dragging the grip switches the card from auto to pinned. Closing the menu resets it to
auto, so the next open re-anchors beside its target. The pinned position is deliberately
**not** remembered across opens.

## Component: `PopupDragHandle`

New file `internal/ui/popup_drag_handle.go`. One embeddable value type owning all drag
state, so neither menu grows a bespoke state machine.

```go
type PopupDragHandle struct {
    pinned   bool        // user has dragged → auto-anchoring is off
    origin   image.Point // pinned top-left, grid-pane space
    dragging bool
    grab     image.Point // cursor − panel.Min at press, so the card doesn't jump
}
```

| Method | Role |
|---|---|
| `GripRect(panel) image.Rectangle` | the 16×16 glyph rect, inset `sidebarPad` from the card's top-left |
| `HitRect(panel) image.Rectangle` | `GripRect` grown to the density touch-min (~44px) on mobile; grip-sized on desktop |
| `Place(auto, pane image.Rectangle, size image.Point) image.Rectangle` | **the chokepoint.** Returns `auto` when not pinned, else `origin` re-clamped into `pane`. Called from each menu's layout, every frame |
| `Begin(pt image.Point, panel image.Rectangle)` | records `grab`, sets `dragging` |
| `MoveTo(pt image.Point, pane image.Rectangle, size image.Point)` | sets `pinned = true`, `origin = pt − grab`, clamped |
| `End()` | clears `dragging`, keeps `pinned` |
| `Dragging() bool` | for `Capturing()` and the `panOK` gate |
| `Reset()` | back to auto mode; called from each menu's `Open`/`Close` |

### Clamping rules

Applied in both `MoveTo` and `Place`, so a pinned card stays legal as it is dragged **and**
as it later changes size:

1. The card stays **fully inside the grid pane**. Non-negotiable: the node card renders
   through the `grid-sidebar` GridTree layer with `clip:true`, so anything outside the pane
   is scissored away regardless.
2. The **grip's hit rect never lands inside `gridTopButtonsReservedRect()`** (gear + file +
   logs). Escape guarantee: those buttons sit at z≈300 against the menus' z=200/210 and
   swallow presses in the overlap, so a card parked under them must still expose its grip
   to be dragged back out.

The rest of the card *may* overlap that cluster — the user put it there deliberately, and
rule 2 plus Esc and tap-away all remain available.

### Grip glyph

`IconGrip` joins the icon system: a const in `icons.go`, a `drawGripIcon`, and an entry in
the `drawIconByID` switch. A 2×3 dot matrix — the conventional drag affordance. Routing it
through `IconID` / `DrawIcon` rather than hand-drawn rects keeps it inside the design-token
discipline: no new `color.RGBA` literals, tint from
`WithAlpha(genColorBorder, genAlphaScrollbarThumb)`, brightening to `colTextSecondary` on
hover and `colAccent` while dragging.

## Integration

Both menus already re-run their layout every frame and re-derive an origin, so the
integration is a one-line diversion at the end of that derivation.

### `NodeSidebar` (`game_node_sidebar.go`)

`layout()` ends with `AnchorPopupRect` → `avoidReservedRect` → translate every rect by
`origin`. That block becomes the `auto` argument:

```go
anchored := AnchorPopupRect(gridPane, nodeR, w, cardH, PopupBelow)
if avoid := g.gridTopButtonsReservedRect(); !avoid.Empty() {
    anchored = avoidReservedRect(anchored, avoid, gridPane.Dx(), gridPane.Dy())
}
origin := sb.drag.Place(anchored, gridPane, image.Pt(w, cardH)).Min   // ← only new line
```

Header rects shift right by grip width + gap so the title clears the glyph; the close ×
and the coordinate line are otherwise untouched. `Open` and `Close` call `sb.drag.Reset()`.

### `GroupMenu` (`game_group_menu.go`)

Identical shape, with `placeGroupMenu(...)` as the `auto` value. When pinned, the
member-overlap and gear-avoidance passes are **skipped entirely** rather than
computed-then-overridden, so a dragged card cannot be yanked off its drop point by a member
moving underneath it. Its `title` rect shifts right by the same grip width + gap, so the
group-color swatch and name clear the glyph; the Save and close buttons keep their
right-aligned header slots. `openAt` and `Close` call `m.drag.Reset()`.

### Input

One branch at the top of each `HandleInput`, above everything else — including the
sidebar's scroll / deferred-tap ladder, which only handles presses in the content viewport
and therefore cannot race a grip that lives in the header band:

```go
if sb.drag.Dragging() {
    if pressed { sb.drag.MoveTo(pt, gridPane, size) } else { sb.drag.End() }
    return InputCaptured
}
if pressed && pt.In(sb.drag.HitRect(panelR)) {
    sb.drag.Begin(pt, panelR)
    return InputCaptured
}
```

`Capturing()` returns the drag state — new for `GroupMenu` (currently hardcoded `false`),
OR-ed with the existing scroll state for `NodeSidebar`. `InputCaptured` makes
`InputDispatcher` route every subsequent frame straight to the menu without re-hit-testing
(`input_dispatch.go:36-42`), so the drag survives the cursor leaving the card. No
dispatcher changes are needed, and the per-frame handler re-registration in
`game_update.go:413` reads `InputBounds()` live, so a moving panel needs no
`dispatcherDirty` poke.

## Edge cases

1. **Camera pan during drag.** `panOK` (`game_update.go:692`) gates panning on
   `!g.menuHit(mx, my)`, which is cursor-position based. Once a drag pulls the cursor off
   the card, panning would kick in and the grid would slide under the moving menu. Fix: add
   `&& !g.gridMenuDragging()` to `panOK`, a helper that ORs both menus' drag state.
2. **Card grows while pinned** (a section expands, a group gains a member). `Place`
   re-clamps the pinned origin every layout, so the card slides up/left just enough to stay
   inside the pane instead of spilling.
3. **Mobile single-finger drag** already maps to mouse through the touch override, so the
   grip drag needs no touch-specific code — it arrives as `pressed=true` with moving
   coordinates, exactly like desktop. The only mobile-specific piece is `HitRect`'s touch
   expansion.
4. **Shift+press** inside the node card closes it and yields the press to the connect /
   marquee gesture (existing behavior). The grip branch sits *below* that check, so
   shift+drag from a node under the card keeps working.

## Telemetry

None. The move is a view-only, non-undoable gesture. A new `hooks.Kind` would require a
`KindAll` entry, an `ActionRegistry` entry with a lowercase tag, an `eventlogger` formatter,
and a `NumNonVerbose` bump — for a line that fires on every nudge. The existing
`EventUITap` does not fire either, since it is emitted by `Button.applyPress` and the grip
is not a `Button`.

## Testing

**Functional Go tests** driving real `Game.Update` frames (project rule: functional over
isolation), in `popup_drag_handle_test.go` plus per-menu tests:

- press grip → move → release moves the panel by exactly the cursor delta, with no jump at
  press time (the `grab` offset);
- the pin survives ~30 subsequent frames *and* a camera pan;
- close + reopen restores the anchored default position;
- a press on the grip creates no node and starts no scroll;
- a drag past every pane edge leaves the card fully inside the pane;
- the grip can never come to rest inside `gridTopButtonsReservedRect()`;
- `GroupMenu.Capturing()` reports true mid-drag and false after release.

**Density test:** `HitRect` meets the 44px touch-min under the mobile profile, via
`SetDensityForTest`.

**Screenshot verification (mandatory, gating).** Two new scenes in `scene_catalog.go` —
`node_menu_dragged` and `group_menu_dragged` — each with a `MobileSetup`, driving the grip
drag through the real input path. Captured and inspected at desktop 1280×720 and mobile
390×844 portrait before the work is called done: grip glyph legible in the top-left, title
clear of it, card intact at the dropped position.

## Files touched

| File | Change |
|---|---|
| `internal/ui/popup_drag_handle.go` | **new** — the component |
| `internal/ui/icons.go` | `IconGrip` const, `drawGripIcon`, switch entry |
| `internal/ui/game_node_sidebar.go` | `drag` field, `Place` call, header shift, `Reset` in Open/Close, input branch, `Capturing` |
| `internal/ui/game_group_menu.go` | same, plus skip auto-placement when pinned |
| `internal/ui/game_update.go` | `panOK` gains `!g.gridMenuDragging()` |
| `internal/ui/game_input_drag.go` | `gridMenuDragging` helper |
| `internal/ui/scene_catalog.go` | two screenshot scenes |
| tests | `popup_drag_handle_test.go` + per-menu additions |
