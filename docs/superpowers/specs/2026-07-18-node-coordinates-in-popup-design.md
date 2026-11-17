# Node coordinates in the pop-up menu — design

**Date:** 2026-07-18
**Status:** Approved for planning

## Goal

Make the main node pop-up menu (the `NodeSidebar`) show, for the selected node:

1. **Grid coordinates** `(i, j)` — where the node sits on the 2D grid.
2. **Beat + subdivision** — *when* the node fires within its circuit, user-readable.
3. **Live last/next fire** during playback — when this node last fired and when it
   fires next, updating in real time as the playhead moves.

All of this must reuse existing infrastructure (the graph traversal paths, the
Predictor, the Timeline service, and the grid subdivision math) rather than
introducing a parallel notion of "node timing". The beat/subdivision must be
**circuit-aware** — derived from the node's position in the traversal path, so it
is correct for both closed circuits (loops) and open paths.

## Non-goals

- No new traversal logic. Paths are already built by `updateBeatInfos`.
- No change to audio scheduling, the predictor's prediction rules, or the timeline.
- No per-node persisted state in the JSON format. Timing is derived, not stored.

## Key correctness principle

The existing cursor label (`grid_pane_draw.go:1315`) feeds a node's **grid-X**
(`node.I`) into `Grid.BeatSubdivision`. That is pure geometry: it only equals the
real beat position for a straight left-to-right line. A real circuit that loops or
bends makes grid-X and traversal-timing diverge.

**This feature derives beat/subdivision from the node's index in its traversal
path (`beatInfosByRow`), never from `node.I`.** Each `BeatInfo` in a path is exactly
one subdivision step (including the synthesized invisible steps for edge crossings),
so the path index *is* the absolute subdivision count from the circuit's start.

## Reused infrastructure

| Concern | Reused API | File |
|---|---|---|
| Node's row (which path contains it) | `g.nodeRows[nodeID]` | `game_struct.go`, built in `game_graph_update_beat_infos.go` |
| Per-row traversal path | `g.beatInfosByRow[row]` (`[]model.BeatInfo`) | published to predictor via `SetPaths` |
| Loop geometry | `g.isLoopByRow[row]`, `g.loopStartByRow[row]` | `game_graph_beat_index.go` (`beatInfoAtRow` loop-wrap) |
| Subdivisions per beat | `g.grid.MaxDiv()` | `grid.go` |
| "does node fire at idx" (future) | `engine.Predictor.TriggeredAt(row, idx)` | `core/engine/predictor_api.go` |
| Predictor window bounds | `Predictor.WindowStart()`, `Predictor.Horizon()` | `core/engine/predictor_api.go` |
| "did node fire at abs" (past) | `timeline.Service.Committed(row, abs) (bool, NodeType, bool)` | `internal/timeline/service_segments.go` |
| Playhead (absolute subdivisions) | `g.elapsedBeats` / `g.playheadAbsSubdiv()` | `game_seek.go` |
| Path change signature | per-row FNV path signature | `game_graph_update_beat_infos.go` |

## The math

### Static position (grid + beat/subdivision)

`BeatSlot(idx)` converts an absolute subdivision index into a **1-based** beat and a
**1-based** subdivision slot of `div`:

```
div  = Grid.MaxDiv()
beat = idx / div + 1
slot = idx % div + 1
```

Example (`div = 8`): path index `19` → `beat = 3`, `slot = 4/8`.

This is a new pure helper next to `BeatSubdivision` in `grid.go`. It is kept
**separate** from `BeatSubdivision` because that function returns a *reduced
fraction* (`1/2`, `1/4`), which is the wrong presentation for "beat and
subdivision" (we want the raw slot `4/8`).

### The node's home position

```
row, ok := g.nodeRows[nodeID]           // node's row
if !ok { off-circuit }                   // node in no path (disconnected / open dead-end)
infos := g.beatInfosByRow[row]
firstIdx, occurrences := scan infos for NodeID == nodeID
```

- `firstIdx` → `BeatSlot(firstIdx)` → the displayed `Beat B · slot/div`.
- `occurrences > 1` (self-crossing circuit) → append `(+N)` where `N = occurrences-1`.
- `!ok` → show grid coords + `off-circuit`, no beat.

### Live last/next fire (during playback)

A node in a loop fires once per cycle, and logic gates (probability, skip-every-N,
etc.) mean it does **not** fire every cycle. So last/next must consult the actual
fire decisions, not just the static position.

**Candidate indices** (cheap, arithmetic — no per-subdivision scan). The node's
firing positions repeat every loop period. From its occurrence offsets within the
loop and the loop length we generate candidate absolute indices `base + k·loopLen`
around the playhead. For a non-looping (open) path the candidates are just the
finite occurrence indices.

- **Next fire** = smallest candidate `idx >= playhead` with
  `Predictor.TriggeredAt(row, idx) == true`, bounded by `Predictor.Horizon()`. The
  predictor's window buffers are already maintained by the scheduler, so each
  `TriggeredAt` is an O(1) cached read; we sample only at candidate indices.
