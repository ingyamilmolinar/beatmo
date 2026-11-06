# Move-Node Circuit Healing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Moving a node extracts it from its chain (all old edges removed), heals the chain behind it (`Np→Nn`, direction preserved, stitched through intermediates), and asks "Break the circuit?" via a modal portal when healing is impossible.

**Architecture:** A pure classifier `moveHealPlan` decides silent / pass-through / unhealable; `moveNode` gains extraction+heal semantics for ALL callers; the interactive move-placement path gates unhealable moves behind a portal-based confirm that keeps move mode armed on Cancel.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), ebitenstub functional tests, OverlayPortal, hooks.Bus, i18n.

**Spec:** `docs/superpowers/specs/2026-07-18-move-node-circuit-healing-design.md`

## Global Constraints

- **NEVER run `git add` or `git commit`** — standing user preference. "Checkpoint" = run listed tests and report.
- Go commands: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`; full `./internal/ui/` runs need `-timeout 25m`.
- Edges are DIRECTED (`[2]NodeID{from,to}`); heal preserves direction (`Np→N→Nn` heals `Np→Nn`, never the reverse).
- Heal classification (verbatim from spec): exactly one incoming + one outgoing, distinct endpoints, orthogonal (`Np.I==Nn.I || Np.J==Nn.J`) → **healPassThrough**; ≤1 edge → **healSilent**; anything else (corner, ≥3 edges, double-in, double-out, 2-cycle `A→N→A`) → **healUnhealable**.
- Interactive dialog: Break → move commits + `modes.Commit()`; Cancel → **mode STAYS ARMED**, graph untouched, NO `mode.canceled` emitted.
- All heal/extraction edge mutations inside the single `"move node"` undo group; one undo restores original topology exactly.
- hooks.Bus is async — event tests use subscribe+channel+timeout (pattern: `grid_mode_test.go`).
- No new color literals (token ratchet); new user-facing strings go through i18n (en + es; `TestCatalogCompleteness` binds both).
- A concurrent unrelated session edits this working tree — touch ONLY task files.
- Pre-existing reds NOT ours: node-logic volume tests; 5 timeline-offset ui tests while the concurrent timeline session is mid-edit; `touch_gestures` browser Test 1.

## File Structure

| File | Responsibility |
|---|---|
| Create `internal/ui/grid_move_heal.go` | `healClass`, `healPlan`, `moveHealPlan`, `nodesOnSpan` (pure classification + span query) |
| Create `internal/ui/grid_move_heal_test.go` | Classification unit-test table |
| Create `internal/ui/grid_move_break_confirm.go` | `breakPendingState`, begin/Confirm/Cancel, portal open/close/draw/rects |
| Modify `internal/ui/game_graph_nodes.go` | `moveNode` extraction+heal; prune rung closes the dialog on subject deletion |
| Modify `internal/ui/game_input_touch.go` | `handleArmedModeTap` unhealable gate |
| Modify `internal/ui/game_input_editor.go` | armed block yields while the dialog is open |
| Modify `internal/ui/game_input_shortcuts.go` | ESC rung for the dialog (above the armed-mode rung) |
| Modify `internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go` | `KeyBreakCircuitTitle`, `KeyCapBreak` |
| Modify `internal/ui/grid_mode_flow_test.go`, `move_node_test.go` | Heal + break-confirm matrices; legacy rewrites |

---

### Task 1: `moveHealPlan` classification (pure) + unit tests

**Files:**
- Create: `src/go/internal/ui/grid_move_heal.go`
- Test: `src/go/internal/ui/grid_move_heal_test.go`

**Interfaces:**
- Consumes: `g.edges []uiEdge` (fields `.A, .B *uiNode`), `g.nodes []*uiNode`, `uiNode{ID model.NodeID; I, J int}`.
- Produces (Tasks 2–3 rely on these exact names): `type healClass int` with consts `healSilent`, `healPassThrough`, `healUnhealable`; `type healPlan struct { class healClass; from, to *uiNode }`; `func (g *Game) moveHealPlan(n *uiNode) healPlan`; `func (g *Game) nodesOnSpan(a, b *uiNode) []*uiNode`.

- [ ] **Step 1: Write the failing tests** (`grid_move_heal_test.go`). Build graphs with the same Game/test-fixture helpers `move_node_test.go` uses (READ it first; reuse its constructor + `g.tryAddNode`/`g.addEdge`). Table-driven over these cases (assert `class` and, for pass-through, `from.ID`/`to.ID`):

| Case | Graph around N | Expect |
|---|---|---|
| isolated | no edges | healSilent |
| endpoint-in | A→N only | healSilent |
| endpoint-out | N→A only | healSilent |
| straight chain vertical | A(0,0)→N(0,4)→B(0,8) | healPassThrough, from=A, to=B |
| straight chain horizontal reversed | B(8,0)→N(4,0)→A(0,0) | healPassThrough, from=B, to=A |
| collinear-but-offset pass-through | A(0,0)→N(4,4)→B(0,8) (Np,Nn share I=0) | healPassThrough, from=A, to=B |
| corner | A(0,0)→N(0,4)→B(4,4) (A,B share nothing) | healUnhealable |
| junction 3 edges | A→N, B→N, N→C | healUnhealable |
| double-in | A→N, B→N (nothing out) | healUnhealable |
| double-out | N→A, N→B | healUnhealable |
| two-cycle | A→N, N→A | healUnhealable |

Also test `nodesOnSpan`: chain A(0,0), M(0,3), B(0,8) → `nodesOnSpan(A,B)` returns exactly M; `nodesOnSpan(A,M)` returns empty; non-orthogonal pair returns empty.

- [ ] **Step 2: Run to verify failure**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMoveHealPlan|TestNodesOnSpan'`
Expected: FAIL (compile error: `moveHealPlan` undefined).

