# Grid Guide / Crosshair — Design

**Date:** 2026-07-13
**Status:** Approved (brainstorm complete)

## Problem

In large circuits spanning many grid units, adding, moving, or removing nodes
requires zooming far in and pixel-hunting for the right cell. The user has no
visible feedback about beat/subdivision position or about where a new node
would land relative to its two nearest neighbors along an edge. Most of the
time the user wants to place a node at a simple fraction of the span between
two adjacent nodes (1/2, 1/3, 2/3, 1/4, …), and today nothing helps them find
those positions.

## Solution overview

An always-on **grid guide**: a crosshair that follows the cursor (desktop) or
finger (mobile aiming gesture) through the grid pane, snapping to a **guide
target** — the nearest node, the nearest meaningful cell on the nearest edge,
or the free grid cell. The guide shows beat:subdivision coordinates and, when
on an edge, the exact fraction of the span between the edge's two endpoint
nodes. A ghost node previews the commit cell before any click/release.
Committing calls the existing `tryAddNode`; the existing auto-stitch
(`stitchEdgesAt`, `game_graph_nodes.go:167`) already splits the edge through
the new node, so no new graph-mutation code is needed.

## Core API — one engine, one file

`src/go/internal/ui/grid_guide.go`:

```go
type GuideKind int

const (
    GuideCell GuideKind = iota // free cell — no circuit element in radius
    GuideNode                  // snapped to an existing node
    GuideEdge                  // snapped to a cell on an existing edge
)

type GuideTarget struct {
    Kind     GuideKind
    I, J     int     // the commit cell (always valid)
    Node     *uiNode // set when Kind == GuideNode
    Edge     *uiEdge // set when Kind == GuideEdge
    Num, Den int     // reduced fraction of the A→B span (GuideEdge only)
}

// guideTargetAt resolves the guide target for a screen-space cursor position.
func (g *Game) guideTargetAt(mx, my int) GuideTarget
```

### Resolution rules

1. **Screen→world**: invert the camera exactly as existing call sites do
   (`(screen − Offset) / Scale`, y adjusted by `gridTopOffset()`).
2. **One snap radius**, defined in screen pixels via a density token, so its
   world footprint is `radiusPx / cam.Scale`. All snapping behavior is
   therefore zoom-proportional: zoomed out the radius spans many cells
   (coarse, musical snapping); zoomed in it spans less than one cell (exact
   per-unit placement).
3. **Precedence**: nearest node within radius → `GuideNode`; else nearest
   edge within radius (perpendicular distance to the axis-aligned segment,
   clamped) → `GuideEdge`; else `GuideCell` from `Grid.Snap`.
4. **Smallest-denominator rule** (the only "special fraction" logic): among
   the integer cells of the edge that lie within the snap radius of the
   cursor's along-axis position, choose the cell whose reduced fraction
   `off/span` has the smallest denominator; ties go to the nearest cell.
   This makes 1/2 beat 1/3, 1/3 beat 5/12, etc., with no marker-set
   configuration. Edge cells are strictly interior (`0 < off < span`);
   endpoints resolve as `GuideNode`.
5. **Tie-breaking across edges**: smallest perpendicular distance, then
   smaller span. Span-1 edges have no interior cells → node targets only.
   If the chosen cell is already occupied by a node (crossing circuit),
   resolve as `GuideNode` for that node.
6. Edges are orthogonal-only (enforced by `addEdgeNoRefresh`,
   `game_graph_edges.go:10`), so all geometry is axis-aligned.

## Rendering

One new grid layer **`GZGuide` = 35** (between `GZPulses` 30 and
`GZCoordBadge` 40, clip=true), registered in `registerGridTree`
(`grid_pane_layers.go`) with a `drawGridGuide` method following the existing
`drawGrid*` pattern (reads `g.gridDrawCtx` / `g.gridDrawScreen`):

- **Crosshair**: one vertical + one horizontal line through the *snapped*
  target position, spanning the grid pane. Screen-constant 1 px stroke,
  dim accent token (`WithAlpha(TokenAccentDim, …)`).
- **Active edge emphasis** (GuideEdge only): brighten the snapped edge and
  draw small tick marks at the interior cells whose reduced fraction has a
  small denominator (the same fractions the smallest-denominator rule
  prefers). Ticks are screen-constant size.
