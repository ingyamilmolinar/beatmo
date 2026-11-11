# Node Coordinates in Pop-up Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a node's grid coordinates, its circuit-aware beat/subdivision position, and its live last/next fire in the `NodeSidebar` pop-up menu.

**Architecture:** A new pure `Grid.BeatSlot` converts an absolute subdivision index to a 1-based beat + slot. A new `game_node_coords.go` resolves a node's position from the existing traversal paths (`beatInfosByRow`) and computes live last/next fire by sampling the Timeline (past) and Predictor (future) at the node's own indices, memoized as node state. The sidebar header band grows by one two-line block and renders the readout.

**Tech Stack:** Go 1.26, Ebiten (stubbed under `-tags test`), existing `core/engine` Predictor + `internal/timeline` Service.

## Global Constraints

- **Never run `git add` / `git commit`.** The user manages all staging/commits. Each task ends at a working-tree checkpoint (build + tests green), left uncommitted.
- **Test command (fast path):** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/timeline/`
- **Use the bundled Go:** `../../.tools/go/bin/go` (never system `go`).
- **Functional tests over isolation:** drive a real `Game` via existing helpers (`New`, `Layout`, `tryAddNode`, `addEdge`, `updateBeatInfos`, `SetPlaying`, `advanceFrames`), not hand-built stubs.
- **No new color literals:** reuse `colTextSecondary` / existing `Token*` accessors (token-discipline ratchet test).
- **No forbidden glyphs** (`←`, `→`, etc.) in `internal/ui/`. Use the text labels `last` / `next`.
- **Beat/subdivision is circuit-aware:** derive from the node's path index (`beatInfosByRow`), never from `node.I`.

## File Structure

- `src/go/internal/ui/grid.go` — add `BeatSlot` next to `BeatSubdivision`.
- `src/go/internal/ui/grid_test.go` — `BeatSlot` unit table.
- `src/go/internal/ui/game_node_coords.go` — **new.** `nodeCircuitPosition`, `nodeTiming`, `nodeTimingMemo`, `nodeLiveTiming`, and the two formatters.
- `src/go/internal/ui/game_struct.go` — add one memo field to the `Game` struct.
- `src/go/internal/ui/node_coords_test.go` — **new.** Functional tests for position + live timing + formatters.
- `src/go/internal/ui/game_node_sidebar.go` — grow header band (`layout`), render coord lines (`drawHeader`), fix `contentTop` / `sepY` in `Draw`.
- `src/go/internal/ui/node_popup_info_test.go` — extend `TestNodePopupShowsCoordinates`; add a circuit-based coord-line test.

---

### Task 1: `Grid.BeatSlot` pure helper

**Files:**
- Modify: `src/go/internal/ui/grid.go` (add after `BeatSubdivision`, ~line 197)
- Test: `src/go/internal/ui/grid_test.go`

**Interfaces:**
- Produces: `func (g *Grid) BeatSlot(idx int) (beat, slot, div int)` — 1-based beat, 1-based subdivision slot, denominator `= MaxDiv()`. NOT reduced (slot `4/8`, not `1/2`). `idx < 0` and `div <= 0` clamp to `(1, 1, ...)`.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/grid_test.go`:

```go
func TestBeatSlot(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions: %v", err)
	}
	cases := []struct {
		idx, beat, slot, div int
	}{
		{0, 1, 1, 8},   // first slot of beat 1
		{3, 1, 4, 8},   // mid beat 1
		{7, 1, 8, 8},   // last slot of beat 1
		{8, 2, 1, 8},   // rolls to beat 2
		{19, 3, 4, 8},  // beat 3, slot 4/8
		{-4, 1, 1, 8},  // negative clamps to start
	}
	for _, c := range cases {
		beat, slot, div := g.BeatSlot(c.idx)
		if beat != c.beat || slot != c.slot || div != c.div {
			t.Errorf("BeatSlot(%d) = (%d,%d,%d), want (%d,%d,%d)",
				c.idx, beat, slot, div, c.beat, c.slot, c.div)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestBeatSlot`
