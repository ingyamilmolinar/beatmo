# Node Groups: Marquee Multi-Select + Group Logic — Design

**Date:** 2026-07-09
**Status:** Approved by user (sections 1–3 approved interactively; API layer added at user request)
**Branch context:** `add-core-node-types`

## Summary

Users can Shift+drag a rubber-band rectangle over the grid pane to select multiple
nodes into a **persistent, named node group**. On release, a **GroupMenu** (modeled
on the existing node sidebar) opens, offering:

1. **Batch edits** — one-shot pitch/volume/duration offsets applied to every member.
2. **One ongoing rule** — "change *param* by *delta* every *N* rounds", evaluated
   non-destructively at trigger time.

Groups serialize in `beatmo.json`, are undoable, and their rules run identically on
desktop and browser (predictor/sequencer parity). Group creation/editing is
desktop-first in v1; on mobile, groups load, render, and their rules run, but the
marquee/menu entry points are deferred.

## Decisions made with the user

| Question | Decision |
|---|---|
| Group nature | Persistent named groups (saved in project JSON, re-editable) |
| Gesture | Shift+drag from empty grid space (pan already suppressed under Shift; link-drag only arms on a node — the gesture slot is free) |
| Menu scope | Batch edits + exactly one ongoing rule shape in v1 |
| Rule model | Computed at trigger time (non-destructive overlay; nodes keep base params) |
| Mobile | Desktop-first; creation/editing UI deferred, rules/rendering work everywhere |
| Overlap | A node may belong to **multiple groups**; rules stack |
| Architecture | Approach A: first-class model groups + one shared pure evaluator |
| API layer | Model-level CRUD APIs for groups & rules, well-tested; UI is a thin consumer |
| Extra requirements | Moving a node preserves its group memberships; trigger evaluation must be efficient and composable |

## 1. Data model (core/model)

Groups live on `model.Graph`, beside `Nodes`/`Edges`. Because the undo system
snapshots the exported document bytes, putting groups in export/import makes them
undoable with no extra journal work.

```go
type GroupID int

type NodeGroup struct {
    ID      GroupID
    Name    string      // default "Group N"
    NodeIDs []NodeID    // unique; a node may appear in several groups
    Rules   []GroupRule // v1 UI edits at most one; slice for forward-compat
}

type GroupParam string // "pitch" | "volume" | "duration"

type GroupRule struct {
    Param    GroupParam
    Delta    float64 // per-step change: semitones (pitch) or multiplier offset (vol/dur)
    EveryN   int     // one step every N rounds; N >= 1
    Min, Max float64 // clamp on the accumulated effect (defaults per param)
}
```

Graph additions: `Groups map[GroupID]*NodeGroup`, `nextGroupID GroupID`, and an
`onGroupChanged(GroupID)` callback (sibling of `onNodeChanged`).

### Rule semantics

