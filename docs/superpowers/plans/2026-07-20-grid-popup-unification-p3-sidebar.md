# Grid Pop-up Unification — P3: Node Sidebar → Portal (Node-Anchored Compact Card)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax. **This plan needs user approval of the persistence design (§2) before execution.**

**Goal:** Make the node sidebar a node-anchored compact card that pops up next to the tapped node (like the long-press popup), instead of a full-height panel docked to the grid pane's left edge.

**Architecture:** Migrate `NodeSidebar` from a **GridTree draw-layer + grid `inputDispatcher` handler** onto the **global `OverlayPortal`** (via `compPortalOverlay`), and size it as a compact card anchored with `(*Game).gridAnchoredPopupRect`. The portal move is mandatory, not cosmetic (see §1).

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten v2, ebitenstub `-tags test`.

## Global Constraints
- **Go binary:** `../../.tools/go/bin/go` from `src/go`. Fast tests: `-tags test -modfile=go.test.mod`.
- **Never stage or commit.** Each task ends at green tests + inspected screenshots; the user commits.
- **Mandatory UI verification:** screenshot every visible change (desktop + `-mobile`), open the PNG, iterate.
- **No new color literals.**
- **Reuse:** anchor via the existing `(*Game).gridAnchoredPopupRect` (added in P0); do not re-roll positioning.

---

## 1. Why the portal migration is mandatory (proven, not assumed)