- **Ghost node**: rendered at the commit cell before committing, using the
  move-mode ghost visual (`genColorVizGlow`, see `drawGridMoveMode`,
  `grid_pane_draw.go:1239-1251`), **world-scaled like a real node** — it
  previews the actual node, so it scales with zoom while all other guide
  chrome stays screen-constant. Nothing in the guide grows when zooming in.

**Readout**: extend the existing cursor-label layer (`GZCursorLabel` 70,
unclipped, `drawGridCursorLabel`, `grid_pane_draw.go:1281`) — it already
formats beat:subdivision via `Grid.BeatSubdivision`. Add a fraction chip when
the target is an edge (e.g. `5/12` between the endpoint labels), highlighted
when the denominator is small. Enable the label on mobile during the aiming
gesture, offset above the finger so it isn't covered.

**Tokens**: existing accessors only (`TokenAccentDim`, grid-line colors,
`genColorVizGlow`, viz-pill tokens, `WithAlpha` buckets). No new color
literals (token-discipline ratchet untouched). Zero per-frame allocations
targeted for the crosshair path (no `fmt.Sprintf` beyond what the existing
cursor label already does).

> Implementation note (2026-07-14): the fraction chip renders as plain appended label text; the "highlighted when the denominator is small" emphasis was not implemented (deviation accepted at final review).

## Input integration

### Desktop

- Guide is always on while the cursor is inside the grid pane, gated the same
  way as `Profile().ShowCursorLabel`.
- The add-node release path in `game_input_editor.go` resolves through
  `guideTargetAt` instead of raw `clickI/clickJ`:
  - `GuideEdge` / `GuideCell` → `tryAddNode(t.I, t.J, …)`; auto-stitch
    splits the edge when the cell lies on one.
  - `GuideNode` → existing select behavior, unchanged.
- **Move-mode** ghost placement resolves through the same engine, so moving a
  node also snaps to fraction cells.
- The guide stays visible during link-drag and marquee but placement is
  suppressed (those gestures keep their existing semantics).

### Mobile

- **Touch-down** in the grid shows the guide immediately (crosshair + readout
  pill offset above the finger).
- If the touch starts **within snap radius of a node or edge**, the drag is
  an *aiming gesture*: pan is suppressed for that touch, the guide tracks the
  finger, and **release commits** the node at the target cell (release on a
  `GuideNode` target = existing tap-on-node behavior).
- Drags starting in open space pan the camera as today. Two-finger pan always
  pans. Long-press (stationary) keeps its menu — aiming requires movement
  past the slop threshold. Releasing outside the grid pane cancels.

## Edge cases

- Multiple edges in radius → nearest perpendicular distance, then smaller span.
- Span-1 edges: no interior cells; endpoints only.
- Chosen cell occupied by a node from a crossing circuit → `GuideNode`.
- Placement during playback: unchanged from today (the known circuit-edit
  audio disruption is a separate, pre-existing issue).
- Subdiv changes rescale node coordinates (`game_subdivisions.go:64-70`);
  the guide reads live state each frame, so no stale-state handling needed.

## Testing

Per repo discipline — functional tests driving real `Game.Update`, fast
`-tags test` path:

1. **Snap engine (table-driven)**: precedence node > edge > cell; fraction
   reduction; smallest-denominator rule (span 12: cursor near 1/3 point
   picks 4/12→1/3 over 5/12; span 8: no thirds exist, 1/2 and 1/4 win);
   radius scaling with `cam.Scale` (zoomed in → exact cell; zoomed out →
   simplest fraction); tie-breaking; span-1 and occupied-cell cases.
2. **Desktop functional**: simulate mouse move + click through `Game.Update`
   on a two-node circuit; assert the node is created at the fraction cell,
   `stitchEdgesAt` rewired A→new→B, and undo restores the original edge.
3. **Mobile functional**: injected touch drag starting near an edge →
   release → node placed at target; drag starting in open space → camera
   panned, no node created; long-press unaffected.
4. **Rendering**: crosshair/ghost pixel presence via the ebitenstub
   interception pattern (`audio_panel_render_pixels_test.go` style); new
   layer entry added to `grid_layer_zorder_test.go`.
5. **Discipline**: no new color literals (token ratchet); gofmt; z-order
   table updated.

## Out of scope

- Triplet subdivisions in the grid model (1/3 positions exist only when the
  inter-node span is divisible by 3 — the guide is honest about what is
  representable).
- Edge hit-testing for selection/deletion of edges themselves.
- Any change to graph mutation, undo grouping, or traversal semantics.
