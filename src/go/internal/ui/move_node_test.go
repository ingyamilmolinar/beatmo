package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// hasEdgeBetween reports whether a UI edge exists between the two node IDs,
// in either direction. Defined in this untagged file (rather than the
// //go:build test grid_mode_flow_test.go) so it's available to both the fast
// ebitenstub path and the real-Ebiten build.
func hasEdgeBetween(g *Game, id1, id2 model.NodeID) bool {
	for _, e := range g.edges {
		if (e.A.ID == id1 && e.B.ID == id2) || (e.A.ID == id2 && e.B.ID == id1) {
			return true
		}
	}
	return false
}

// hasDirectedEdge reports whether a UI edge exists from fromID to toID,
// direction-sensitive (A==from, B==to only). Heal preserves the direction of
// the healed Np->Nn pair, so several extraction/heal tests need to assert the
// reverse direction is NOT present — hasEdgeBetween can't distinguish that.
func hasDirectedEdge(g *Game, fromID, toID model.NodeID) bool {
	for _, e := range g.edges {
		if e.A.ID == fromID && e.B.ID == toID {
			return true
		}
	}
	return false
}

// directedEdgeCount counts UI edges from fromID to toID (direction-sensitive).
// Used by the heal-dedup test to assert extraction+heal never produces a
// second parallel edge when a direct edge already exists.
func directedEdgeCount(g *Game, fromID, toID model.NodeID) int {
	n := 0
	for _, e := range g.edges {
		if e.A.ID == fromID && e.B.ID == toID {
			n++
		}
	}
	return n
}

func TestMoveNodeBasic(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 3, model.NodeTypeRegular)
	origID := n.ID

	// A bystander edge x-y that the destination (5,7) sits on the interior
	// of: moveNode must splice n into it (stitchEdgesAt), same as
	// tryAddNode/pasteNodeFrom do for a node placed onto an edge.
	x := g.tryAddNode(5, 0, model.NodeTypeRegular)
	y := g.tryAddNode(5, 14, model.NodeTypeRegular)
	g.addEdge(x, y)

	ok := g.moveNode(n, 5, 7)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	if n.I != 5 || n.J != 7 {
		t.Fatalf("expected (5,7), got (%d,%d)", n.I, n.J)
	}
	if n.ID != origID {
		t.Fatalf("NodeID changed: was %d, now %d", origID, n.ID)
	}
	// Verify graph model updated
	mn, ok2 := g.graph.GetNodeByID(origID)
	if !ok2 {
		t.Fatal("node missing from graph")
	}
	if mn.I != 5 || mn.J != 7 {
		t.Fatalf("graph node expected (5,7), got (%d,%d)", mn.I, mn.J)
	}
	// The bystander x-y edge must be split (stitched) around the moved node.
	if hasEdgeBetween(g, x.ID, y.ID) {
		t.Fatal("expected the x-y edge to be gone (stitched away by the move)")
	}
	if !hasEdgeBetween(g, x.ID, n.ID) || !hasEdgeBetween(g, n.ID, y.ID) {
		t.Fatal("expected stitched x-n and n-y edges")
	}
}