At round `r` (0-based from the row's loop start): `steps = r / EveryN`.

- **pitch**: `Δ = clamp(Delta × steps, Min, Max)`; default clamp ±24 semitones
  (existing node pitch range).
- **volume / duration**: `mul = clamp(1 + Delta × steps, Min, Max)`; linear ramp on
  the multiplier. Defaults: volume [0, 4], duration [0.1, 4].

### Stacking (multi-group membership)

Across all groups containing a node: pitch deltas **sum**; volume/duration
multipliers **multiply**; then one final clamp to the engine's legal ranges.
Additive/multiplicative composition is order-independent — no precedence rules.

### Round definition

`round = max(0, absIdx − loopStart) / loopLen` per row (indices before the loop
start count as round 0) — the same math the existing
mute-node `every_n_triggers` cycle logic uses (`predictor_logic.go:98-104`,
`game_trigger_logic.go:60-68`). A row with no loop has `round = 0` forever, so
rules are inert there (documented behavior, not an error). A group may span nodes
on different rows; each trigger uses its own row's round.

### Membership lifecycle

- Membership keys on the stable `NodeID`. **Moving a node preserves membership**
  (requirement; guarded by a test that moves a grouped node and asserts membership
  and rule effect survive).
- Deleting a node removes it from every group; a group that empties is
  auto-deleted (its menu closes if open).
- The marquee always creates a *new* group; overlaps are fine.

## 2. Group API (core/model) — the single mutation chokepoint

The UI (GroupMenu, marquee release, sidebar chips), JS exports, and import all go
through this API. No caller mutates `Graph.Groups` directly.

```go
// CRUD — each mutation normalizes/validates, updates the reverse index,
// and fires onGroupChanged(id).
func (g *Graph) CreateGroup(name string, nodeIDs []NodeID) (GroupID, error)
func (g *Graph) DeleteGroup(id GroupID) error
func (g *Graph) RenameGroup(id GroupID, name string) error
func (g *Graph) SetGroupNodes(id GroupID, nodeIDs []NodeID) error   // replace membership; empty set = error (use DeleteGroup)
func (g *Graph) AddNodeToGroup(id GroupID, n NodeID) error
func (g *Graph) RemoveNodeFromGroup(id GroupID, n NodeID) error     // empties ⇒ auto-delete

// Rules
func (g *Graph) SetGroupRules(id GroupID, rules []GroupRule) error  // replace; v1 UI passes 0 or 1
func (g *Graph) ClearGroupRules(id GroupID) error

// Reads
func (g *Graph) Group(id GroupID) (NodeGroup, bool)   // copy, not aliasing internal state
func (g *Graph) AllGroups() []NodeGroup               // stable order (by ID)
func (g *Graph) GroupsForNode(n NodeID) []GroupID     // via reverse index
```

Validation errors (typed sentinel errors): unknown group ID; empty/duplicate node
set; unknown node IDs (dropped on import, rejected from API calls); unknown
`Param`; `EveryN < 1`; `Min > Max`. Deltas/min/max clamp to the per-param defaults.

Node-delete cleanup hooks into the existing node-removal path
(`game_graph_nodes.go` → graph delete) by calling `RemoveNodeFromGroup` for each
affected group, so the invariant "no dangling NodeIDs" holds at the API layer.

**Testing (dedicated, model-level):** table tests per API function — happy path,
each validation error, empties-auto-delete, reverse-index consistency after every
mutation, copy-semantics of reads, callback firing exactly once per mutation.

## 3. Evaluation — pure, shared, parity-safe

One pure function in `core/model`:

```go
// GroupIndex is a precomputed snapshot: reverse map NodeID→[]*rules.
// Rebuilt only on group mutation (onGroupChanged), never per trigger.
func BuildGroupIndex(groups map[GroupID]*NodeGroup) *GroupIndex

// GroupEffect returns (pitchDelta, volMul, durMul) for nodeID at round.
// Pure; O(number of rules affecting nodeID).
func GroupEffect(idx *GroupIndex, nodeID NodeID, round int) (float64, float64, float64)
```

**Integration point (corrected during planning):** group rules never change
*whether* a node fires — only vol/pitch/dur — and the predictor computes only
trigger booleans (`evalAudible` carries no params). Effective vol/pitch/dur are
composed in exactly **one** existing helper: `evalNodeParamsOnly`
(`internal/ui/game_trigger_logic.go:127`), called by both the audio scheduler
(`game_sequencer_schedule.go:323`) and the highlight/preview path
(`game_tick_highlight.go:155`), each under `seqMu`. `GroupEffect` hooks in
there, passing the row's round from `(max(0, absIdx − loopStart)) / loopLen`
(loop state already lives on `Game`: `isLoopByRow` / `loopStartByRow` /
`beatInfosByRow`). Browser parity is automatic — WASM runs the same Go code. Live rule edits take effect from the next re-predicted trigger via the
existing in-flight-drop + parity-generation-bump mechanism (matches the
established "param edits defer to next trigger" convention).

**Efficiency & composability (user requirement):** the `GroupIndex` snapshot makes
per-trigger cost a single map lookup plus a few arithmetic ops; purity makes the
evaluator table-testable and composable (stacking is just fold over the node's
rule list). The predictor's background goroutine reads an immutable snapshot —
rebuilds swap the pointer, no locking of live group state during walks.

## 4. UI/UX

### Marquee gesture (desktop)

- New `marqueeDrag{active bool; startX, startY int}` state handled in
  `handleEditor` (`game_input_editor.go`), structurally mirroring `dragLink`.
- Arm on Shift+press over empty grid space (link-drag claims Shift+press **on** a
  node; pan is suppressed under Shift — verified in `game_update.go:585` and
  `game_input_drag.go:33-38`). Require > ~4 px movement so Shift+click stays a
  no-op. Add `&& !g.marquee.active` to `panOK` as belt-and-suspenders.
- Draw: translucent accent-tinted rect + border, theme tokens only (token
  ratchet), new grid layer at GZ ≈ 46 (below long-press popup at 50), registered
  in `registerGridTree` (`grid_pane_layers.go`).
- Release: hit-test every node whose screen rect (`nodeScreenRect`) intersects the
  marquee rect. ≥1 node → `CreateGroup` + emit hook + open GroupMenu anchored at
  the release point (clamped on-screen, like the long-press popup). 0 nodes →
  dissolve silently.

### GroupMenu

New type cloned from the `NodeSidebar` pattern (`game_node_sidebar.go`): same
`mk(id,label,onClick)` button builder, `enqueueUI` deferral, draw-only grid layer
at GZ 62 (above sidebar 60), `InputHandler` in `g.inputDispatcher` at ZIndex 210
(above sidebar's 200). Sections:

- **Header**: group name (default "Group N", read-only in v1 — `RenameGroup` API
  exists but no text input in the menu), member count, Delete group, Close.
- **Batch edits** (one-shot): Vol ±, Pitch ±1 semitone, Dur ±0.1 for all members —
  loop of `SetNodeParams` + `emitNodeParamsChanged` per member inside one
  `beginUndoGroup("group edit")` / `endUndoGroup()` so each tap = one undo step.
- **Rule editor**: param dropdown (pitch/volume/duration, sidebar logic-dropdown
  idiom), Delta stepper, Every-N-rounds stepper, rule on/off. `PlainEnglish`
  glosses on labels.

All mutations go through the §2 API; the menu holds only a `GroupID` and reads
back via `Group(id)` each frame.

### Re-opening a group

The node sidebar gains a "Groups" section when the selected node belongs to any:
one chip per group; tapping a chip closes the sidebar and opens the GroupMenu for
that group. No new global browser UI in v1.

### Member rendering

Member nodes get a tinted ring in `drawGridNodes` (same seam as the selected-node
ring, `grid_pane_draw.go:701`): faint ring always (discoverability), full-strength
ring for the open GroupMenu's members. Colors via theme tokens.

### Mobile (v1)

Groups load, render rings, and rules run. Creation/editing entry points are
desktop-only; a select-mode button is a follow-up.

## 5. Persistence

Top-level `groups` array in `beatmo.json` (`omitempty`; no version bump — unknown
keys are tolerated in both directions):

```json
"groups": [
  {"id": 1, "name": "Group 1", "node_ids": [3, 5, 9],
   "rules": [{"param": "pitch", "delta": 2, "every_n": 4, "min": -24, "max": 24}]}
]
```

Import (replace-not-merge, matching instruments): unknown `node_ids` dropped;
groups that end up empty dropped; `every_n` floored to 1; delta/min/max clamped
(`clampF64` discipline); malformed rule params rejected per-rule, not per-file.

## 6. Undo, hooks, logging

Three new `hooks.Kind`s, following the "Adding a new hooks.Kind" checklist
(KindAll entry, ActionRegistry entry, eventlogger formatter, NumNonVerbose bump):

| Kind | Tag | Undoable | Emit site |
|---|---|---|---|
| `EventGroupCreated` | `group` | yes | marquee release / JS export |
| `EventGroupChanged` | `group` | yes | rename, membership, rule edits |
| `EventGroupDeleted` | `group` | yes | delete button, auto-delete on empty |

Batch edits reuse `EventNodeParamsChanged` per member inside one undo group.
Groups serialize in the document, so the snapshot journal restores them on
undo/redo automatically.

## 7. JS bridge (charter-compliant)

Minimal exports for tests/debug — `dumpGroups()`, `createNodeGroup(ids…)`,
`setGroupRule(groupId, param, delta, everyN)`, `deleteGroup(groupId)` — thin
wrappers over the §2 API, registered in the `wasm_bridge_smoke.browser.test.js`
catalogue (existence + callable). No new dedicated JS test file; logic
correctness stays in Go.

## 8. Testing

- **Model API** (§2): per-function table tests — CRUD happy paths, every
  validation error, auto-delete on empty, reverse-index consistency, read-copy
  semantics, callback firing.
- **Evaluator** (§3): table tests for `GroupEffect` — steps math, clamps,
  multi-group stacking (sum/multiply), round 0 / non-loop inertness, EveryN edge
  cases.
- **Functional UI** (real `Game.Update`, no isolation — house convention):
  synthetic Shift+drag creates group and opens menu; 0-node marquee no-op;
  Shift+click no-op; batch edit applies to all members and undoes as one step;
  rule edit via menu; node-delete auto-cleanup; **node-move preserves membership
  and rule effect**; sidebar chip reopens menu; GroupMenu input isolation
  (discipline suite covers the catch-all).
- **Parity**: extend the `parity_node_logic_audio_test.go` pattern with a
  group-rule case (predictor vs. audible locked); a round-boundary test asserting
  the pitch step lands exactly at `loopStart` of round N.
- **Import/export**: round-trip with groups; malformed-input clamping in
  `import_validation_test.go` style; legacy file without `groups` loads clean.
- **Auto-enforced by existing discipline suites**: hooks registry tests
  (tag/classification/NumNonVerbose), token ratchet, zone-input isolation.

## 9. Error handling

- Empty marquee → no-op; tiny (<4 px) Shift+drag → no-op.
- Group emptied by node deletions → auto-delete; its menu closes if open.
- Non-loop rows → rules inert (round stays 0).
- All imported numerics clamped; per-rule rejection, never file rejection.
- GroupMenu anchor clamped on-screen.
- API returns typed errors; UI surfaces nothing for programmer-impossible states
  (menu only ever holds IDs it created or verified).

## 10. Out of scope (v1)

- Mobile creation/editing UI (select-mode button) — follow-up.
- Multiple rules per group in the UI (schema supports it; UI edits one).
- Rule shapes beyond linear delta-every-N (wrap/ping-pong, probability ramps).
- Group-of-groups, naming UI beyond default names (rename API exists; a text
  input in the menu is a stretch goal, not required for v1).

## Amendment: Save/cancel mechanism (user request, 2026-07-09)

Marquee-created groups are **provisional** until the user presses the new
**Save** button in the GroupMenu header. Dismissing the menu any other way
(close button, click outside on the grid, clicking a node, import, the group
emptying) silently deletes the provisional group — no orphans, no undo-history
pollution (nothing is emitted until Save; Save emits the single undoable
`group created` event and closes the menu). While provisional, rule edits emit
nothing; batch node edits still record their own node-param undo step and
survive a discard. Opening a second marquee discards the previous provisional
group. Reopening a committed group via a sidebar chip is non-provisional —
closing it never deletes. The JS export `createNodeGroup` commits immediately
(programmatic path).

## Implementation deviations (as shipped, 2026-07-09)

- §4: the rule-param control is a cycle-button, not a dropdown (functionally
  equivalent; final review accepted). `PlainEnglish` glosses were omitted from
  the menu (YAGNI call at implementation time).
- §5: imported rule `min`/`max` are not clamped at import; out-of-range values
  are bounded at evaluation time by the final legal-range clamp in
  `evalNodeParamsOnly` (audible effect identical).
- §6: auto-delete-on-empty (last member node deleted) does not emit
  `EventGroupDeleted` — only explicit deletes (menu button, JS export) do. The
  deletion is still undoable (it rides the node-delete snapshot); only the
  narrative/JSONL stream omits a line. Candidate follow-up.
- Additions beyond spec, found necessary during review: marquee cancels when a
  release is consumed by an overlay/out-of-pane/multi-touch/import; plain grid
  clicks close the GroupMenu (mutually exclusive with sidebar);
  `Graph.ResetGroups()` on import so group IDs are deterministic across
  import/undo/redo (byte-identical undo documents); undo action-case +
  localized-label registrations for the three group kinds.
