# Grid Pop-up Unification — Design

**Date:** 2026-07-20
**Status:** Draft (awaiting review)
**Area:** `src/go/internal/ui/`

## Problem

Pop-up menus opened from the grid pane are inconsistent in *where* they appear and *how* they are built. Menus triggered from the grid's top-right button cluster (Settings, Logs) render pinned into the **bottom drum/audio panel** instead of next to the button that opened them. Underneath, grid pop-ups are split across **two unrelated rendering + input paths**, and two unrelated positioning systems. The goal is to unify all interactive grid pop-ups onto **one render/input path (the global `OverlayPortal`)** and **one grid-aware positioning helper**, so every menu appears next to the control/node that opened it, while each menu keeps its own content.

## Root cause of the "menu appears in the bottom panel" bug

Overlays position themselves via `AnchorPopupRect(clampBounds, anchor, w, h, prefer)` (`overlay_anchor.go:35`). Most DrumView-owned menus pass `dv.Bounds` as the clamp region — but `dv.Bounds` is **only the bottom drum pane** (`splitter.go:41`; its top edge is the split line). When the *anchor* is a grid-pane button (Y above the split line), `ClampRectInto` (`overlay_anchor.go:93`) shoves the card **downward until its top hits the split line**, pinning it to the top of the bottom panel.

The File/overflow menu already fixed this in `overflowPopupRect` (`drumview_context_menu.go:766`) by swapping the clamp region to the **full window** `Rect(0,0,winW,winH)`. Settings (`settings_overlay.go:64`, centers on `screenBounds`) and Logs (`logs_component.go:279`, centers on `dv.Bounds`) never got that treatment.

## The two paths (current state)

**Path A — global `OverlayPortal`** (`portal.go`, owned by `dv.overlayTree`, `drumview.go:290`). Composites *above* the grid pane (`game_draw.go:27`, `root_tree.go:205`), hit-tests at z=300+, gives click-outside/Esc close. Fronted by two wrappers: `compPortalOverlay` (wraps a self-contained `compOverlayable` component, `comp_portal_overlay.go`) and `dvOverlayPortal` (wraps DrumView methods, `dv_overlay_portal.go`). Shared chrome: `AnchorPopupRect`, `drawMenuRow`/`MenuRowSpec` (`menu_chrome.go:198`), `MenuScroll` (`menu_scroll.go`), `drawMenuSearchBox` (`menu_search_field.go`).

Path A already hosts: context menu, subdiv menu, nav menu, instrument menu, wheel/slider popups, action/overflow "⋯" menu, color wheel, rename, FX panel, naming dialog, **File menu**, **Settings**, **Logs**, and all **confirmations** (`ConfirmDialog`, `confirm_dialog.go`).

**Path B — GridTree grid-local draw + grid `inputDispatcher`.** Draw is a `gridLayer` in `grid_pane_layers.go` at a `GZ*` z-index (draw-only; grid input is deliberately NOT tree-routed). Input is a separately-registered `InputHandler` in the grid pane's `inputDispatcher` (`game_update.go:410-443`), or an ad-hoc per-frame handler in `Game.Update`.

Only **three** interactive grid pop-ups remain on Path B:

| Path-B pop-up | Render | Input | Positioning |
|---|---|---|---|
| **Node long-press popup** (Move/Copy/Connect/Delete) | `grid-longpress` z=`GZLongPress`(50), `grid_pane_layers.go:45` | ad-hoc `Game.Update` intercept + slide-release gesture (`game_longpress_popup.go:89`, short-circuits dispatcher at `game_update.go:397`) | bespoke clamp to `GridW/GridH` + `avoidReservedRect` (`game_longpress_popup.go:14`) |
| **Group menu** (marquee) | `grid-group-menu` z=`GZGroupMenu`(62), `grid_pane_layers.go:47` | dispatcher `Register(g.groupMenu)`, `ZIndex 210` (`game_update.go:436`, `game_group_menu.go:260`) | `placeGroupMenu` — anchor-adjacent + `clampRectToPane` + gear-avoid + member-avoid (`grid_reserved_zone.go:98`) |
| **Node sidebar** | `grid-sidebar` z=`GZSidebar`(60), `grid_pane_layers.go:46` | dispatcher `Register(g.sidebar)`, `ZIndex 200` (`game_update.go:433`, `game_node_sidebar.go:1030`) | docked full-height left panel: `Rect(0,0,w,gridH)` (`game_node_sidebar.go:267`) |