// TestMoveNodeExtractsAndHeals (was TestMoveNodePreservesEdges): b is a
// pass-through node (a->b->c, a and c orthogonal), moved to (6,0) — a point
// on the healed a-c span. Under extraction+heal semantics the old a->b/b->c
// edges are removed, the chain heals as a->c, and since b's NEW position
// lands on that healed span it immediately re-splices into a->b/b->c again
// (nodesOnSpan naturally includes the moved node — see grid_move_heal.go).
// The final topology is edge-count-identical to the old re-anchor behavior,
// but reached via extraction+heal+resplice, not a "keep surviving edges"
// re-add loop — assert the healed-then-resliced outcome directly, plus that
// no unsplit a->c edge survives.
func TestMoveNodeExtractsAndHeals(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a horizontal 3-node chain: a(0,0) -> b(4,0) -> c(8,0)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	c := g.tryAddNode(8, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()

	edgesBefore := len(g.edges)

	if plan := g.moveHealPlan(b); plan.class != healPassThrough {
		t.Fatalf("setup: expected b to classify as healPassThrough, got %v", plan.class)
	}

	// Move b along the same row, to a point interior to the a-c span.
	ok := g.moveNode(b, 6, 0)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	if !hasDirectedEdge(g, a.ID, b.ID) || !hasDirectedEdge(g, b.ID, c.ID) {
		t.Fatal("expected a->b and b->c restored via heal+resplice at b's new position")
	}
	if hasDirectedEdge(g, a.ID, c.ID) {
		t.Fatal("expected no unsplit a->c heal edge to survive (b sits on its span)")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("expected %d edges, got %d", edgesBefore, len(g.edges))
	}
}

// TestMoveNodeDropsNonOrthogonalEdges: b sits between a and c but a-c is NOT
// orthogonal (a.I=0,a.J=0 vs c.I=4,c.J=4), so moveHealPlan classifies b as
// healUnhealable — extraction still removes ALL of b's old edges regardless
// of orthogonality, but no heal edge is produced since the endpoints can't
// be closed without a dialog (Task 3 wires the confirm; a direct moveNode
// call here breaks silently, matching spec #13's programmatic-corner case).
func TestMoveNodeDropsNonOrthogonalEdges(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build: a(0,0) -> b(4,0), b(4,0) -> c(4,4)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	c := g.tryAddNode(4, 4, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()

	if plan := g.moveHealPlan(b); plan.class != healUnhealable {
		t.Fatalf("setup: expected b to classify as healUnhealable, got %v", plan.class)
	}

	// Move b to (2,2) - diagonal to both a and c, neither orthogonal
	ok := g.moveNode(b, 2, 2)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	if hasEdgeBetween(g, a.ID, b.ID) || hasEdgeBetween(g, b.ID, c.ID) {
		t.Fatal("expected b's old edges extracted (removed) regardless of orthogonality")
	}
	if hasEdgeBetween(g, a.ID, c.ID) {
		t.Fatal("expected no a-c heal edge (unhealable topology)")
	}
	if len(g.edges) != 0 {
		t.Fatalf("expected 0 edges after unhealable move, got %d", len(g.edges))
	}
}

func TestMoveNodeOccupiedBlocked(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)

	ok := g.moveNode(b, 0, 0) // occupied by a
	if ok {
		t.Fatal("moveNode should return false for occupied destination")
	}
	// b should remain at original position
	if b.I != 4 || b.J != 0 {
		t.Fatalf("node should not have moved, got (%d,%d)", b.I, b.J)
	}
}

func TestMoveNodeDuringPlayback(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.SetPlaying(true)
	advanceFrames(g, 10)

	ok := g.moveNode(b, 8, 0)
	if !ok {
		t.Fatal("moveNode during playback returned false")
	}

	advanceFrames(g, 10)
	if !g.Playing() {
		t.Fatal("playback stopped after move")
	}
	stopPlaybackForTest(g)
}

func TestMoveNodeDrumRowOriginPreserved(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()

	// Move the origin node
	ok := g.moveNode(a, 2, 0)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	// Origin should still point to the same NodeID
	if g.drum.Rows[0].Origin != a.ID {
		t.Fatalf("drum row origin changed: expected %d, got %d", a.ID, g.drum.Rows[0].Origin)
	}
}

// TestMoveButtonClickDoesNotImmediatelyPlace verifies that clicking the MOVE
// button and then releasing the mouse does NOT place the node on the release.
// The user must perform a separate click to choose the destination.
func TestMoveButtonClickDoesNotImmediatelyPlace(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Place a node and enter move mode directly (desktop-only button).
	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.modes.Enter(GridModeMove, n)
	g.moveSkipRelease = true
	g.sidebar.Close()

	origI, origJ := n.I, n.J

	// Pick a position in the grid pane for the simulated mouse.
	cx, cy := 400, 150

	// Frame 1: press held — moveSkipRelease active.
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	_ = g.Update()
	if !g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should be true")
	}
	if !g.moveSkipRelease {
		t.Fatal("moveSkipRelease should be true while pressed")
	}

	// Frame 2: release — this should be eaten by moveSkipRelease guard,
	// NOT interpreted as a destination click.
	pressed = false
	_ = g.Update()

	if !g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should still be true after button release")
	}
	if g.moveSkipRelease {
		t.Fatal("moveSkipRelease should be cleared after release")
	}
	if n.I != origI || n.J != origJ {
		t.Fatalf("node moved on button release: (%d,%d) -> (%d,%d)",
			origI, origJ, n.I, n.J)
	}
}