- **Prev fire** = largest candidate `idx < playhead` with
  `timeline.Committed(row, idx)` returning `true`. Because each `idx` in a row maps
  1:1 to one node (`beatInfoAtRow`), the timeline's `bool` *is* "did this node fire
  at idx", even for probability nodes whose past is non-deterministic. The timeline
  is the cache for the frozen past.

This is the requested split: **Timeline caches the past (prev), Predictor/scheduler
caches the future (next).**

**When stopped:** playhead is treated as position 0 (loop start). Show `next` only —
the node's first upcoming fire from the start — and omit `last` (there is no past).
When playback runs, both `last` and `next` update live.

## Caching / node-state (optimization)

A small derived value object, recomputed lazily, not per frame:

```go
type nodeTiming struct {
    homeIdx     int   // first occurrence index (static)
    occurrences int
    inCircuit   bool

    prevIdx int   // last fire (playing only)
    prevOK  bool
    nextIdx int   // next fire
    nextOK  bool
}
```

Invalidation stamp = `(nodeID, pathSignature, playheadBucket)`:

- **Path signature change** (add/remove node/edge, subdivision change, origin
  change) → recompute everything.
- **Playhead crosses the cached `nextIdx`** → recompute prev/next only. Between
  crossings the memo is returned unchanged, so the common per-frame path is a stamp
  comparison + return.

Only one sidebar is open at a time, so at most one node's timing is live. The memo
lives on `Game` (UI/render state), keyed by node id, and is exposed as node state
via a `(*Game) nodeLiveTiming(id) nodeTiming` accessor the sidebar reads. It is
derived render state, never serialized.

### Concurrency

`beatInfosByRow` / `nodeRows` are mutated under `seqMu` in `updateBeatInfos`; the
sidebar reads them during `Draw` on the UI goroutine, matching the existing pattern
(`drawHeader` already reads `g.nodeRows`). No new lock is introduced. Predictor and
timeline reads use their existing thread-safe query APIs.

## Rendering

The coordinates are an **always-visible** read-only block in the sidebar's **fixed
header band** (not the scrollable content), directly under the instrument-name
title:

```
● Kick
(5, 2) · Beat 3 · 4/8          <- static: grid + home beat/subdivision
next  Beat 3 · 4/8             <- stopped: next-from-0
last  Beat 2 · 4/8   next  Beat 6 · 4/8   <- playing: live last/next
```

- Grow the header band height to fit the extra line(s). `contentTop` already derives
  from `sidebarHeaderH`, so scrolled content shifts down automatically — one constant.
- Draw in `drawHeader` via `DrawTextStyled` at `RoleCaption` / `colTextSecondary`.
- Labels `last` / `next` are text (the arrow glyphs `←`/`→` are on the forbidden-glyph
  list). `"Beat"` stays a literal, matching the existing `formatBeatReadout`.
- Off-circuit node: `(5, 2) · off-circuit`, no beat/last/next.

## New/changed code

| Item | Location | Kind |
|---|---|---|
| `(*Grid) BeatSlot(idx) (beat, slot, div int)` | `grid.go` | new pure helper |
| `(*Game) nodeCircuitPosition(id) (idx, occurrences int, inCircuit bool)` | new file `game_node_coords.go` | new |
| `nodeTiming` + `(*Game) nodeLiveTiming(id)` (candidate-index prev/next + memo) | `game_node_coords.go` | new |
| coord-line formatter(s) | `game_node_coords.go` | new |
| header band height + coord-line draw | `game_node_sidebar.go` (`drawHeader`, `layout`, header constants) | changed |

## Testing

Follow the functional-tests-over-isolation convention (drive a real `Game`, no
hand-built stubs where a real path exists).

- **`BeatSlot`** unit table: on-beat, mid-beat, `idx = 0`, negative idx, `div`
  boundary (last slot of a beat), `div <= 0` guard.
- **`nodeCircuitPosition`**: looping circuit, open path, multi-occurrence
  (self-crossing) circuit, disconnected node → `inCircuit == false`.
- **`nodeLiveTiming`**: build a real looping circuit, advance the playhead, assert
  `prev`/`next` indices and that they honor a probability/skip-every-N node
  (a gated cycle is skipped in `next`); assert `prev` reads back through the timeline
  after a fire commits; assert stopped → `next`-from-0, no `prev`.
- **Memo invalidation**: assert recompute on path-signature change and on playhead
  crossing `nextIdx`, and that an unchanged frame returns the cached value.
- **Sidebar display**: extend the existing `TestNodePopupShowsCoordinates` to assert
  the grid line and the beat/subdivision text; add a playing-state assertion for the
  `last`/`next` line.

## Risks / open points

- **Next fire beyond horizon**: if the next candidate is past `Predictor.Horizon()`,
  show `next  …` rather than forcing an `Ensure`. In practice the scheduler keeps the
  horizon ahead of the playhead by more than one loop, so this is rare.
- **Design-token discipline**: any pill/well fills reuse existing `Token*`/`WithAlpha`
  accessors; no new `color.RGBA` literals (ratchet test).