## Two things "unify" must reconcile

1. **A positioning fork.** Path A's `AnchorPopupRect` is a simple flip+clamp with no awareness of the gear cluster or the nodes underneath. Path B's `placeGroupMenu`/`avoidReservedRect`/`clampRectToPane` *is* grid-aware — it dodges the gear buttons and avoids covering selected nodes. Naively moving a Path-B menu onto `AnchorPopupRect` would regress placement. The shared helper must **absorb the grid-aware avoidance** as options.
2. **A render+input fork.** The three Path-B pop-ups must move onto the portal wrappers, and their GridTree layers + parallel input registrations deleted.

## Target architecture

### Principle
Every interactive grid pop-up renders through the **single global `OverlayPortal`** and positions with **one shared grid-aware helper**, anchored to the control/node that opened it. **Plumbing + positioning are shared; menu content stays independent** — no menus are merged, no bespoke chrome is forced into `drawMenuRow` unless the content is already a plain row list.

### Component 1 — shared grid-aware positioning helper (foundation)
Generalize the existing grid-aware placer (do **not** introduce a third positioner) into one helper:

```go
type GridAnchorOpts struct {
    Prefer      PopupSide         // default PopupBelow
    Avoid       []image.Rectangle // e.g. the reserved gear-button zone
    KeepVisible []image.Rectangle // e.g. selected group members
}

// gridAnchoredPopupRect positions a popup of size (w,h) next to `anchor`,
// clamped to the FULL WINDOW (so it drops into the grid pane instead of being
// pinned into the bottom drum pane), dodging the reserved gear zone and any
// KeepVisible rects.
func (g *Game) gridAnchoredPopupRect(anchor image.Rectangle, w, h int, opts GridAnchorOpts) image.Rectangle
```

Internals reuse the existing pieces: `AnchorPopupRect` for the base flip, a full-window clamp box (`Rect(0,0,g.winW,g.winH)`), then the `avoidReservedRect` + least-overlap adjustment already living in `placeGroupMenu`/`grid_reserved_zone.go`. This **single helper subsumes `overflowPopupRect` and `placeGroupMenu`**. The pure `AnchorPopupRect` stays as-is for bottom-pane menus (context/subdiv/wheel) that correctly clamp to `dv.Bounds` and need no gear-avoidance. Mobile `Profile().UseBottomSheet` behavior is preserved (the helper defers to the bottom-sheet path on mobile where those menus already do).

**Parity proof:** File menu adopts the helper in this phase with **zero visible change**.

### Component 2 — one render/input path: the portal
Port the three Path-B pop-ups onto portal wrappers (`compPortalOverlay`; all three already expose `IsOpen/Close/InputBounds/HandleInput/HandleWheel/Capturing/Draw`-shaped methods, so glue is small), then delete their GridTree layers + parallel input registrations. The portal already composites above the grid pane, so there is no z regression.

| Surface | From | To | Notes |
|---|---|---|---|
| **Settings** | portal, centered on bounds | anchor under gear via helper | thread `gridHelpButtonRect` as anchor |
| **Logs** | portal, centered on `dv.Bounds` | anchor under logs button via helper | thread `gridLogsButtonRect` as anchor |
| **Group menu** | GridTree z=62 + dispatcher z=210 | `compPortalOverlay` + helper | **same dimensions/content**; KeepVisible=members, Avoid=gear |
| **Node sidebar** | GridTree z=60 docked + dispatcher z=200 | `compPortalOverlay` + helper | **same dimensions/content/layout**; anchor = tapped node's screen rect (`nodeScreenRect`); Avoid=gear |
| **Long-press popup** | GridTree z=50 + ad-hoc intercept | `compPortalOverlay` + helper | slide-release gesture re-expressed as portal OnPress/OnDrag/OnRelease **with capture handed over on open**; Avoid=gear |

**Node sidebar / group menu — no reshape.** Per the requirement "same overall dimensions and everything, but docked to the node instead of the side," the sidebar keeps its current width and full grid-height and all content/layout; only its *position* changes from the left edge to beside the tapped node. The group menu likewise keeps its dimensions and stays anchored to the selected node/group.