`game_draw.go` draws the **grid pane first, then the drum pane**. The sidebar is a GridTree layer (`grid-sidebar`, `GZSidebar=60`, `grid_pane_layers.go:46`), so **anything it paints below the split line is overpainted by the transport/rows/EQ panel.** A node-anchored card for a mid-grid node needs ~288px, and the grid pane is only ~288px tall — so the card *must* paint past the split. Only portal overlays composite above both panes (this is exactly why P1's Settings/Logs/File work). Verified empirically: with the card correctly sized (`contentH=240, cardH=288, origin=(679,238)`) it still rendered header-only because the content fell below the split and was covered.

**Corollary:** the compact-card *layout* changes are known-good and reusable (see §3); the blocker was purely the render/input surface.

## 2. Persistence design (NEEDS USER APPROVAL)

The sidebar is unlike every other portal overlay: it is **non-modal, persistent, and follows node-selection** (tap another node → sidebar retargets; it does not close). Portal modes don't fit cleanly (`Modal:true` blocks the grid; `Passive:true` owns no input; default non-modal closes on outside tap).

**Recommended design — "reopen-on-select, close-on-deselect":**
- Open the sidebar as a **non-modal, non-passive** portal entry (`Modal:false, Scrim:false, Passive:false`).
- Node-selection already calls `selectNodeOpenSidebar → sb.Open(node)` (`game_node_interaction.go:89`). Keep that: tapping another node closes the current entry (outside-tap) and immediately reopens it for the new node via the existing selection path. Net effect: it *follows* selection (one-frame close→reopen; verify no visible flicker in the screenshot pass).
- Tapping empty grid closes it = **deselect**, which is reasonable UX.
- The close (X) button and Esc close it explicitly.

**Alternative (if flicker is visible):** add a `Sticky bool` to `PortalEntry` (portal.go) that suppresses outside-tap close for that entry; the sidebar then updates its node in place on selection without close/reopen. More code, no flicker. **Decision point for the user: accept reopen-on-select, or invest in the `Sticky` flag?**

## 3. The compact-card layout change (verified working; re-apply in Task 2)

In `(*NodeSidebar).layout()` (`game_node_sidebar.go:267`), after `contentH` is computed (currently ~line 383):
- Size the card from content instead of full pane: `cardH := contentTop + contentH + sidebarPad; if cardH > gridH { cardH = gridH }`.
- `viewportH := cardH - contentTop`; `sb.rects["panel"] = image.Rect(0,0,w,cardH)`.
- Anchor + translate: `nodeR` from `g.nodeScreenRect(sb.node)` (returns floats → `image.Rect(int(x1)…)`); `origin := g.gridAnchoredPopupRect(nodeR, w, cardH, GridAnchorOpts{}).Min`; if non-zero, translate every `sb.rects[k]` and `sb.scroll.VS.View` by `origin`.
- Replace the two hard-coded `gridH` viewport bottoms with the card bottom:
  - `inViewport` (`game_node_sidebar.go:1436`): use `sb.rects["panel"].Max.Y`.
  - `HandleInput` viewport (`:1119`): use `panelR.Max.Y`.
  - Draw content-clip `SubImage` (`:1541`): use `panel.Max.Y` (not `gridH`).
- Remove the early `sb.rects["panel"] = Rect(0,0,w,gridH)` (`:289`) and the early `viewportH := gridH - contentTop` (`:326`).

(These exact edits compiled and sized correctly in the spike; they were reverted only because the surface was still GridTree.)

## 4. Portal migration touch points
- **Remove** the `grid-sidebar` gridLayer (`grid_pane_layers.go:46`) and `GZSidebar` const (update `grid_layer_zorder_test.go`).
- **Remove** `inputDispatcher.Register(g.sidebar)` (`game_update.go:433`).
- **Add** `compOverlayable` conformance to `NodeSidebar`: confirm `IsOpen/Close/InputBounds/HandleInput/HandleWheel/Capturing/Draw` exist and match the interface (`comp_portal_overlay.go:13`); add any missing method.
- **Open via portal** in `selectNodeOpenSidebar` (`game_node_interaction.go:89`): `dv.portal().Open(PortalEntry{ID: nodeSidebarID, Overlay: &compPortalOverlay{comp: g.sidebar, tag: nodeSidebarID, updateFn: …}, Modal:false, Scrim:false, OnClose: g.sidebar.Close})`. `Close()` path closes the portal entry.
- **Draw** no longer calls `sb.layout()` itself via GridTree; the portal drives `Draw` (which still calls `layout()` first — fine).
- **Collapsed/expand-tab** (`expandTabRect`, `drawExpandTab`): decide — a node-anchored card likely drops the left-edge collapse tab entirely (it made sense only for a docked panel). Simplest: remove the collapse affordance in the portal form (open = card, close = gone).

## 5. Tasks (each: failing test → impl → green → screenshot)
- **Task 1 — persistence spike + decision:** implement reopen-on-select; screenshot two-node retarget; if flicker, switch to `Sticky` flag. Gate: user-approved design from §2.
- **Task 2 — compact-card layout:** re-apply §3; unit-test `cardH`/anchor via a Game-driven test asserting the panel rect is near the node and sized to content (< full pane when content is short).
- **Task 3 — portal migration:** §4; remove GridTree layer + dispatcher reg; open via portal; discipline tests (`grid_layer_zorder_test.go`, `zone_input_isolation_discipline_test.go`).
- **Task 4 — input parity:** functional tests that a real click on a sidebar section/close/increment through `Game.Update` still works via the portal path (mirror `settings_overlay_modal_close_test.go`'s `makeHoldTap` pattern); scroll + deferred-tap still work.
- **Task 5 — visual verification:** scenes `node_sidebar_open`, `node_sidebar_all_expanded`, `node_sidebar_groups`, desktop + `-mobile`; confirm the card sits at the node, content fully visible (paints over the drum pane), scrolls when tall, and mobile is sane.

## 6. Risks
1. **Input routing change (dispatcher → portal hit areas).** The sidebar's `HandleInput` is gesture-heavy (deferred tap, scroll commitment). The portal routes via `compHitHandler` OnPress/OnDrag/OnRelease → `comp.HandleInput`. Verify the gesture state machine survives; this is the top risk.
2. **Persistence flicker** (reopen-on-select). Mitigation: `Sticky` flag.
3. **Node moves while open** — the card re-anchors each `layout()` (per-frame), so it tracks a dragged node; verify it doesn't fight a node-drag gesture.
4. **Mobile** — a full-content card on a phone may exceed the viewport; the helper clamps + the card scrolls, but verify.
