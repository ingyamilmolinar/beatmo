# Move-Node Circuit Healing (Extraction Semantics)

**Date:** 2026-07-18
**Status:** Approved
**Branch:** add-core-node-types
**Builds on:** `2026-07-17-node-quick-action-modes-design.md` (edge-only move placement, gridModeManager, stitch-on-move)

## Summary

Moving a node out of a chain must keep the circuit connected: `Np → N → Nn`
becomes `Np → Nn` (the chain "heals" behind the extracted node), and N
reconnects by splicing into the destination edge as already shipped. When
healing is impossible (corner node with perpendicular neighbors, junctions),
the interactive flow asks the user to confirm breaking the circuit before
committing. TDD with a robust test matrix.

## Semantics

### Heal plan — `moveHealPlan(n) healPlan` (pure classification, no mutation)

Classifies N's current edges (edges are DIRECTED `[2]NodeID{from,to}`):

| Topology | Classification | Behavior |
|---|---|---|
| Exactly one incoming `Np→N` AND one outgoing `N→Nn`, `Np != Nn`, and `Np`,`Nn` orthogonal (`Np.I==Nn.I \|\| Np.J==Nn.J`) | **healable pass-through** | heal edge `Np→Nn` (direction preserved) |
| ≤ 1 edge total (endpoint / isolated) | **silent** | nothing to heal; extract + re-splice |
| Anything else (perpendicular in/out pair = corner; ≥3 edges = junction; two ins; two outs) | **unhealable** | interactive: confirm dialog; programmatic: silent break |

Heal details:
- If other nodes lie strictly on the healed `Np→Nn` span, the heal edge is
  split through each of them (`stitchEdgesAt` per intermediate node, inside
  the same undo group) so the chain remains fully connected node-to-node.
- If `Np→Nn` already exists, the heal is a no-op (`addEdgeNoRefresh` dedupes).

### `moveNode` — extraction (replaces re-anchoring)

For every caller (interactive AND programmatic `moveNodeGrid`):
1. Remove ALL of N's old edges (the previous "re-add still-orthogonal edges
   at the new position" behavior is retired).
2. Apply the heal edge when the plan is healable (+ stitch through
   intermediates on the healed span).
3. Move N; splice N into the destination edge (existing stitch-on-move).
4. All inside the single `"move node"` undo group; one undo restores the
   original topology exactly (old edges back, heal edge gone).

Moving a node along its own chain (destination on the healed span) works by
construction: extract → heal `Np→Nn` → N splices into the healed edge →
`Np→N→Nn` again at the new cell.

### Interactive placement — confirm before breaking

In `handleArmedModeTap`'s move case, after the existing edge-only validity
check (`GuideEdge`):
- Plan healable or silent → commit immediately (unchanged UX).
- Plan unhealable → open a **"Break the circuit?"** confirmation portal
  (same `OverlayPortal` pattern as the paste-overwrite confirm, so
  `popup.opened`/`popup.closed` events are emitted by the portal chokepoint):
  - **Break** → `moveNode` commits (circuit breaks, still one undo step);
    move mode commits.
  - **Cancel** → portal closes; **move mode STAYS ARMED** (blink + ghost
    live; the user may pick another destination or ESC). NO
    `mode.canceled` event is emitted on dialog-Cancel.
- The pending destination `(i,j)` and subject are snapshotted in the portal
  state (mirroring `pastePendingState`) so graph edits while the dialog is
  open cannot misplace the node. If the subject node is deleted while the
  dialog is open, the dialog closes and the mode cancels (existing
  `subject-deleted` prune rung).

### Programmatic `moveNodeGrid`

Applies extraction + heal identically; when unhealable it breaks silently
(no dialog). Return value semantics unchanged (false only when destination
occupied / node missing).

## Events

- Heal/extraction edge changes emit their normal `edge.added` /
  `edge.deleted` events inside the move undo group.
- Confirm portal emits `popup.opened` / `popup.closed` (portal chokepoint).
- `node.moved` on commit; `mode.armed`/`mode.canceled` unchanged
  (dialog-Cancel emits neither — the mode stays armed).

## Testing (TDD — write RED first)

Functional tests drive real handlers (`handleTapInGrid` /
`handleArmedModeTap` / portal buttons), per project convention.

**Heal matrix** (`grid_mode_flow_test.go` / `move_node_test.go`):
1. Straight chain A→N→B (collinear): move N to another circuit's edge point
   → A→B exists (direction preserved), A→N and N→B gone, N spliced at
   destination; total connectivity preserved.
2. Direction: B→N→A heals B→A (assert the directed pair; A→B absent).
3. Heal span with intermediate node M on the A–B line → healed chain
   A→M→B (stitched), not an overlapping A→B.
4. Heal dedupe: A→B edge already exists → move commits, no duplicate edge.
5. Move along own chain: A(0,0)→N(0,4)→B(0,8), move N to (0,6) → A→N→B
   again with N at (0,6); no leftover A→B unsplit edge; single undo restores.
6. Endpoint: A→N only; move N → edge dropped silently, no dialog, N
   spliced at destination.
7. Isolated node (0 edges): moves silently (existing behavior).

**Break-confirm matrix:**
8. Corner node in a square circuit (perpendicular Np/Nn): tap valid edge
   destination → dialog opens, graph UNTOUCHED while open, mode still armed.
9. Dialog **Break** → move commits: old edges gone, no heal edge, N spliced
   at destination; ONE undo restores the entire original square.
10. Dialog **Cancel** → graph untouched, mode STILL ARMED (blink on), a
    second tap on a different valid destination retries the flow; ESC still
    aborts. No `mode.canceled` emitted by the Cancel itself.
11. Junction (3 edges): dialog opens (unhealable classification).
12. Double-in (X→N←Y) and double-out (X←N→Y): dialog opens.

**Programmatic:**
13. `moveNodeGrid` on a pass-through heals silently; on a corner breaks
    silently (no dialog state set).

**Events:**
14. Dialog flow emits `popup.opened`/`popup.closed`; Cancel emits no
    `mode.canceled`; Break emits `node.moved`.

**Legacy updates:** `TestMoveNodePreservesEdges` and
`TestMoveNodeDropsNonOrthogonalEdges` are REWRITTEN for extraction semantics
(re-anchoring retired — intended behavior change);
`TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently` becomes the
corner/junction confirm cases; `TestMoveMode_SingleUndoStep` extended to
cover heal reversal. Browser suite: no new JS tests required (logic is
Go-owned per the JS Test Charter); existing gesture tests must stay green
(their mover node is edge-free = silent class).

## Out of scope

- Best-effort pairwise healing of junctions.
- Healing on node DELETE (delete keeps its current semantics).
- Changes to copy/connect modes.
