# Node Quick-Action Modes — hold → slide → arm → tap-to-place

**Date:** 2026-07-17
**Status:** Approved
**Branch:** add-core-node-types

## Summary

Extend the node long-press quick-action popup into a unified, extensible "armed
mode" interface, reachable from both touch and desktop mouse. The user
long-presses (touch) or click-and-holds (mouse) a node to open the popup, slides
the same finger/button onto an option, and releases to arm that mode. While a
mode is armed the user can pan/zoom freely; the next tap places (move/paste) or
the mode cancels on an invalid target. The source node blinks and a ghost node
previews the placement. Every UX step emits an event to the central `hooks.Bus`
store.

Most machinery already exists: the long-press popup with slide-to-select
(`game_longpress_popup.go`), move mode with ghost + pulsing outline
(`drawGridMoveMode`, `grid_pane_draw.go:1230`), `moveNode`
(`game_graph_nodes.go:377`), the node clipboard (`node_clipboard.go`), and the
`EventNodeMoved` / `EventNodeCopied` / `EventNodePasted` hook kinds. This change
is additive and surgical.

## Interaction model

1. **Open the popup**
   - Touch: long-press a node (existing, `GestureLongPress`, 500 ms / ≤10 px).
   - Mouse (NEW): click-and-hold a node for `longPressThresholdMS` (500 ms)
     while moving ≤ `tapMaxMovePx` (10 px) opens the same popup. Movement beyond
     the threshold cancels the hold (becomes camera pan as today). The pending
     click is cancelled so the hold never double-fires as a tap/select.
   - Popup options: **Move / Copy / Connect / Delete** (Copy is NEW).

2. **Arm a mode** (existing slide-to-select mechanic, `updateLongPressPopup`)
   - Slide onto an option and release → activates it. Release elsewhere →
     dismiss. Tap-and-release on a node (<500 ms) still opens the sidebar menu —
     untouched.
   - **Move** → move mode armed (`moveMode`, `movingNode`). Source node
     **blinks** (body alpha oscillation) in addition to the existing pulsing
     outline. Ghost node renders at the snapped cursor/touch cell.
   - **Copy** (NEW) → node snapshot to clipboard
     (`snapshotNodeToClipboard`) + copy mode armed. Ghost of the would-be paste
     renders at the cursor cell.
   - **Connect / Delete** → unchanged.

3. **Navigate while armed**: pan/zoom work exactly as today (armed modes
   intercept taps only, never drags).

4. **Place (next tap)**
   - **Move mode** — valid target = tapped cell lies on an existing edge
     segment (`guideTargetAt` → `GuideEdge`) and is unoccupied.
     - Valid: commit **immediately** via `moveNode` — no edge-loss confirm
       dialog (retired for interactive moves; non-orthogonal edges drop
       silently, still a single undo step). Additionally the target edge is
       **stitched** through the node (`stitchEdgesAt` semantics: edge A–B →
       A–node–B), so the node joins the traversal path where dropped. The node
       keeps its own still-orthogonal edges (existing `moveNode` rule).
     - Invalid (empty cell, occupied cell, tap on a node, off-grid): move mode
       **cancels**; node stays put.
     - Rule applies to move mode from ALL interactive entry points (long-press
       popup AND sidebar Move button). The `moveNodeGrid` JS export stays
       unrestricted (programmatic/tests).
   - **Copy mode** — existing paste semantics, NOT edge-only: any free cell
     pastes (`pasteNodeFrom`, which stitches when landing on an edge); occupied
     cell raises the existing overwrite confirm; paste commits and copy mode
     disarms. A tap outside the grid pane (or on no resolvable cell) cancels
     copy mode. Keyboard Ctrl+C/V flow untouched.

## Visual feedback

- **Blink**: while move mode is armed, the source node's body alpha oscillates
  (blink) on top of the existing pulsing `genColorVizGlow` outline in
  `drawGridMoveMode`. Design-token compliant (no new colors; alpha buckets).