Expected: FAIL — `g.BeatSlot undefined`.

- [ ] **Step 3: Write minimal implementation**

Add to `src/go/internal/ui/grid.go` after `BeatSubdivision`:

```go
// BeatSlot converts an absolute subdivision index into a 1-based beat number and
// a 1-based subdivision slot within that beat (slot of div). Unlike
// BeatSubdivision, the slot is NOT reduced: with div=8, idx=19 -> beat 3, slot 4,
// div 8 ("4/8"), matching the step grid's raw subdivision slots. This is the
// user-facing "beat and subdivision" form for a node's position in its circuit.
// Negative idx and div<=0 clamp to the first slot of beat 1.
func (g *Grid) BeatSlot(idx int) (beat, slot, div int) {
	div = g.MaxDiv()
	if div <= 0 {
		return 1, 1, 1
	}
	if idx < 0 {
		idx = 0
	}
	beat = idx/div + 1
	slot = idx%div + 1
	return beat, slot, div
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestBeatSlot`
Expected: PASS.

- [ ] **Step 5: Checkpoint (do NOT commit)**

Run the package build to confirm nothing else broke:
`cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Leave changes uncommitted.

---

### Task 2: `nodeCircuitPosition` — the node's home position

**Files:**
- Create: `src/go/internal/ui/game_node_coords.go`
- Test: `src/go/internal/ui/node_coords_test.go`

**Interfaces:**
- Consumes: `g.nodeRows` (`map[model.NodeID]int`), `g.beatInfosByRow` (`[][]model.BeatInfo`).
- Produces: `func (g *Game) nodeCircuitPosition(id model.NodeID) (homeIdx, occurrences, row int, inCircuit bool)` — `homeIdx` = first index of `id` in its row's path; `occurrences` = count in that path slice; `inCircuit == false` when the node is in no path (disconnected / off-circuit).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/node_coords_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// buildLoopGame builds a 2-node loop a(0,0) -> b(4,0) -> a on row 0 at subdiv 8.
func buildLoopGame(t *testing.T) (g *Game, a, b *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions: %v", err)
	}
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	return g, a, b
}

func TestNodeCircuitPosition(t *testing.T) {
	g, a, b := buildLoopGame(t)

	homeA, occA, rowA, inA := g.nodeCircuitPosition(a.ID)
	if !inA || rowA != 0 {
		t.Fatalf("node a: inCircuit=%v row=%d, want true/0", inA, rowA)
	}
	if homeA != 0 {
		t.Fatalf("node a homeIdx=%d, want 0 (circuit start)", homeA)
	}
	if occA < 1 {
		t.Fatalf("node a occurrences=%d, want >=1", occA)
	}

	homeB, _, _, inB := g.nodeCircuitPosition(b.ID)
	if !inB {
		t.Fatal("node b should be in the circuit")
	}
	if homeB <= homeA {
		t.Fatalf("node b homeIdx=%d should be after node a's %d", homeB, homeA)
	}

	// A disconnected node is off-circuit.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	if _, _, _, inD := g.nodeCircuitPosition(d.ID); inD {
		t.Fatal("disconnected node should report inCircuit=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNodeCircuitPosition`
Expected: FAIL — `g.nodeCircuitPosition undefined`.

- [ ] **Step 3: Write minimal implementation**

Create `src/go/internal/ui/game_node_coords.go`:

