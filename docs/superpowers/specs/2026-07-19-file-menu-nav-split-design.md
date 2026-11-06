# Split navigation from file options; folder icon; pressable beat counter

**Date:** 2026-07-19
**Status:** Design — awaiting user review
**Branch:** add-core-node-types

## Problem

Today three affordances all funnel into one portal (`"overflow-menu"`),
differentiated only by a page number:

- the transport **bullseye** button (`navBtn`, `IconTarget`) → `OpenNavigation()`
- the transport **kebab** button (`overflowBtn`, `IconOverflow`) → the overflow menu
- the **beat-counter** tap (timeline zone) → `OpenNavigation()`

Portal page 0 stacks a **NAVIGATION panel band** on top of a **FILE section**
(Upload / Import / Export / Load Template → templates browser). Navigation and
file options are not actually separate, and two of the three openers are
redundant transport buttons that consume scarce mobile toolbar slots.

## Goals

1. **Separate** the navigation menu from the file-options menu.
2. **Remove** the kebab. File options open from a new **folder icon** placed
   next to the settings gear in the grid pane.
3. **Remove** the bullseye. Navigation opens by tapping the **beat/time counter**,
   which is restyled to look like a pressable button.
4. Free the **two** freed mobile toolbar slots.
5. Desktop behaves the same way.
6. Add a DESIGN.md rule: **a popup menu opens adjacent to the control that
   triggered it.**

Out of scope: retrofitting every existing menu to the new adjacency rule
(the rule is normative going forward; only the two menus in this feature are
re-anchored here).

## Key architectural facts (verified against current code)

- `dv.portal()` is a **single global `OverlayPortal`** on `dv.overlayTree`,
  the highest-z `RootTree` child (`drumview_ctor.go:350-357`).
- The overflow menu is opened with `Scrim: true` → it is a **blocking** portal.
  When a blocking portal is open, `RootTree.dispatchInput` routes **all** input
  to that subtree regardless of `dv.Bounds` (`root_tree.go:106-111`,
  `blockingPortalOwner`). So a press anywhere on screen (grid pane included)
  reaches the menu's `inputFn`, and click-outside closes it.
- `Game.Draw` draws the **grid pane first, then the drum pane** (which
  composites the DrumView overlay portal) — `game_draw.go:27,31`. So an overlay
  anchored in the grid-pane region **paints on top of** the grid pane. This is
  exactly how the gear's full-screen settings overlay already works.
- Therefore the **only** thing pinning the overflow popup to the drum pane is
  `overflowPopupRect()`'s self-imposed clamp to `dv.Bounds`
  (`drumview_context_menu.go:803-853`). Anchoring under a grid-pane button +
  clamping to screen bounds is sufficient — **no new portal is required** to get
  a pixel-anchored, grid-pane-hosted file menu.
- The beat-counter tap is already wired to `OpenNavigation` via the
  isolation-tested `beatCounterHitAdapter` (`timeline_zone.go:1190-1232`); only
  its **visual** needs to change.
- The gear button (`gridHelpBtn`) is a `Button` routed through
  `gridHelpInputHandler` (z=300) in the grid pane's `inputDispatcher`, drawn in
  `drawGridHelpButton`, rect from `gridHelpButtonRect()` (30 px, 10 px inset,
  top-right). The folder button mirrors this exactly.

## Design

### A. Transport bar — remove two buttons (both platforms)

Delete `navBtn` and `overflowBtn` end-to-end in `transport_zone.go`: struct
fields, `initButtons`, style assignment, `layoutMobile` / `layoutDesktop`
placement, `rebuildHitAreas` (`transport-nav`, `transport-overflow` entries in
`simpleButtons`), the `square()` calls, draw, dirty-hash mixing, and
expand-rects.

- **Mobile single-row** weights `[]float64{1,1,1,3,1,1,1,1}` → `[]float64{1,1,1,3,1,1}`
  (order becomes Play · Stop · Record · [−|BPM|+] · Subdiv · Vol). Two removed
  cells = the two freed positions.
- **Mobile two-row fallback** row-1 grid `[1,1,1]` (Vol|Nav|Overflow) → `[1]` (Vol).
- **Desktop** running order loses Nav and Overflow → 9 controls
  (Play · Stop · Record · BPM box · BPM ± · Subdiv · Vol · Undo · Redo); update
  `nGaps` / `totalUnits` in `layoutDesktop` accordingly.

The transport `OnNavigate` / `OnOverflowOpen` callbacks and their ctor wiring
are removed. `uploadBtn`/`importBtn`/`exportBtn` (hidden helper buttons whose
`OnClick` are the real file handlers) **stay** — the file menu invokes them.

### B. Folder icon in the grid pane

- Add `IconFolder` to `icons.go` + `drawFolderIcon` in `drawing_icons.go`
  (Lucide-style folder: tab + body, stroke `IconStrokeWeight`) + dispatch case.
- Add `g.gridFileBtn *Button` (`game_struct.go`), constructed in `game_new.go`
  (`NewSpecButton("", ComponentButtonSecondary, g.openFileMenu)`, `Icon = IconFolder`).
- `gridFileButtonRect()`: same 30 px square, placed one gap (`SpaceSM`) to the
  **left** of `gridHelpButtonRect()`. Present on both platforms (mirror the gear).
- `gridFileInputHandler` in the grid pane `inputDispatcher`, z just below the
  gear's 300 (rects are disjoint, so z ordering among them is immaterial as long
  as both beat the drum at 100). Draw beside the gear in `drawGridHelpButton`
  (or a sibling `drawGridFileButton`). Layout sync in `game_layout.go`.
