# Settings-gear protected zone — design

**Date:** 2026-07-10
**Status:** Approved, implementing

## Problem

In the main grid view, the settings gear (`gridHelpBtn`) lives in the grid pane's
top-right corner. Grid-pane popups (the node-group menu, the node long-press
popup, and group badges) clamp their geometry only to the grid-pane bounds
`[0,gridW]×[0,gridH]`. When anchored near the top-right, their panels slide into
the gear's rectangle — visually *underneath* the gear (they draw at a lower z, so
the gear paints on top, but their panel still invades the gear's area). The gear
area must be a protected region no popup or UI element can invade, in **either**
z-direction:

- **On top of it** — nothing paints over the gear.
- **Below / underneath it** — no popup panel geometry overlaps the gear's rect.

## Key facts (verified)

- Gear rect: `(*Game).gridHelpButtonRect()` — a 30px square, 10px inset from the
  grid pane's top-right corner (`game_input_shortcuts.go:262`).
- The grid pane's origin is always `(0,0)` (`Splitter.GridRect` returns
  `image.Rect(0,0,…)`), so the gear rect is directly comparable to every popup's
  grid-local geometry — **no coordinate translation needed**.
- Z-order: the gear is `GZGridHelpButton = 80`, the **topmost** GridTree layer,
  already above group-menu (62), sidebar (60), long-press (50), badges (41).
- No exclusion-zone / reserved-rect concept exists today; all popup clamps only
  keep content inside the pane.

## Design

### Reserved rect

Add `(*Game).gridHelpReservedRect() image.Rectangle` — `gridHelpButtonRect()`
inflated by a small margin (`SpaceSM` = 6px) on all sides. Empty when the gear
does not exist (returns `image.Rectangle{}`); callers treat an empty reserved
rect as "nothing to avoid".

### Spatial exclusion helper (pure)

```go
// avoidReservedRect nudges panel out of reserved (a top-right-corner exclusion
// zone) while keeping it inside the grid pane [0,gridW]×[0,gridH]. Returns the
// adjusted panel:
//   1. no overlap            → panel unchanged
//   2. fits below reserved   → push down  (panel.Min.Y = reserved.Max.Y)
//   3. else fits left of it  → push left  (panel.Max.X = reserved.Min.X)
//   4. else                  → best-effort clamp (pane too small to avoid)
func avoidReservedRect(panel, reserved image.Rectangle, gridW, gridH int) image.Rectangle
```

Empty `reserved` → returns `panel` unchanged.

### Application sites

Call `avoidReservedRect(panel, g.gridHelpReservedRect(), gridW, gridH)` after the
existing pane-clamp at each site:

- `GroupMenu.layoutAt` (`game_group_menu.go`) — adjust `panel` before building
  the child rects (child rects derive from `panel`, so they follow).
- `showLongPressPopup` (`game_longpress_popup.go`) — adjust the popup rect
  before deriving the three button rects.
- `groupBadgeRects` (`game_group_badges.go`) — adjust each badge rect.
- `NodeSidebar.layout` (`game_node_sidebar.go`) — apply too (cheap; only matters
  on a pathologically narrow pane where the left-docked sidebar reaches the
  right edge).

### Z-order invariant

The gear is already topmost. Pin it: a test asserts `GZGridHelpButton` is
strictly the maximum z among all registered grid layers, so a future layer
cannot be added above it silently. Full-screen modal overlays (the settings
overlay itself, dialogs) drawn by the overlay portal legitimately cover
everything — that is the gear's own modal, not an invasion, and is out of scope.

## Tests (TDD — written first)

1. `TestAvoidReservedRect` — pure helper: no-op, push-down, push-left,
   empty-reserved, tiny-pane cases.
2. `TestGroupMenuAvoidsSettingsGear` — open a group menu anchored at the
   top-right corner via real `Game`; assert `groupMenu.InputBounds()` does not
   intersect `gridHelpButtonRect()`.
3. `TestLongPressPopupAvoidsSettingsGear` — long-press a node near the corner;
   assert `longPressPopupRect` does not intersect the gear rect.
4. `TestGroupBadgesAvoidSettingsGear` — group with members in the top-right;
   assert no badge rect intersects the gear rect.
5. `TestSettingsGearIsTopmostGridLayer` — z-order invariant.

## Non-goals

- Re-parenting popups into a different tree or changing input routing.
- Protecting against full-screen modal overlays (intended behavior).
- Any visual restyle of the gear or popups.