```go
package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeCircuitPosition resolves a node's static position within its traversal
// path. row is the node's row (from g.nodeRows); homeIdx is the first index at
// which the node appears in that row's flattened path (beatInfosByRow), which is
// the absolute subdivision count from the circuit's start. occurrences counts how
// many times the node appears in one path slice (a self-crossing circuit visits a
// node more than once). inCircuit is false when the node participates in no path
// (a disconnected / open dead-end node), in which case it has no beat position.
func (g *Game) nodeCircuitPosition(id model.NodeID) (homeIdx, occurrences, row int, inCircuit bool) {
	r, ok := g.nodeRows[id]
	if !ok || r < 0 || r >= len(g.beatInfosByRow) {
		return 0, 0, -1, false
	}
	infos := g.beatInfosByRow[r]
	homeIdx = -1
	for i := range infos {
		if infos[i].NodeID == id {
			if homeIdx < 0 {
				homeIdx = i
			}
			occurrences++
		}
	}
	if homeIdx < 0 {
		return 0, 0, r, false
	}
	return homeIdx, occurrences, r, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNodeCircuitPosition`
Expected: PASS.

- [ ] **Step 5: Checkpoint (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Leave changes uncommitted.

---

### Task 3: `nodeLiveTiming` — live last/next fire, memoized as node state

**Files:**
- Modify: `src/go/internal/ui/game_node_coords.go` (add struct + method)
- Modify: `src/go/internal/ui/game_struct.go` (add one memo field to `Game`)
- Test: `src/go/internal/ui/node_coords_test.go`

**Interfaces:**
- Consumes: `nodeCircuitPosition` (Task 2); `g.Playing()`; `g.elapsedBeats`; `g.pathSigByRow`; `g.isLoopByRow`; `g.loopStartByRow`; `g.beatInfoAtRow(row, idx)`; `g.engine.Predictor.TriggeredAt/Horizon`; `g.timeline.Committed(row, abs)`.
- Produces:
  - `type nodeTiming struct { inCircuit bool; homeIdx, occurrences, row int; prevIdx int; prevOK bool; nextIdx int; nextOK bool }`
  - `func (g *Game) nodeLiveTiming(id model.NodeID) nodeTiming`
  - `Game` field `nodeTimingCache nodeTimingMemo`.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/node_coords_test.go`:

```go
func TestNodeLiveTimingStopped(t *testing.T) {
	g, a, b := buildLoopGame(t)

	// Stopped: next-from-0 (loop start), no prev.
	ta := g.nodeLiveTiming(a.ID)
	if !ta.inCircuit {
		t.Fatal("node a should be in circuit")
	}
	if !ta.nextOK || ta.nextIdx != ta.homeIdx {
		t.Fatalf("stopped next: ok=%v idx=%d, want home %d", ta.nextOK, ta.nextIdx, ta.homeIdx)
	}
	if ta.prevOK {
		t.Fatal("stopped: prev must not be set")
	}

	tb := g.nodeLiveTiming(b.ID)
	if !tb.nextOK || tb.nextIdx != tb.homeIdx {
		t.Fatalf("stopped next for b: idx=%d want home %d", tb.nextIdx, tb.homeIdx)
	}

	// Off-circuit node: no timing.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	if td := g.nodeLiveTiming(d.ID); td.inCircuit || td.nextOK || td.prevOK {
		t.Fatalf("off-circuit node timing should be empty: %+v", td)
	}
}

func TestNodeLiveTimingPrevFromTimeline(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	g.SetPlaying(true)
	advanceFrames(g, 2)

	// Place the playhead exactly one loop period past node a's home index and
	// record a past fire of node a at that home index. The prev scan covers
	// [playhead-period, playhead-1] = [homeA, homeA+period-1], which includes
	// homeA (node a's only occurrence in that window). Using +period (not
	// +period+1) keeps homeA inside the window.
	period := len(g.beatInfosByRow[0])
	if g.isLoopByRow[0] {
		period = len(g.beatInfosByRow[0]) - g.loopStartByRow[0]
	}
	homeA, _, _, _ := g.nodeCircuitPosition(a.ID)
	g.elapsedBeats = homeA + period
	g.timeline.RecordCommitKind(0, homeA, true, model.NodeTypeRegular,
		timeline.CommitKindPlayback, 256, 256)

	ta := g.nodeLiveTiming(a.ID)
	if !ta.prevOK || ta.prevIdx != homeA {
		t.Fatalf("playing prev: ok=%v idx=%d, want home %d", ta.prevOK, ta.prevIdx, homeA)
	}
}