- Extend `gridHelpReservedRect()` to the **union** of gear + folder rects so
  grid-pane popups (`avoidReservedRect` callers) dodge both.
- `g.openFileMenu()` → `g.drum.OpenFileMenu()`.

### C. Two menu modes on the reused global portal

Add `overflowMode` to DrumView: `overflowModeNav` | `overflowModeFile`.

- `overflowItems()`:
  - **File mode**, page 0 → the FILE rows only (Upload/Import/Export/Load
    Template); page 1 → templates browser (unchanged).
  - **Nav mode** → **no** list rows (the NAVIGATION panel band is the whole body).
- `overflowNavPanelH()` → panel height only when `overflowMode == nav` (was
  keyed on `overflowPage == 0`). Nav mode always page 0.
- `overflowAnchorRect()` (new): file mode → `dv.game.gridFileButtonRect()`;
  nav mode → the beat-counter rect (`dv.beatCounterRect`).
- `overflowPopupRect()`: replace the hard-coded `overflowBtn().Rect()` anchor
  with `overflowAnchorRect()`; clamp to **screen bounds** (`dv.game.winW/winH`)
  in file mode so the popup may render into the grid pane, and to `dv.Bounds`
  in nav mode. Drop-below with flip-above fallback; content-sized width.
- `OpenFileMenu()` (new): `overflowMode = file`, `overflowPage = 0`, open portal.
- `OpenNavigation()`: set `overflowMode = nav`, `overflowPage = 0`, open portal.
- The hover/draw/input funcs (`drawOverflowMenu`, `handleOverflowMenuInput`,
  `configureOverflowScroll`, template search, page-1 browser, go-to editors) are
  reused unchanged; they already branch on `overflowNavPanelH()` and the item
  list, both now mode-derived.
- `overflowPopupRect`'s nil-guard on `overflowBtn()` is replaced by an
  anchor-empty guard.

Result: file menu = file rows only; nav menu = navigation panel only.

### D. Beat/time counter → keycap button

`drawBeatCounter` (`timeline_zone.go:554-589`) is restyled from a flat
`colSurface1` chip to a **raised keycap**: keycap fill + border, hover-glow when
hovered, pressed-inset when pressed, with a **leading `IconTarget`** before the
readout text. Hover/press state is tracked on the beat-counter hit adapter
(press already flows through it) and read by `drawBeatCounter`. Input path and
`OnOpenNavigation` wiring are unchanged (keeps the isolation tests valid).
`beatCounterSlotWidth`/`beatCounterRect` layout grows to accommodate the leading
icon so the readout never truncates (mobile compact `"12 · 1:04"`, desktop
`"Beat 12 · 1:04"`).

### E. DESIGN.md rule

Add, near the menu/popup component section, a **Menu placement** rule:

> **Menu placement — adjacent to trigger.** A popup menu opens adjacent to the
> control that triggered it, dropping below the trigger (flipping above when
> there is no room below), so the visual origin of the menu is unambiguous.
> This applies to floating dropdown/context menus on both platforms. The mobile
> full-width bottom sheet remains the form for large modal menus, but even then
> it is invoked from — and reads as originating from — its trigger control.

## Testing (TDD — tests written first)

New / updated Go tests in `internal/ui/` (fast `-tags test` path):

1. **Transport slot removal** — no `transport-nav` / `transport-overflow` hit
   areas registered; mobile single-row lays out 6 controls; desktop lays out 9.
   (Update/replace `transport_nav_button_test.go`,
   `transport_icon_uniqueness_test.go`.)
2. **Folder button** — `gridFileButtonRect()` non-empty on both platforms, sits
   immediately left of `gridHelpButtonRect()`, disjoint, same size; icon ==
   `IconFolder`; click opens the file menu.
3. **Reserved-zone union** — `gridHelpReservedRect()` (or its successor) covers
   both gear and folder; a popup anchored at the folder corner is pushed clear.
4. **File menu content** — `OpenFileMenu()` yields a menu whose rows are exactly
   Upload/Import/Export/Load Template and whose `overflowNavPanelH() == 0`
   (no navigation band).
5. **Nav menu content** — `OpenNavigation()` yields `overflowNavPanelH() > 0`
   and **zero** file rows.
6. **Menu adjacency** — file-menu popup rect is anchored under the folder button
   (top edge near the button's bottom, right-aligned within screen); nav-menu
   popup rect anchored under the beat counter.
7. **Keycap counter render** — `drawBeatCounter` emits a raised keycap
   (fill + border + leading icon) via the `drawRect`/`DrawIcon` interceptors,
   and reflects hover/press state.
8. Update affected existing tests: overflow/scene-catalog scenes
   (`mobile_overflow_open`, `overflow_menu_open`), settings-entry tests,
   beat-counter chip tests, nav-menu tests.

Then **visual verification**: `make screenshots-all` on desktop + mobile
(scenes covering the grid pane top-right, the file menu open, the nav menu open,
and the restyled counter), reviewed at the end.

## Risks / notes

- The DESIGN.md adjacency rule mildly tensions the existing "mobile context menu
  = full-width bottom sheet" convention; the rule text is phrased to keep bottom
  sheets valid for large modal menus while making trigger-origin the default.
- Reusing the global portal (rather than a genuinely separate grid-pane portal)
  delivers the same pixel-anchored result the user selected, at far lower churn,
  because the portal already draws over the grid pane and owns input globally
  when its scrim is up. If a future need arises for a non-blocking grid-pane
  menu, a separate portal would then be warranted.
- `RibbonBeatsPerPixel` and other nav-related stubs are untouched.