- **Ghost**: the existing ghost placement indicator gains a validity state:
  - Valid target (on-edge & free for move; free cell for copy): glow color, as
    today.
  - Invalid target: outline-only rendering at a faint alpha bucket
    (`WithAlpha(genColorVizGlow, AlphaSubtle)`-style; no new colors).
- Copy mode reuses the same ghost drawing at the snapped cell.

## Decoupling guarantees

- Tap-release (<500 ms, ≤10 px) on a node → sidebar menu. Path untouched.
- The popup keeps its exclusive input intercept while visible
  (`game_update.go:366`); no `inputDispatcher` changes.
- The mouse-hold detector cancels `pendingClick` so hold never also selects.
- Armed modes claim taps only; single-finger drags remain camera pan; the
  marquee/link-drag gates are untouched.

## Centralized mode domain (`gridModeManager`)

All armed-mode state and transitions live in ONE place — a small manager owned
by `Game` — instead of today's scattered booleans (`moveMode`, `connectMode`,
`movingNode`, per-mode cancel funcs). Deliberately minimal; no new goroutines,
no hooks.Bus round-trip for intra-frame state.

```go
type GridModeKind int // GridModeNone, GridModeMove, GridModeCopy, GridModeConnect

type gridModeManager struct {
    kind      GridModeKind
    subject   *uiNode            // moving node / copy source / connect source
    observers []GridModeObserver // opt-in registrations
}

// Opt-in API: any component that must react to mode transitions registers once.
type GridModeObserver interface {
    OnGridModeChanged(prev, next GridModeKind, subject *uiNode)
}
```

- **Single chokepoint**: `Enter(kind, subject)`, `Cancel()`, `Commit()` are the
  only mutation paths. Each notifies observers synchronously (same goroutine,
  called from the `Update` path — sync direct-call tier per the two-tier event
  doctrine; observers must not re-acquire `seqMu`).
- **Event emission is centralized too**: `Enter` emits `EventGridModeArmed`,
  `Cancel` emits `EventGridModeCanceled` — call sites cannot forget to log.
  `Commit` emits nothing itself (the domain action — `EventNodeMoved` /
  `EventNodePasted` — is emitted by the operation it commits).
- **Rollback is structural**: armed modes mutate nothing until commit — the
  graph is only touched inside `Commit`'s single undo group — so `Cancel` is
  always safe: clear state, notify observers, done.
- **Readers**: `Active() GridModeKind`, `Is(kind)`, `Subject()`. Draw layers
  (ghost, blink, move banner), `handleTapInGrid` branches, and pan gates read
  through these instead of loose fields. `connectMode` migrates into the
  manager; the legacy `moveMode`/`movingNode`/`connectMode` fields are removed.
- **Initial observers** (proves the opt-in API without overengineering): the
  node sidebar (closes when a mode arms), and the move-mode draw layer
  visibility predicate. Other components can register later; nothing is forced
  to participate.
- Discipline test: no direct writes to `gridModeManager` fields outside the
  manager's own file (structural grep test, mirroring existing dispatch
  discipline tests).

## Events (central store: hooks.Bus → eventlogger narrative + JSONL)

| UX action | Event | Notes |
|---|---|---|
| Popup opened / closed | `EventPopupOpened` / `EventPopupClosed` (existing Layer-B) | surface `node-longpress`; emitted manually from `showLongPressPopup` / `dismissLongPressPopup` (popup is not portal-based) |
| Popup option selected | `EventUITap` (existing Layer-B) | label = option name |
| Mode armed | **NEW** `EventGridModeArmed` | tag `node`, `ScopeInteraction`, non-verbose; payload names mode: move/copy/connect |
| Mode canceled (invalid tap / dismiss) | **NEW** `EventGridModeCanceled` | same shape |
| Move committed | `EventNodeMoved` (existing) | already emitted by `moveNode` |
| Copy armed / paste committed | `EventNodeCopied` / `EventNodePasted` (existing) | |

New-Kind recipe applies: `KindAll` entry, `ActionRegistry` row with lowercase
tag, `internal/eventlogger` formatter, `NumNonVerbose` bump. Guarded by
`TestEveryKindClassified`, `TestEveryActionHasLowercaseTag`,
`TestHooks_NumNonVerboseUpdated`, formatter coverage test.