func TestNodeLiveTimingMemoReuse(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	_ = g.nodeLiveTiming(a.ID)
	if !g.nodeTimingCache.valid || g.nodeTimingCache.id != a.ID {
		t.Fatal("memo should be populated after first call")
	}
	// Second call with nothing changed reuses the cache (playhead unchanged).
	sigBefore := g.nodeTimingCache.pathSig
	_ = g.nodeLiveTiming(a.ID)
	if g.nodeTimingCache.pathSig != sigBefore {
		t.Fatal("memo should be stable across identical calls")
	}
}
```

Add the timeline import to the test file's import block:

```go
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNodeLiveTiming`
Expected: FAIL — `g.nodeLiveTiming undefined` / `g.nodeTimingCache undefined`.

- [ ] **Step 3a: Add the memo field to the Game struct**

In `src/go/internal/ui/game_struct.go`, inside the `Game` struct (near the other UI render-state fields — e.g. just after the `state *gamestate.State` line at ~line 34), add:

```go
	// nodeTimingCache memoizes the sidebar's per-node beat/last/next readout so
	// nodeLiveTiming does not rescan the circuit every frame. Derived render
	// state, never serialized. See game_node_coords.go.
	nodeTimingCache nodeTimingMemo
```

- [ ] **Step 3b: Add the struct + method to game_node_coords.go**

Append to `src/go/internal/ui/game_node_coords.go`:

```go
// nodeTiming is the derived timing readout for one node: its static home position
// in its circuit plus (during playback) its previous and next ACTUAL fire relative
// to the playhead. "Actual" means honoring logic gates (probability, skip-every-N):
// next is read from the Predictor, prev from the frozen Timeline.
type nodeTiming struct {
	inCircuit   bool
	homeIdx     int
	occurrences int
	row         int

	prevIdx int
	prevOK  bool
	nextIdx int
	nextOK  bool
}

// nodeTimingMemo caches one nodeTiming keyed by node id + row path signature +
// play state. It stays valid while the playhead sits within [prevIdx, nextIdx);
// crossing nextIdx (or seeking back past prevIdx) forces a recompute.
type nodeTimingMemo struct {
	valid    bool
	id       model.NodeID
	pathSig  uint64
	playing  bool
	playhead int
	t        nodeTiming
}