- [ ] **Step 3: Implement** `grid_move_heal.go`:

```go
package ui

// healClass classifies how extracting a node from its current position
// affects circuit connectivity (see the circuit-healing design spec).
type healClass int

const (
	// healSilent: ≤1 edge — extraction disconnects nothing else; no heal.
	healSilent healClass = iota
	// healPassThrough: exactly Np→N→Nn with Np,Nn orthogonal — heal Np→Nn.
	healPassThrough
	// healUnhealable: corner / junction / double-in / double-out / 2-cycle —
	// extraction breaks the circuit; interactive callers must confirm.
	healUnhealable
)

// healPlan is the pre-computed extraction plan for moving a node.
type healPlan struct {
	class healClass
	from  *uiNode // Np (healPassThrough only)
	to    *uiNode // Nn (healPassThrough only)
}

// moveHealPlan classifies n's current edges. Pure — no mutation. Edges are
// directed: an incoming edge is e.B==n, an outgoing edge is e.A==n.
func (g *Game) moveHealPlan(n *uiNode) healPlan {
	var ins, outs []*uiNode
	for i := range g.edges {
		e := &g.edges[i]
		switch {
		case e.A.ID == n.ID:
			outs = append(outs, e.B)
		case e.B.ID == n.ID:
			ins = append(ins, e.A)
		}
	}
	if len(ins)+len(outs) <= 1 {
		return healPlan{class: healSilent}
	}
	if len(ins) == 1 && len(outs) == 1 && ins[0].ID != outs[0].ID {
		np, nn := ins[0], outs[0]
		if np.I == nn.I || np.J == nn.J {
			return healPlan{class: healPassThrough, from: np, to: nn}
		}
	}
	return healPlan{class: healUnhealable}
}

// nodesOnSpan returns the nodes lying strictly between a and b on their
// shared row/column (empty for non-orthogonal pairs). Callers stitch each
// so a healed edge stays split node-to-node.
func (g *Game) nodesOnSpan(a, b *uiNode) []*uiNode {
	var out []*uiNode
	switch {
	case a.I == b.I:
		lo, hi := min(a.J, b.J), max(a.J, b.J)
		for _, m := range g.nodes {
			if m.I == a.I && m.J > lo && m.J < hi {
				out = append(out, m)
			}
		}
	case a.J == b.J:
		lo, hi := min(a.I, b.I), max(a.I, b.I)
		for _, m := range g.nodes {
			if m.J == a.J && m.I > lo && m.I < hi {
				out = append(out, m)
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify pass** (same command). Expected: PASS. gofmt the two files.

- [ ] **Step 5: Checkpoint** — report. Do NOT commit.

---

### Task 2: `moveNode` extraction + heal semantics

**Files:**
- Modify: `src/go/internal/ui/game_graph_nodes.go` (`moveNode`, currently ~line 365)
- Test: `src/go/internal/ui/grid_mode_flow_test.go` (append), `src/go/internal/ui/move_node_test.go` (rewrite legacy)

**Interfaces:**
- Consumes: Task 1's `moveHealPlan`/`healPassThrough`/`nodesOnSpan`; existing `deleteEdgeNoRefresh`, `addEdgeNoRefresh`, `stitchEdgesAt`, `beginUndoGroup`.
- Produces: `moveNode(n, newI, newJ) bool` with extraction semantics — Task 3 calls it unchanged.

- [ ] **Step 1: Write the failing tests.** Spec matrix cases 1–7 + 13 + undo (real handlers where placement is interactive; direct `g.moveNode` for programmatic cases). Required scenarios and assertions (write real Go against the file's existing helpers):

```go
// TestMoveNode_HealsStraightChain (spec #1): A(0,0)→N(0,4)→B(0,8) plus a
// separate edge C(6,0)→D(6,8). moveNode(N, 6, 4) → assert: edge A→B exists
// (hasEdgeBetween direction-sensitive), A→N and N→B gone; N at (6,4) spliced
// into C–D (C→N and N→D exist, C→D gone).
//
// TestMoveNode_HealPreservesDirection (spec #2): B(0,8)→N(0,4)→A(0,0);
// after move, edge B→A exists and A→B does NOT (assert directed pair).
//
// TestMoveNode_HealStitchesThroughIntermediate (spec #3): A(0,0)→N(0,6)→B(0,12)
// with bystander M(0,3) (no edges). Move N away → healed chain is A→M and
// M→B; NO direct A→B edge remains.
//
// TestMoveNode_HealDedupes (spec #4): A→N, N→B AND pre-existing direct A→B.
// Move N away → exactly one A→B edge (count edges between A,B == 1).
//
// TestMoveMode_MoveAlongOwnChain (spec #5, interactive): A(0,0)→N(0,4)→B(0,8);
// arm move on N, tap cell (0,6) (interior of the healed A→B span) →
// A→N, N→B exist again with N at (0,6); NO unsplit A→B edge; then ONE undo
// restores N to (0,4) with the original A→N→B.
//
// TestMoveNode_EndpointSilent (spec #6): A→N only; moveNode → edge gone,
// no heal, N spliced at destination. And spec #7: isolated node moves fine.
//
// TestMoveNode_ProgrammaticCornerBreaksSilently (spec #13): square corner
// A→N→B perpendicular; direct g.moveNode succeeds, both edges gone, no
// heal edge, no dialog state (g.breakConfirmActive() is Task 3 — here just
// assert no panic and edges' absence).
//
// TestMoveMode_SingleUndoStep (EXTEND existing): after a healed move, one
// undo restores: N back, A→N/N→B back, A→B heal edge GONE, destination
// edge restored unsplit.
```

- [ ] **Step 2: Run to verify failure**

Run: `... -run 'TestMoveNode_Heal|TestMoveNode_Endpoint|TestMoveNode_Programmatic|TestMoveMode_MoveAlongOwnChain' ./internal/ui/`
Expected: FAIL (no healing; re-anchor semantics still active).

- [ ] **Step 3: Implement.** Replace `moveNode`'s edge handling (keep the occupied-check, undo group, and the tail — cache/coords/predictor/badge/emit — unchanged):

```go
	// Extraction: classify BEFORE touching edges, then remove ALL of n's
	// edges — the chain closes behind the node (heal below); n reconnects
	// only via the destination splice.
	plan := g.moveHealPlan(n)
	type edgeRef struct{ from, to *uiNode }
	var touching []edgeRef
	for _, e := range g.edges {
		if e.A.ID == n.ID || e.B.ID == n.ID {
			touching = append(touching, edgeRef{e.A, e.B})
		}
	}
	for _, er := range touching {
		g.deleteEdgeNoRefresh(er.from, er.to)
	}

	// Move the node in the graph model (unchanged block: MoveNode, cacheNode,
	// n.I/n.J/n.X/n.Y updates) …

	// Heal: close the chain behind a pass-through node, direction preserved,
	// splitting the healed edge through any node on its span (which includes
	// n itself when the destination lies on the healed span — that re-splice
	// is exactly the move-along-own-chain case). AFTER the coords update so
	// n's OLD cell no longer splits the heal.
	if plan.class == healPassThrough {
		if g.addEdgeNoRefresh(plan.from, plan.to) {
			for _, m := range g.nodesOnSpan(plan.from, plan.to) {
				g.stitchEdgesAt(m)
			}
		}
	}