## Architecture / touch points

| Concern | File(s) |
|---|---|
| Mouse-hold detection (NEW) | `game_input_editor.go` press tracking — stationary-hold timer on press-over-node; opens `showLongPressPopup`; cancels pending click |
| Popup 4th button (Copy) | `game_longpress_popup.go` — layout widens to 4 keycaps, hover key `"copy"`, draw, activation branch; i18n key `KeyCapCopy` |
| Centralized mode domain (NEW) | `grid_mode.go` — `gridModeManager` + `GridModeObserver`; absorbs `moveMode`/`movingNode`/`connectMode`; discipline test `grid_mode_discipline_test.go` |
| Copy armed mode (NEW state) | `GridModeCopy` in the manager; branch in `handleTapInGrid`; cancel resets |
| Edge-only move placement | `handleTapInGrid` move-mode branch (`game_input_touch.go:41-75`) — require `GuideEdge`, commit or `cancelMoveMode`; stitch target edge after `moveNode` (inside the same undo group) |
| Retire move confirm dialog | remove `moveConfirm` path + `drawGridMoveConfirm` layer + related state |
| Blink + ghost validity | `drawGridMoveMode` (`grid_pane_draw.go`) + copy-mode ghost |
| Events | `event_helpers.go` emitters; `internal/hooks/events.go`, `actions.go`; `internal/eventlogger` formatter |

## Error handling

- Hold interrupted by movement > threshold → no popup; normal pan/click.
- Popup open + release off-button → dismiss (existing) + `EventPopupClosed`.
- Armed mode + tap invalid target → cancel + `EventGridModeCanceled`; graph
  untouched.
- Move onto edge whose stitch would self-loop (target edge already touches the
  moving node) → stitch skips that edge (same guard as `stitchEdgesAt` uses for
  self-edges); move still commits.
- Undo: move (incl. stitch) is one undo group; paste is one step (existing).

## Testing

**Go functional tests** (drive real `Game.Update` with ebitenstub; no manual
state poking):
- Mouse-hold: opens popup at 500 ms; drag during hold cancels; short click
  still opens sidebar; hold+release-off-button dismisses.
- Slide-release arms each of the four options; Copy snapshots clipboard.
- Move placement matrix: on-edge commit (+ target edge stitched, own
  orthogonal edges kept), empty-cell cancel, occupied cancel, tap-on-node
  cancel, off-grid cancel; single undo step round-trip.
- Copy placement: free-cell paste, on-edge paste stitches, occupied →
  overwrite confirm, invalid → cancel.
- Render: blink present while armed; ghost valid vs invalid states
  (structural/pixel assertions per ebitenstub conventions — measure delta).
- Mode manager: transition matrix (None→Move→Cancel, None→Copy→Commit,
  Move armed while Connect requested, double-Enter, Cancel when None is a
  no-op); observer notification order and payloads; discipline test that no
  file outside `grid_mode.go` writes manager fields; sidebar-closes-on-arm via
  the observer registration.
- Events: subscriber sink asserts sequence
  `popup.opened → ui.tap → grid-mode armed → node.moved` (and copy/paste
  variant).
- Update existing: `TestHandleTapInGrid_MoveMode*` (EmptyTarget now cancels;
  EdgeLoss confirm tests removed), `move_node_test.go` (confirm-dialog tests
  retired; stitch assertions added), `longpress_popup*` tests (4 buttons).

**JS browser tests** (charter: real input dispatch through the canvas):
- Real-touch long-press → slide onto Move → release → pan → tap on edge point;
  verify graph state via exports (`dumpRowState`/node queries). Mobile
  viewport pass.
- Same for Copy → paste.
- Extend existing suites (`touch_gestures` / `e2e_node_click` style); no new
  logic-retesting files (JS Test Charter).

## Out of scope

- Group drag-to-move; continuous drag-the-node gesture (superseded by armed
  modes); portal-based popup rewrite; changes to keyboard clipboard flow;
  changes to `moveNodeGrid` JS export semantics.
