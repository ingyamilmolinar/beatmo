# Unified Menu-Row Renderer

**Date:** 2026-06-27
**Status:** Approved (design)
**Scope:** `src/go/internal/ui/`

## Problem

The template list (the overflow menu's Templates page) does not share the
styling and animation of the main instrument menu. Today only the **instrument
menu** renders its rows through `btn.Draw()` — giving each row the full keycap
treatment (dark socket/wall shell, matte bevel, press-depth cap travel, hover
glow, `AdvancePressAnim` per frame). Every other list menu — the template /
overflow menu, the row context menu, and the subdivision selector — draws rows
*flat*: just the shared `drawMenuItemBackgroundAccent` background tint plus an
icon and label, with an explicit `// Flat: no keycap chrome` comment. They have
no press animation and no hover glow.

The template/overflow menu has a second problem: its buttons are recreated every
frame (`NewButton` inside `overflowPopupBtns`, called from both draw and input),
so even if it called `btn.Draw()` the animation state could never accumulate
across frames.

The goal: **unify menu/list component code** so styling and animation logic are
exactly the same across list menus, while each menu keeps its own *behavior*
(template = import-and-close; instrument = select-and-stay-open; subdiv =
pick-and-close; context = action-and-close).

## Current state (verified)

| Menu | File | Persists buttons? | Calls `btn.Draw()` (keycap+anim)? |
|---|---|---|---|
| Instrument rows / category rows | `drumview_overlay_inst_comp.go` (`drawInstRow` 1851, `drawCategoryRow` 1899) | ✅ `instBtns` / `categoryBtns` | ✅ yes — the reference |
| Row context menu | `drumview_context_menu.go` (`drawContextMenuItem` ~514) | ✅ `contextMenuBtns` | ❌ flat |
| Subdivision selector | `drumview_overlay_subdiv_comp.go` (`drawSubdivRow` 288) | ✅ `buttons` | ❌ flat |
| Template / overflow menu | `drumview_context_menu.go` (`drawOverflowMenu` 913, `overflowPopupBtns` 876) | ❌ rebuilt every frame | ❌ flat |
| Node sidebar dropdown / headers | `game_node_sidebar.go` (1331, 1474) | ✅ (own `drawBtn`) | ✅ already animates (different arrangement) |

All row backgrounds already share `drawMenuItemBackgroundAccent` /
`drawMenuItemBackground` in `menu_chrome.go`. The keycap chrome + animation is
the only styling axis that diverges.

## Out of scope

The **node sidebar** (`game_node_sidebar.go`) is left as-is (user decision). Its
logic/groove dropdown already animates via its own `drawBtn`, and its
collapsible section headers are not selectable list rows. Unifying the four
overlay popup menus is the full scope.

## Design

### 1. Shared primitive — `menu_chrome.go`

Add one renderer that owns *all* row styling + animation, generalizing the
existing `drawInstRow` body. Styling/animation is fixed and identical for every
menu; only the `MenuRowSpec` fields vary per menu.

```go
type MenuRowSpec struct {
    Accent      color.Color   // hover/active tint + active stripe color
    State       menuItemState // rest/hover/active — caller's selection model decides
    Swatch      color.Color   // optional leading rounded-square swatch (instrument color); nil = none
    IconID      IconID        // optional leading icon; "" = none
    IconTint    color.Color   // icon tint; nil/zero defaults to colMenuIcon
    Highlights  []int         // optional fuzzy-match highlight runs
    Label       string        // row text (drawn here; btn.Text is blanked during btn.Draw)
    LabelColor  color.Color    // nil = colTextPrimary (lets context menu keep its red Delete)
    LabelRole   TextRole      // zero = RoleBody
    CenterLabel bool          // subdiv centers its label; the others left-align
}

// drawMenuRow renders one list-menu row with the unified styling + animation:
//   1. drawMenuItemBackgroundAccent(dst, r, spec.State, spec.Accent)
//   2. blank btn.Text, btn.Draw(dst), restore — keycap shell + bevel + press
//      travel + hover glow + AdvancePressAnim (the per-frame anim tick)
//   3. optional swatch (rounded square) and/or leading icon
//   4. optional fuzzy-match highlights behind the label
//   5. label (left-aligned after swatch/icon, or centered) at LabelRole/LabelColor
//
// Every list menu calls this so styling and animation are byte-identical;
// behavior (open/close/toggle) and content live in the caller + MenuRowSpec.
func drawMenuRow(dst *ebiten.Image, btn *Button, spec MenuRowSpec)
```

Leading-element layout precedence: swatch XOR icon takes the leading slot (swatch
wins if both set, matching the instrument menu which has a swatch and no icon);
label x is computed from whichever leading element is present, reusing
`menuRowLabelX` / `menuRowIconRect` geometry where applicable.

### 2. Migration map (styling/animation unified; behavior untouched)

- `drawInstRow` → `MenuRowSpec{Swatch: instColor, Highlights, Accent: instColor, State: active-if-ActiveInstrumentID}` → `drawMenuRow`. This is the reference path and must remain visually identical after the refactor — it is the regression anchor.
- `drawCategoryRow` → `MenuRowSpec{Accent: menuAccent, State: active-if-PopupButtonStyle}` → `drawMenuRow`.
- `drawContextMenuItem` → `MenuRowSpec{IconID, IconTint: colMenuIcon, LabelColor: btn.TextColor, Accent: contextMenuAccent, hover-only}` → `drawMenuRow`. **Gains** keycap + animation.
- `drawSubdivRow` → `MenuRowSpec{CenterLabel: true, Accent: colAccent, State: active-if-Current, LabelColor: colTextAccent-if-current}` → `drawMenuRow`. **Gains** keycap + animation.
- `drawOverflowMenu` template/file rows → `MenuRowSpec{IconID, Accent: contextMenuAccent, hover-only}` → `drawMenuRow`. **Gains** keycap + animation **and** the persistence refactor below.

### 3. Persistence refactor — template/overflow menu only

Replace the per-frame `overflowPopupBtns` (which calls `NewButton` every
invocation, from both `drawOverflowMenu` and `handleOverflowMenuInput`) with a
persisted `dv.overflowBtns []*Button`, mirroring the existing `contextMenuBtns`
pattern:

- Rebuild the slice on menu open and on page switch (File ↔ Templates), when the
  item set/count changes.
- `SetRect` each frame to apply the current scroll offset (rects change per
  frame; the `*Button` objects — and their `pressDepth` / `hovered` state —
  persist).
- Route `handleOverflowMenuInput` hit-testing and the deferred-tap `FireTapAt`
  to the same persisted buttons, so the press lifecycle (`pressTarget` set on
  press/release) drives the animation that `btn.Draw()` then renders.

This is the only menu needing a persistence change; the other three already
persist their buttons.

### 4. Behavior preserved per menu

No `onClick` semantics change. Template rows still reset to page 0, close the
menu, and call `dv.onImport`. Instrument rows still update the live selection and
keep the menu open for audition. Subdiv rows still pick and close. Context items
still run their action and close. The unification is purely draw + animation.

## Testing

- **Regression anchor:** existing instrument-menu tests + a screenshot diff
  confirm `drawInstRow` is visually unchanged after routing through `drawMenuRow`.
- **New unit test** `drawMenuRow` (in `menu_chrome_test.go` or a new
  `menu_row_test.go`): each spec variant (swatch / icon / centered / highlighted
  / active-stripe / per-item label color) renders without panic; active state
  paints the accent stripe; swatch and icon land in the expected leading rect.
- **New animation-persistence test** for the overflow menu, mirroring
  `button_anim_test.go`: press a template row → `pressDepth` advances across
  successive `Draw` frames → settles at rest after release; verifies buttons
  persist (not recreated per frame).
- **Existing suites** kept green: `template_menu_test.go`,
  `template_menu_scroll_test.go`, `template_menu_width_test.go`,
  `template_import_test.go`, `drumview_overlay_subdiv_comp_test.go`, context-menu
  tests, `menu_chrome_test.go`, `menu_chrome_accent_test.go`.
- **Fast path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`.
- **Screenshots:** `make screenshots-all SCENES=<template + inst menu scenes>` for a visual parity check.

## Risks / notes

- The matte-retro restyle means keycap chrome is near-flat at rest (the socket
  fill blends with the panel); the visible delta is on hover/press (cap travel +
  glow). Expect a subtle but real change to the template/context/subdiv rows.
- `token_discipline_test.go` / `design_md_drift_test.go`: the shared renderer
  must use existing `Token*` accessors and design tokens — no new inline color
  literals.
- `git` commit intentionally skipped: the user forbids staging/committing; this
  doc and all subsequent work is left uncommitted for review.