```

The existing destination `stitchEdgesAt(n)` (guarded `!g.importing`) stays after this block — it no-ops when the heal already re-spliced n. DELETE the old "re-add edges that remain orthogonal" loop.

- [ ] **Step 4: Run new tests → PASS; then rewrite legacy tests:** `TestMoveNodePreservesEdges` → rename/rewrite as `TestMoveNodeExtractsAndHeals` (its old graph now heals — assert extraction outcome); `TestMoveNodeDropsNonOrthogonalEdges` → assert ALL old edges removed regardless of orthogonality (extraction) with heal when applicable; `TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently` (grid_mode_flow_test.go) → its topology is likely now unhealable-classified: convert it to assert `healUnhealable` classification via `moveHealPlan` + direct `moveNode` silent break (the interactive dialog is Task 3; keep a `// Task 3 wires the dialog` note). Run the full move/flow set:

Run: `... -run 'TestMoveNode|TestMoveMode|TestGridMode|TestCopyMode' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — report, listing legacy rewrites. Do NOT commit.

---

### Task 3: "Break the circuit?" confirm portal + interactive gate

**Files:**
- Create: `src/go/internal/ui/grid_move_break_confirm.go`
- Modify: `src/go/internal/ui/game_input_touch.go` (`handleArmedModeTap` move case), `game_input_editor.go` (armed block), `game_input_shortcuts.go` (ESC rung), `game_graph_nodes.go` (prune rung)
- Modify: `src/go/internal/i18n/keys.go` (+`KeyBreakCircuitTitle Key = "confirm.break_circuit"`, `KeyCapBreak Key = "cap.break"`), `catalog_en.go` (`"Break the circuit?"`, `"Break"`), `catalog_es.go` (`"¿Romper el circuito?"`, `"Romper"`)
- Test: `src/go/internal/ui/grid_mode_flow_test.go` (append)

**Interfaces:**
- Consumes: Task 1 `moveHealPlan`/`healUnhealable`; Task 2 `moveNode`; the paste-confirm portal pattern in `node_clipboard.go:182-300` (mirror it EXACTLY — including the `escapeFn` cleanup-only contract comment); `g.modes` manager; `dvOverlayPortal`, `PortalEntry`, `g.drum.portal()`.
- Produces: `breakConfirmActive() bool`, `beginBreakConfirm(n *uiNode, i, j int)`, `ConfirmBreakCircuit()`, `CancelBreakCircuit()`, const `breakConfirmPortalID = "break-circuit-confirm"`.

- [ ] **Step 1: Write the failing tests** (spec #8–12, #14, plus retry/deletion):

```go
// Square circuit fixture for the corner cases: A(0,0)→N(0,6)→B(6,6)→C(6,0)→A,
// plus a separate target edge X(10,0)→Y(10,8) to supply a valid GuideEdge
// destination at (10,4).
//
// TestMoveMode_CornerOpensBreakConfirm (#8): arm move on N (corner:
// in=A(0,0), out=B(6,6) — perpendicular), tap (10,4) → g.breakConfirmActive()
// true, graph UNCHANGED (all 4 square edges intact, N at (0,6)),
// g.modes.Is(GridModeMove) still true.
//
// TestMoveMode_BreakConfirmBreak (#9): …then g.ConfirmBreakCircuit() →
// N at (10,4) spliced into X–Y; A→N and N→B gone; NO heal edge A→B;
// mode committed (GridModeNone). ONE undo restores the entire square AND
// unsplits X–Y.
//
// TestMoveMode_BreakConfirmCancelStaysArmed (#10): …instead
// g.CancelBreakCircuit() → graph untouched, breakConfirmActive false,
// mode STILL ARMED with subject N; a follow-up tap on the same (10,4)
// re-opens the dialog (retry works); ESC (handleEscape) then cancels the
// dialog first (rung) and a second ESC disarms the mode.
//
// TestMoveMode_JunctionOpensBreakConfirm (#11): N with 3 edges → dialog.
// TestMoveMode_DoubleInOpensBreakConfirm (#12): A→N, B→N → dialog; and
// double-out variant.
//
// TestMoveMode_BreakConfirmEvents (#14): subscribe EventPopupOpened/
// EventPopupClosed/EventGridModeCanceled/EventNodeMoved. Corner tap →
// popup.opened{ID:"break-circuit-confirm"}. Cancel → popup.closed, NO
// mode.canceled within 150ms. Re-tap + Confirm → node.moved arrives.
// (Portal Open/Close emit popup events automatically via portal.go.)
//
// TestMoveMode_SubjectDeletedClosesBreakConfirm: dialog open, delete N
// (g.deleteNode), run the prune pass → breakConfirmActive false, mode None.
// NOTE: if the headless test Game has no portal (g.drum.portal()==nil), the
// begin/Confirm/Cancel logic must still work — mirror how paste-confirm
// tests handle it (see node_clipboard_test.go).
```

- [ ] **Step 2: Run to verify failure** (`-run 'TestMoveMode_Corner|TestMoveMode_Break|TestMoveMode_Junction|TestMoveMode_DoubleIn|TestMoveMode_SubjectDeletedCloses'`). Expected: FAIL (compile: `breakConfirmActive` undefined).

- [ ] **Step 3: Implement.**

(a) `grid_move_break_confirm.go` — mirror `node_clipboard.go:182-300` structure exactly:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const breakConfirmPortalID = "break-circuit-confirm"

// breakPendingState holds an in-flight "Break the circuit?" confirmation:
// the armed subject and the tapped destination, captured at tap time so
// graph edits while the dialog is open cannot misplace the node.
type breakPendingState struct {
	active bool
	node   *uiNode
	i, j   int
}

func (g *Game) breakConfirmActive() bool { return g.breakPending.active }

// beginBreakConfirm arms the confirmation and opens the modal portal. The
// graph is NOT mutated until ConfirmBreakCircuit; move mode stays armed.
func (g *Game) beginBreakConfirm(n *uiNode, i, j int) {
	g.breakPending = breakPendingState{active: true, node: n, i: i, j: j}
	g.openBreakConfirmPortal()
}

// ConfirmBreakCircuit commits the pending unhealable move: the node
// extracts (circuit breaks), splices at the destination, mode commits.
func (g *Game) ConfirmBreakCircuit() {
	if !g.breakPending.active {
		return
	}
	n, i, j := g.breakPending.node, g.breakPending.i, g.breakPending.j
	g.breakPending = breakPendingState{}
	g.closeBreakConfirmPortal()
	if n == nil || g.nodeByID(n.ID) == nil {
		g.modes.Cancel("subject-deleted")
		return
	}
	if g.moveNode(n, i, j) {
		g.modes.Commit()
	} else {
		g.modes.Cancel("move-failed")
	}
}

// CancelBreakCircuit dismisses the dialog without touching the graph.
// Move mode STAYS ARMED — the user picks another destination or hits ESC.
func (g *Game) CancelBreakCircuit() {
	if !g.breakPending.active {
		return
	}
	g.breakPending = breakPendingState{}
	g.closeBreakConfirmPortal()
}
```

