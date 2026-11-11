# GridTree — z-ordered render + input for the grid pane

**Date:** 2026-06-12
**Status:** Approved (design)
**Branch:** add-core-node-types

## Problem

### The reported bug
On desktop, the node coordinate badge — the `(i, j)` pill drawn above a selected
node — renders **on top of** the node sidebar (the node "pop-up menu").

Root cause: the grid pane (`game_draw_grid_pane.go`) draws **everything as a flat
sequence of direct draw calls** in one ~1100-line function. Z-order is an emergent
property of statement order. The order is:

1. grid background → edges → nodes → pulses
2. node sidebar — `g.sidebar.Draw(screen)` (`game_draw_grid_pane.go:996`), a
   left-anchored panel `image.Rect(0, 0, w, gridH)` covering the left strip of the
   grid full-height.
3. coordinate badge (`game_draw_grid_pane.go:1053-1080`).
4. long-press popup → connect-mode → cursor label.

The badge draws **after** the sidebar. The mobile branch guards it:

```go
showBadge = (g.sel == g.coordBadgeNode) && !g.sidebar.IsOpen()   // line 1059
```

with an explicit comment ("the badge renders on top of the popup since it draws
after it"). The **desktop** branch has no such guard:

```go
showBadge = (g.frame-g.coordBadgeFrame < 180)                     // line 1062
```

So on desktop: select a node on the left → the sidebar opens over that region → the
`(i, j)` badge keeps painting on top of the sidebar for ~3 seconds.

### The deeper cause (what we actually fix)
Z-order and input priority in the grid pane are unstructured. Any new overlay added
in the "wrong" line of `drawGridPane` silently mis-composites. Input has the same
problem: nodes are hit-tested with direct `nodeAtScreen`/cursor checks, not the
`HitIndex` the audio pane uses, so click-through bugs (a tap on an overlay also
hitting a node beneath) are a permanent risk.

The audio/drum pane already solved exactly this with `DrumViewTree`
(Zone/Layer/HitArea/HitIndex, strict 4-phase Layout→Update→Input→Draw, opaque-to-z
catch-all contract). The grid pane never adopted it. This design brings the same
discipline to the grid pane.

## Goals

- Fix the badge-over-popup bug **structurally** (not a per-platform guard).
- Give the grid pane a z-ordered render tree and HitIndex-based input arbitration,
  reusing the existing primitive types.
- Be strictly incremental and behavior-preserving for the intricate grid gesture
  code (pan, node-drag, connect/move/origin modes).

## Non-goals

- Rewriting the grid gesture state machines (pan/node-drag/connect/move). Their
  internals are reused, called from a HitHandler. (Decision: "Wrap + arbitrate.")
- Touching `DrumViewTree` or the audio pane. It is left untouched → zero regression
  risk to the bottom pane.
- Moving grid modals into a shared `OverlayPortal`. v1 keeps grid dialogs as
  high-z grid zones. (Can be revisited later.)

## Architecture

### A sibling tree reusing shared primitives
The primitive types are standalone in package `ui` and reusable as-is:
`Zone`, `Layer`, `HitArea`, `HitHandler`, `HitIndex`, `zoneAsLayer`,
`InputResult`, `NewInputCaptureHitArea`.

New file `grid_tree.go` defines a `GridTree` struct that reuses those types and runs
the **same 4-phase loop and capture/suppress + opaque-to-z catch-all contract** as
`DrumViewTree`, minus what the grid does not need:

- **No `OverlayPortal`** (grid modals are plain high-z zones in v1).
- **No transport/eq keyboard auto-focus** (those are drum-pane specifics).

`GridTree` owns its own `HitIndex` and its own bounds
(`g.split.GridRect(g.winW, g.winH)` — the top pane).

> Note on sharing: the capture-lifecycle fields (`capturedHandler`, `suppress`,
> `wasPressed`, …) are duplicated in v1 to keep `DrumViewTree` untouched. A later
> cleanup may factor them into a tiny embeddable helper used by both trees. Not in
> scope for v1.

### Coordinate space
`HitIndex` and `cursorPosition()` are in **screen** pixels. Node HitAreas are
computed in screen space via the existing `g.nodeScreenRect`. The world-space camera
(`cam.Scale`, `OffsetX/Y`, `gridTopOffset()`) is used only inside Layer/Zone `Draw`
methods, exactly as today.

### Component decomposition (grid-local z)

Grid-local z values 0–70. Screen-wide portal overlays owned by `DrumViewTree`
(z ≥ 300) are unaffected.

| Participant | Kind | z | Migrated from (`game_draw_grid_pane.go` unless noted) |
|---|---|---|---|
| `GridBackgroundLayer` | Layer | 0 | gradient + tile cache (43–144) |
| `EdgeLayer` | Layer | 10 | edges + edge cache (200–431) |
| `GridCanvasZone` | **Zone** | 20 | node draw / state overlays / glow / selection (485–975) **+ node hit-testing & all grid gestures** |
| `PulseLayer` | Layer | 30 | edge pulses (1001–1025) |
| `CoordBadgeLayer` | Layer | 40 | `(i, j)` badge (1053–1080) — **now below all popups** |
| `MoveModeLayer` | Layer | 42 | move banner + ghost (1082–1121) |
| `ConnectModeLayer` | Layer | 44 | connect visuals (`game_connect_mode.go`) |
| `LongPressPopupZone` | **Zone** | 50 | Move/Connect/Delete popup (`game_longpress_popup.go`) |
| `MoveConfirmZone` | **Zone** | 52 | move-confirm dialog (1123–1140) |
| `NodeSidebarZone` | **Zone** | 60 | node sidebar (`game_node_sidebar.go`) |
| `CursorLabelLayer` | Layer | 70 | desktop cursor label (1148–1169), unclipped |

The bug fix is structural: `CoordBadgeLayer` at z=40 is strictly below
`LongPressPopupZone` (50), `MoveConfirmZone` (52), and `NodeSidebarZone` (60). Since
`GridTree.Draw` walks ascending z, the popups always composite over the badge. The
per-platform `!sidebar.IsOpen()` guard is no longer load-bearing (it may remain for
"don't show a stale badge" UX, but it is not what prevents the overpaint).

### Input model (the robustness win)

`GridCanvasZone` registers a **full-bounds catch-all `HitArea` at z=20** whose
`HitHandler` delegates to the existing gesture functions:

- `OnPress` → node hit-test (`nodeAtScreen`) then the existing
  `handleTapInGrid`-style dispatch (select / open sidebar / create node /
  connect-mode tap / move-mode tap / origin select / long-press start).
- pan / node-drag / zoom continue to use the existing per-frame logic, invoked via
  the handler's drag/release callbacks (or gated legacy polling — see Integration).

The overlay zones register **higher-z** catch-alls + per-control hit areas:

- `NodeSidebarZone` (z=60): full-panel catch-all at z=60 + the sidebar's own controls.
  v1 wraps the existing `sidebar.Hit`/input dispatch behind the catch-all + delegate;
  internal control dispatch is preserved.
- `LongPressPopupZone` (z=50): scrim + panel catch-all at z=50 + three button
  HitAreas (Move/Connect/Delete) at z=51, replacing `updateLongPressPopup`'s manual
  hit math.
- `MoveConfirmZone` (z=52): panel catch-all + Move/Cancel buttons.

Because `HitIndex.At` dispatches z-descending and exact-rect-first, a press on an
overlay is **claimed before it can fall through** to `GridCanvasZone` at z=20. This
eliminates the click-through class of bug (e.g. tapping a sidebar control also
creating/selecting a node beneath).

### Integration

**Draw.** `drawGridPane(screen)` becomes dispatch-only: it lays out the grid-tree
zone rects for the frame and calls `g.gridTree.Draw(screen)`. Every former draw block
moves verbatim into its Layer/Zone `Draw` method — the node sprite cache, edge cache,
and tile cache logic is **relocated, not rewritten**.

**Update.** `Game.Update` calls `g.gridTree.Update()` for the grid region. When the
tree claims a press (an overlay zone consumed it, or the canvas zone captured a
drag), `gridTree.InputHandled()` / suppress gate the legacy grid dispatch so input is
processed exactly once. When the tree does not claim the press, existing grid gesture
polling runs unchanged.

**Two trees, disjoint regions.** `GridTree` (top pane) and `DrumViewTree` (bottom
pane) own non-overlapping screen rects; each `HitIndex` only contains areas within
its own bounds, so a press routes to exactly one tree. Grid modals are positioned
within the grid pane and belong to `GridTree`.

## Testing

| Test | Mirrors | Asserts |
|---|---|---|
| Bug regression (z + pixel) | new | badge z < popup/sidebar z; with sidebar open + node selected on **desktop**, badge pixels never land inside the sidebar rect (drawRect interception) |
| Z-order table | `drumview_layer_zorder_test.go` | pinned grid Layer/Zone z ordering; new participant must update the table |
| Opaque-to-z discipline | `zone_input_isolation_discipline_test.go` | every opaque grid zone registers a full-bounds catch-all at its nominal z |
| Click-through regression | `pads_input_regression_test.go` | press on sidebar/popup/dialog does not reach `GridCanvasZone` (no node create/select) |
| Behavior preservation | existing | node-click / node-drag / connect / move / long-press tests still pass unchanged |
| Grid render-pipeline discipline | `render_pipeline_discipline_test.go` | no draw primitives in `drawGridPane` beyond the `gridTree.Draw` dispatch (Decision: "Add it") |

Fast path (stubbed Ebiten):
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui
```

## Phasing

Each phase is independently shippable and lands with its tests.

1. **Orchestrator.** `grid_tree.go` (`GridTree` reusing primitives, 4-phase loop,
   capture/suppress, opaque-to-z catch-all). Engine unit tests in isolation.
2. **Draw migration.** Move every `drawGridPane` block into Layers/Zones;
   `drawGridPane` becomes `gridTree.Draw` dispatch. **Bug fixed here** (badge as
   low-z Layer). Z-order table test + bug regression test.
3. **Input migration.** `GridCanvasZone` catch-all delegating to existing gestures;
   `NodeSidebarZone` / `LongPressPopupZone` / `MoveConfirmZone` as input zones; route
   `Game.Update` through `gridTree`. Opaque-to-z + click-through tests; behavior-
   preservation suite green.
4. **Cleanup + discipline + docs.** Grid render-pipeline discipline test; update
   `CLAUDE.md` (grid pane is now tree-managed) and project memory.

## Risks & mitigations

- **Node/edge/tile caching is intricate.** Mitigation: relocate the code into Layer
  Draw methods unchanged; the caches key off the same camera/signature inputs.
- **Press/drag/release routing for the canvas zone.** Mitigation: "Wrap + arbitrate"
  keeps the existing gesture state machines; the tree only arbitrates which surface
  owns the press. Behavior-preservation tests guard it.
- **Double dispatch (tree + legacy).** Mitigation: gate legacy grid input on
  `gridTree.InputHandled()` / suppress, mirroring how `DrumView.Update` already
  consults the drum tree.