### Component 3 — Path-B cleanup
After migration, remove: the `grid-longpress` / `grid-sidebar` / `grid-group-menu` gridLayers and their `GZLongPress` / `GZSidebar` / `GZGroupMenu` z-consts (update `grid_layer_zorder_test.go`); the `inputDispatcher.Register(g.sidebar)` / `Register(g.groupMenu)` calls (`game_update.go:432-436`); and the long-press pre-dispatch intercept (`game_update.go:397`). GridTree retains only true content layers (nodes/edges/badges/cursor label). This **tightens** the documented "GridTree is draw-only, grid input via dispatcher" invariant — these three menus were the exceptions that muddied it.

## Non-goals (deliberately excluded)
- **Confirmations** (`ConfirmDialog`) — already shared, already on the portal; *centering* a destructive yes/no dialog over the grid pane is correct UX and must not be anchored to a tiny trigger. Left as the already-unified end-state. (It will consume the shared helper only if it ever needs anchored placement.)
- **Merging menu content** — each menu keeps its own component and chrome. Only plumbing + positioning are shared.
- **Migrating grid node-drag input into a tree** — out of scope; the portal only handles input while an overlay is open, so it does not touch the node-drag gesture model.

## Phasing

Each phase compiles, keeps the fast + real test suites green, and is screenshot-verified before the next begins.

- **P0 — shared helper.** Add `gridAnchoredPopupRect` + `GridAnchorOpts`; refactor `overflowPopupRect` (File menu) to use it (parity proof, no UX change).
- **P1 — Settings + Logs anchoring.** Thread each button rect as the anchor; switch their self-layout to the helper. *Satisfies the original request.*
- **P2 — Group menu → portal.** Wrap in `compPortalOverlay`; positioning via helper (KeepVisible=members, Avoid=gear); remove GridTree layer + dispatcher reg.
- **P3 — Node sidebar → node-anchored.** Wrap in `compPortalOverlay`; anchor to `nodeScreenRect` via helper (Avoid=gear); same dimensions/content; remove GridTree layer + dispatcher reg.
- **P4 — Long-press popup → portal (last, riskiest).** Wrap in `compPortalOverlay`; re-express the hold-slide-release gesture through portal capture; remove GridTree layer + `Game.Update` intercept.

## Testing & verification

**Mandatory UI verification (CLAUDE.md rule).** For each migrated surface, drive a scene into the exact open state and capture a real screenshot (desktop + `MOBILE=1`), open the PNG, and iterate until the menu sits next to its trigger with legible content and no overlap/blank chrome.
- Reuse/extend scenes: `shortcuts_overlay` (Settings), `logs_panel` (Logs), `overflow_menu_open` (File), plus the long-press scene (`scene_catalog.go:239`).
- Add scenes: group menu anchored to a selected group; node sidebar anchored to a tapped node (desktop + mobile).

**Structural / regression guards.**
- `grid_layer_zorder_test.go` — updated for removed layers.
- `zone_input_isolation_discipline_test.go` — opaque-zone catch-alls still correct.
- `grid_reserved_zone` tests — gear-avoidance still honored through the helper.
- Portal / close tests — `popup_close_test.go`, `settings_overlay_modal_close_test.go`, group-menu and long-press flow tests (`grid_mode_flow_test.go`, `longpress_popup_test.go`, `move_node_test.go`).
- Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`.
- Real Ebiten (pixel-truth): `xvfb-run -a` variants where scenes assert pixels.

## Risks

1. **Long-press cross-boundary gesture (P4).** The press starts on a node in the grid input path, then the popup opens mid-gesture and must inherit capture (portal `InputCaptured` → subsequent `OnDrag` routed to the captured handler). Highest risk; isolated to the last phase. Fallback: keep long-press on Path B if the gesture cannot be cleanly re-expressed, and document the single remaining exception.
2. **Full-height sidebar beside a node.** Keeping full grid-height means "docked to the node" mostly shifts it horizontally and flips sides near edges; verify it reads well in screenshots. If it looks wrong, the requirement (not a reshape) is revisited with the user — not silently changed.
3. **Mobile bottom-sheet.** The helper must preserve `Profile().UseBottomSheet` so phones keep full-width sheets rather than tiny anchored cards.

## Open questions
- Sidebar height: confirmed "same dimensions" = keep full grid-height; flag at review if a shorter drop-down card was intended.
- Should the group menu's node-avoidance also apply to the node sidebar (avoid covering the tapped node), or is edge-clamp enough? Default: Avoid=gear only for the sidebar; revisit if screenshots show the card covering its own node.