// TestMoveButtonRemovedFromMobilePopup verifies that on a small screen the
// node popup menu does NOT contain a "Move Node" button rect (moving is
// provided via copy/paste and the long-press popup instead). The desktop
// counterpart is TestNodePopupHasNoMoveButton.
func TestMoveButtonRemovedFromMobilePopup(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	if _, ok := g.sidebar.laidOutID("move"); ok {
		t.Fatal("the sidebar must have no Move control")
	}
}

func TestMoveModeCancelEsc(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.modes.Enter(GridModeMove, n)

	if !g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should be true")
	}

	g.modes.Cancel("test")

	if g.modes.Is(GridModeMove) {
		t.Fatal("moveMode should be false after cancel")
	}
	if g.modes.Subject() != nil {
		t.Fatal("movingNode should be nil after cancel")
	}
}

// --- Extraction + heal spec matrix (Task 2) --------------------------------

// TestMoveNode_HealsStraightChain (spec #1): A(0,0)->N(0,4)->B(0,8) plus a
// separate edge C(6,0)->D(6,8). moveNode(N, 6, 4) heals A->B (N's old chain
// closes behind it) and splices N into the C-D edge at its new position.
func TestMoveNode_HealsStraightChain(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n := g.tryAddNode(0, 4, model.NodeTypeRegular)
	b := g.tryAddNode(0, 8, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)

	c := g.tryAddNode(6, 0, model.NodeTypeRegular)
	d := g.tryAddNode(6, 8, model.NodeTypeRegular)
	g.addEdge(c, d)
	g.updateBeatInfos()

	if ok := g.moveNode(n, 6, 4); !ok {
		t.Fatal("moveNode returned false")
	}

	if !hasDirectedEdge(g, a.ID, b.ID) {
		t.Fatal("expected the healed A->B edge to exist")
	}
	if hasEdgeBetween(g, a.ID, n.ID) || hasEdgeBetween(g, n.ID, b.ID) {
		t.Fatal("expected A-N and N-B gone (extracted)")
	}
	if hasEdgeBetween(g, c.ID, d.ID) {
		t.Fatal("expected the C-D edge gone (spliced by N's destination)")
	}
	if !hasDirectedEdge(g, c.ID, n.ID) || !hasDirectedEdge(g, n.ID, d.ID) {
		t.Fatal("expected N spliced into C->N->D at its new position")
	}
}

// TestMoveNode_HealPreservesDirection (spec #2): B(0,8)->N(0,4)->A(0,0);
// after the move, the healed edge is B->A, never A->B.
func TestMoveNode_HealPreservesDirection(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	b := g.tryAddNode(0, 8, model.NodeTypeRegular)
	n := g.tryAddNode(0, 4, model.NodeTypeRegular)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.addEdge(b, n)
	g.addEdge(n, a)
	g.updateBeatInfos()

	if ok := g.moveNode(n, 6, 6); !ok {
		t.Fatal("moveNode returned false")
	}

	if !hasDirectedEdge(g, b.ID, a.ID) {
		t.Fatal("expected the healed B->A edge (direction preserved)")
	}
	if hasDirectedEdge(g, a.ID, b.ID) {
		t.Fatal("expected NO A->B edge (that would reverse the healed direction)")
	}
}

// TestMoveNode_HealStitchesThroughIntermediate (spec #3): A(0,0)->N(0,6)->B(0,12)
// with bystander M(0,3) (no edges of its own). Moving N away heals the chain
// as A->M and M->B (split at the bystander), never a direct A->B edge.
func TestMoveNode_HealStitchesThroughIntermediate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	m := g.tryAddNode(0, 3, model.NodeTypeRegular)
	n := g.tryAddNode(0, 6, model.NodeTypeRegular)
	b := g.tryAddNode(0, 12, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	g.updateBeatInfos()

	if ok := g.moveNode(n, 9, 9); !ok {
		t.Fatal("moveNode returned false")
	}

	if !hasDirectedEdge(g, a.ID, m.ID) || !hasDirectedEdge(g, m.ID, b.ID) {
		t.Fatal("expected the healed chain split through the bystander: A->M and M->B")
	}
	if hasDirectedEdge(g, a.ID, b.ID) {
		t.Fatal("expected NO direct unsplit A->B edge")
	}
}