Plus `breakConfirmRect`/`breakConfirmBreakBtnRect`/`breakConfirmCancelBtnRect`/`openBreakConfirmPortal`/`closeBreakConfirmPortal`/`drawBreakConfirm` — copy the paste-confirm versions (`node_clipboard.go:219-300`) substituting: portal id, `g.breakPending.active` for `isOpenFn`, title `i18n.T(i18n.KeyBreakCircuitTitle)`, confirm-button label `i18n.T(i18n.KeyCapBreak)` drawn with the destructive fill (`colDeleteFill`/`colDeleteBorder`, dark label per `longPressDeleteTextColor()`), cancel button `i18n.T(i18n.KeyCancel)` neutral. Keep the `escapeFn` cleanup-only contract verbatim: `escapeFn: func() bool { g.breakPending = breakPendingState{}; return false }` with the same explanatory comment, and `OnClose: func() { g.breakPending = breakPendingState{} }`. Add the `breakPending breakPendingState` field beside `pastePending` (grep `pastePending pastePendingState` for the struct — it's on Game/DrumView side; mirror its placement).

(b) `handleArmedModeTap` move case — insert between the `GuideEdge` check and the commit:

```go
		if g.moveHealPlan(n).class == healUnhealable {
			// Breaking move: confirm before committing. Mode stays armed;
			// the portal owns the decision (Break → commit, Cancel → retry).
			g.beginBreakConfirm(n, gt.I, gt.J)
			return true
		}
```

(c) `game_input_editor.go` — at the top of the armed-mode block add:

```go
		// While the break-circuit dialog is open the portal owns input;
		// grid releases must not place/cancel behind it.
		if g.breakConfirmActive() {
			g.leftPrev = left
			return
		}
```

(d) `game_input_shortcuts.go` — add a rung directly below the paste-confirm rung (~line 410), ABOVE the armed-mode rung:

```go
	// Break-circuit confirmation cancel (its own rung; move mode stays armed).
	if g.breakConfirmActive() {
		g.CancelBreakCircuit()
		return true
	}
```

(e) `game_graph_nodes.go` prune rung (from the previous feature): extend the subject-deleted branch to also `g.CancelBreakCircuit()` BEFORE `g.modes.Cancel("subject-deleted")`.

(f) i18n keys + both catalogs as listed in Files.

- [ ] **Step 4: Run tests → PASS**, then the flow set: `-run 'TestMoveNode|TestMoveMode|TestGridMode|TestCopyMode|TestLongPress|TestMouseHold' ./internal/ui/` and full `./internal/i18n/`. Expected: PASS.

- [ ] **Step 5: Checkpoint** — report. Do NOT commit.

---

### Task 4: Verification sweep (controller)

- [ ] Full `./internal/ui/` with `-timeout 25m` (triage vs known external reds), `./internal/hooks/ ./internal/eventlogger/ ./internal/i18n/ ./internal/timeline/`.
- [ ] Guards: `-run 'TestDesignMDDrift|TestTokenDiscipline|TestEveryKindClassified|TestHooks_NumNonVerbose|TestNoUppercaseInfoTags' ./internal/ui/ ./internal/hooks/ ./internal/log/`.
- [ ] Browser: `GO=$(pwd)/.tools/go/bin/go node src/js/touch_gestures.browser.test.js` (Tests 6–8 must stay green — their mover is edge-free = silent class; known Test 1 red excepted) and `wasm_bridge_smoke.browser.test.js`.
- [ ] xvfb spot check: `xvfb-run -a ../../.tools/go/bin/go test -timeout 15m ./internal/ui/ -run 'TestMoveNode|TestMoveMode|TestMoveHealPlan'`.
- [ ] Report results; user commits.