// nodeLiveTiming returns the node's home position and its live last/next fire.
// Next fire is sampled from the Predictor (future, honors logic gates); prev fire
// from the Timeline (frozen past). Because each absolute index in a row maps 1:1
// to one node, timeline.Committed's bool IS "did this node fire at idx". When
// stopped, prev is omitted and next is computed from loop start (position 0).
func (g *Game) nodeLiveTiming(id model.NodeID) nodeTiming {
	home, occ, row, inCircuit := g.nodeCircuitPosition(id)
	if !inCircuit {
		g.nodeTimingCache = nodeTimingMemo{}
		return nodeTiming{inCircuit: false}
	}

	playing := g.Playing()
	playhead := g.elapsedBeats
	var sig uint64
	if row >= 0 && row < len(g.pathSigByRow) {
		sig = g.pathSigByRow[row]
	}

	// Memo hit: same node/path/play-state, playhead still within the cached span.
	if m := &g.nodeTimingCache; m.valid && m.id == id && m.pathSig == sig && m.playing == playing {
		fresh := true
		if playing {
			if m.t.nextOK && playhead >= m.t.nextIdx {
				fresh = false // crossed the next fire
			}
			if m.t.prevOK && playhead < m.t.prevIdx {
				fresh = false // seeked back past the prev fire
			}
			if !m.t.prevOK && playhead != m.playhead {
				fresh = false // a prev may now exist
			}
		}
		if fresh {
			return m.t
		}
	}

	t := nodeTiming{inCircuit: true, homeIdx: home, occurrences: occ, row: row}

	// One full loop period bounds every scan: a node fires at most once per period,
	// so one period is enough to find the nearest fire or conclude there is none.
	period := len(g.beatInfosByRow[row])
	if g.isLoopByRow[row] {
		period = len(g.beatInfosByRow[row]) - g.loopStartByRow[row]
	}
	if period < 1 {
		period = 1
	}

	start := playhead
	if !playing {
		start = 0 // stopped: next-from-0
	}

	// Next actual fire: first candidate idx >= start the Predictor triggers.
	if g.engine != nil && g.engine.Predictor != nil {
		horizon := g.engine.Predictor.Horizon()
		for idx := start; idx <= start+period; idx++ {
			if horizon > 0 && idx >= horizon {
				break
			}
			if g.beatInfoAtRow(row, idx).NodeID == id && g.engine.Predictor.TriggeredAt(row, idx) {
				t.nextIdx = idx
				t.nextOK = true
				break
			}
		}
	}

	// Prev actual fire (playing only): largest committed idx < playhead.
	if playing && g.timeline != nil {
		lo := playhead - period
		if lo < 0 {
			lo = 0
		}
		for idx := playhead - 1; idx >= lo; idx-- {
			if g.beatInfoAtRow(row, idx).NodeID != id {
				continue
			}
			if fired, _, ok := g.timeline.Committed(row, idx); ok && fired {
				t.prevIdx = idx
				t.prevOK = true
				break
			}
		}
	}

	g.nodeTimingCache = nodeTimingMemo{
		valid: true, id: id, pathSig: sig, playing: playing, playhead: playhead, t: t,
	}
	return t
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNodeLiveTiming`
Expected: PASS (all three sub-tests).

- [ ] **Step 5: Checkpoint (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Leave changes uncommitted.

---

### Task 4: Coordinate-line formatters

**Files:**
- Modify: `src/go/internal/ui/game_node_coords.go`
- Test: `src/go/internal/ui/node_coords_test.go`

**Interfaces:**
- Consumes: `g.grid.BeatSlot` (Task 1); `nodeTiming` (Task 3).
- Produces:
  - `func (g *Game) formatNodeCoordHome(i, j int, t nodeTiming) string`
  - `func (g *Game) formatNodeFireLine(t nodeTiming, playing bool) string`

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/node_coords_test.go`:

```go
func TestFormatNodeCoordLines(t *testing.T) {
	g, a, _ := buildLoopGame(t)

	// Home line: grid coords + circuit beat/subdivision. Node a is at grid (0,0),
	// home idx 0 -> Beat 1 · 1/8.
	ta := g.nodeLiveTiming(a.ID)
	home := g.formatNodeCoordHome(a.I, a.J, ta)
	if home != "(0, 0) · Beat 1 · 1/8" {
		t.Fatalf("home line = %q, want %q", home, "(0, 0) · Beat 1 · 1/8")
	}

	// Off-circuit node.
	d := g.tryAddNode(9, 9, model.NodeTypeRegular)
	g.updateBeatInfos()
	td := g.nodeLiveTiming(d.ID)
	off := g.formatNodeCoordHome(d.I, d.J, td)
	if off != "(9, 9) · off-circuit" {
		t.Fatalf("off-circuit line = %q, want %q", off, "(9, 9) · off-circuit")
	}
	if g.formatNodeFireLine(td, false) != "" {
		t.Fatal("off-circuit fire line should be empty")
	}

	// Stopped fire line: next only, from position 0.
	stopped := g.formatNodeFireLine(ta, false)
	if stopped != "next  Beat 1 · 1/8" {
		t.Fatalf("stopped fire line = %q, want %q", stopped, "next  Beat 1 · 1/8")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFormatNodeCoordLines`
Expected: FAIL — `g.formatNodeCoordHome undefined`.

- [ ] **Step 3: Write minimal implementation**

Append to `src/go/internal/ui/game_node_coords.go`:

```go
// formatNodeCoordHome renders the always-visible position line:
//   "(5, 2) · Beat 3 · 4/8"          — in a circuit
//   "(5, 2) · Beat 3 · 4/8 (+1)"     — appears more than once (self-crossing)
//   "(9, 9) · off-circuit"           — participates in no path
func (g *Game) formatNodeCoordHome(i, j int, t nodeTiming) string {
	if !t.inCircuit {
		return fmt.Sprintf("(%d, %d) · off-circuit", i, j)
	}
	beat, slot, div := g.grid.BeatSlot(t.homeIdx)
	s := fmt.Sprintf("(%d, %d) · Beat %d · %d/%d", i, j, beat, slot, div)
	if t.occurrences > 1 {
		s += fmt.Sprintf(" (+%d)", t.occurrences-1)
	}
	return s
}

// formatNodeFireLine renders the live last/next line. Empty when off-circuit.
// Stopped: "next  Beat 3 · 4/8". Playing: "last  Beat 2 · 4/8    next  Beat 6 · 4/8".
// A missing side reads as an em dash.
func (g *Game) formatNodeFireLine(t nodeTiming, playing bool) string {
	if !t.inCircuit {
		return ""
	}
	part := func(prefix string, idx int, ok bool) string {
		if !ok {
			return prefix + "  —"
		}
		beat, slot, div := g.grid.BeatSlot(idx)
		return fmt.Sprintf("%s  Beat %d · %d/%d", prefix, beat, slot, div)
	}
	if !playing {
		return part("next", t.nextIdx, t.nextOK)
	}
	return part("last", t.prevIdx, t.prevOK) + "    " + part("next", t.nextIdx, t.nextOK)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFormatNodeCoordLines`
Expected: PASS.

- [ ] **Step 5: Checkpoint (do NOT commit)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/`
Leave changes uncommitted.

---

### Task 5: Render the coordinate readout in the sidebar header

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`layout`, `Draw`, `drawHeader`)
- Test: `src/go/internal/ui/node_popup_info_test.go`

**Interfaces:**
- Consumes: `formatNodeCoordHome`, `formatNodeFireLine`, `nodeLiveTiming` (Tasks 3-4); `sb.node.I/J`; `sb.rects["coords"]`.
- Produces: a `coords` rect in the fixed header band + two drawn text lines.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/node_popup_info_test.go` (add `"strings"` to its imports):

```go
func TestNodePopupCoordinateLine(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions: %v", err)
	}

	// 2-node loop so node a belongs to a circuit.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()

	g.sidebar.Open(a)
	g.sidebar.layout()

	// The coords rect exists inside the header band and is non-empty.
	coords, ok := g.sidebar.rects["coords"]
	if !ok || coords.Empty() {
		t.Fatal("coords rect missing or empty")
	}
	hdr := g.sidebar.rects["header"]
	if !coords.In(hdr) {
		t.Fatalf("coords rect %v must sit inside header band %v", coords, hdr)
	}

	// The formatted home line carries the grid coords and the circuit beat.
	ta := g.nodeLiveTiming(a.ID)
	line := g.formatNodeCoordHome(a.I, a.J, ta)
	if !strings.Contains(line, "(0, 0)") || !strings.Contains(line, "Beat 1") {
		t.Fatalf("coord line %q missing grid coords or beat", line)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNodePopupCoordinateLine`
Expected: FAIL — `sb.rects["coords"]` missing (empty rect).

- [ ] **Step 3a: Grow the header band in `layout()`**

In `game_node_sidebar.go`, replace the header/content-top block at ~lines 304-311:

```go
	// Fixed header (NOT scrolled)
	closeSize := sidebarBtnH
	headerRect := image.Rect(sidebarPad, sidebarPad, w-sidebarPad, sidebarPad+sidebarHeaderH)
	sb.rects["header"] = headerRect
	sb.rects["close"] = image.Rect(w-sidebarPad-closeSize, sidebarPad, w-sidebarPad, sidebarPad+closeSize)

	// Content starts below header
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
```

with:

```go
	// Fixed header (NOT scrolled): title band + a two-line coordinate readout
	// (grid coords + circuit beat/subdivision, then live last/next fire).
	closeSize := sidebarBtnH
	coordH := 2*StyledTextHeight(RoleCaption) + 6
	headerRect := image.Rect(sidebarPad, sidebarPad, w-sidebarPad, sidebarPad+sidebarHeaderH+coordH)
	sb.rects["header"] = headerRect
	sb.rects["coords"] = image.Rect(sidebarPad, sidebarPad+sidebarHeaderH+2, w-sidebarPad, headerRect.Max.Y)
	sb.rects["close"] = image.Rect(w-sidebarPad-closeSize, sidebarPad, w-sidebarPad, sidebarPad+closeSize)

	// Content starts below the (taller) header band.
	contentTop := headerRect.Max.Y + sidebarGap
```

- [ ] **Step 3b: Fix `contentTop` in `Draw()`**

In `Draw()` at ~line 1466, replace:

```go
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
```

with:

```go
	contentTop := sb.rects["header"].Max.Y + sidebarGap
```

- [ ] **Step 3c: Move the header separator below the coord band**

In `Draw()` at ~line 1748, replace:

```go
	sepY := sidebarPad + sidebarHeaderH
```

with:

```go
	sepY := sb.rects["header"].Max.Y
```

- [ ] **Step 3d: Draw the coordinate lines in `drawHeader`**

In `drawHeader`, after the `drawMenuTitle(dst, titleRect, label, swatchCol)` call (~line 1813), add:

```go
	// Coordinate readout: grid coords + circuit beat/subdivision (line 1) and the
	// live last/next fire (line 2). Always visible in the fixed header band.
	if coordR := sb.rects["coords"]; !coordR.Empty() {
		tm := g.nodeLiveTiming(sb.node.ID)
		line1 := g.formatNodeCoordHome(sb.node.I, sb.node.J, tm)
		line2 := g.formatNodeFireLine(tm, g.Playing())
		lineH := StyledTextHeight(RoleCaption)
		x := coordR.Min.X + 2
		DrawTextStyled(dst, line1, x, coordR.Min.Y, RoleCaption, colTextSecondary)
		if line2 != "" {
			DrawTextStyled(dst, line2, x, coordR.Min.Y+lineH+2, RoleCaption, colTextSecondary)
		}
	}
```

- [ ] **Step 4: Run the test + the existing popup tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestNodePopup|TestCoordBadge'`
Expected: PASS (new `TestNodePopupCoordinateLine`, plus `TestNodePopupShowsCoordinates` / `TestNodePopupShowsInstrument` / `TestNodePopupMoveButtonExists` still green with the taller header).

- [ ] **Step 5: Checkpoint (do NOT commit)**

Full-package run to catch layout-dependent regressions:
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/timeline/`
Leave changes uncommitted.

---

## Final verification

- [ ] **Run the full fast Go suite for the touched packages:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/timeline/`
Expected: PASS.

- [ ] **Confirm no token-discipline / drift regressions:**

`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline' ./internal/ui/`
Expected: PASS (no new color literals introduced).

- [ ] **Manual visual check (optional, per the run/verify skills):** open the node pop-up on a node in a loop, confirm the header shows `(i, j) · Beat B · slot/div` and, during playback, a live `last … next …` line that updates as the playhead moves.

## Self-review notes (coverage vs spec)

- Grid `(i,j)` + circuit-aware Beat·subdivision → Tasks 1, 2, 4, 5.
- Live last/next fire, Predictor (next) + Timeline (prev) → Task 3.
- Cached as node state, invalidated on path-sig change / playhead crossing → Task 3 (`nodeTimingMemo`).
- Always-visible header line, `contentTop` derived, forbidden-glyph-free labels → Task 5.
- Off-circuit + multi-occurrence edge cases → Tasks 2, 4 tests.
- Stopped → next-from-0, no prev → Task 3 test `TestNodeLiveTimingStopped`.