// TestMoveNode_HealDedupes (spec #4): A->N, N->B, AND a pre-existing direct
// A->B edge. Moving N away must not produce a second parallel A->B edge.
func TestMoveNode_HealDedupes(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n := g.tryAddNode(0, 6, model.NodeTypeRegular)
	b := g.tryAddNode(0, 12, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	// Pre-existing direct A->B edge, coexisting with the A-N-B chain.
	g.addEdge(a, b)
	g.updateBeatInfos()

	if ok := g.moveNode(n, 9, 9); !ok {
		t.Fatal("moveNode returned false")
	}

	if got := directedEdgeCount(g, a.ID, b.ID); got != 1 {
		t.Fatalf("expected exactly 1 A->B edge after heal-dedupe, got %d", got)
	}
}

// TestMoveNode_EndpointSilent (spec #6, #7): a node with a single edge has no
// heal (silent extraction) and splices at its destination like any other
// node placement; an isolated node with no edges moves fine.
//
// A(0,0)-N and the C-D span are deliberately placed on the SAME column
// (I=0) as N's destination, so N's own A-N edge would REMAIN orthogonal at
// the new position. Extraction must still drop it unconditionally — under
// the old re-anchor semantics this edge would have survived (re-added
// because it stayed orthogonal), which is exactly the "re-anchoring
// retired" behavior this test pins.
func TestMoveNode_EndpointSilent(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n := g.tryAddNode(0, 4, model.NodeTypeRegular)
	g.addEdge(a, n)

	c := g.tryAddNode(0, 10, model.NodeTypeRegular)
	d := g.tryAddNode(0, 20, model.NodeTypeRegular)
	g.addEdge(c, d)
	g.updateBeatInfos()

	if plan := g.moveHealPlan(n); plan.class != healSilent {
		t.Fatalf("setup: expected n (single edge) to classify healSilent, got %v", plan.class)
	}

	if ok := g.moveNode(n, 0, 15); !ok {
		t.Fatal("moveNode returned false")
	}
	if hasEdgeBetween(g, a.ID, n.ID) {
		t.Fatal("expected A-N edge gone (extracted), even though it would still be orthogonal at the new position — no re-anchoring")
	}
	if hasEdgeBetween(g, c.ID, d.ID) {
		t.Fatal("expected C-D gone (spliced by N's destination)")
	}
	if !hasDirectedEdge(g, c.ID, n.ID) || !hasDirectedEdge(g, n.ID, d.ID) {
		t.Fatal("expected N spliced into C->N->D at its new position")
	}

	// Spec #7: an isolated node (no edges at all) moves without incident.
	iso := g.tryAddNode(20, 20, model.NodeTypeRegular)
	if plan := g.moveHealPlan(iso); plan.class != healSilent {
		t.Fatalf("setup: expected isolated node to classify healSilent, got %v", plan.class)
	}
	if ok := g.moveNode(iso, 21, 21); !ok {
		t.Fatal("moveNode returned false for isolated node")
	}
	if iso.I != 21 || iso.J != 21 {
		t.Fatalf("expected isolated node at (21,21), got (%d,%d)", iso.I, iso.J)
	}
}

// TestMoveNode_ProgrammaticCornerBreaksSilently (spec #13): a square-corner
// A->N->B (perpendicular, unhealable) breaks silently under a direct
// moveNode call — both edges gone, no heal edge, no panic. The interactive
// confirm dialog for this case is Task 3's job; direct moveNode calls bypass
// the GuideEdge-gated interactive path entirely.
//
// Destination (0,9) is deliberately chosen so N's A-N edge would REMAIN
// orthogonal at the new position (A and the destination share I=0) while
// N-B would not — under the old re-anchor semantics this would have kept
// A-N alive (re-added per-edge) while dropping N-B, a partial "half heal".
// Extraction must drop BOTH regardless, since the pair as a whole is
// unhealable.
func TestMoveNode_ProgrammaticCornerBreaksSilently(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 4, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	g.updateBeatInfos()

	if plan := g.moveHealPlan(n); plan.class != healUnhealable {
		t.Fatalf("setup: expected corner topology to classify healUnhealable, got %v", plan.class)
	}

	ok := g.moveNode(n, 0, 9)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	if hasEdgeBetween(g, a.ID, n.ID) || hasEdgeBetween(g, n.ID, b.ID) {
		t.Fatal("expected both of N's old edges gone (extracted), even though A-N alone would still be orthogonal")
	}
	if hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("expected no A-B heal edge (unhealable topology)")
	}
}
