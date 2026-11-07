package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Untagged, so both the stub and the real-Ebiten builds have it.
func hasEdgeBetween(g *Game, id1, id2 model.NodeID) bool {
	for _, e := range g.edges {
		if (e.A.ID == id1 && e.B.ID == id2) || (e.A.ID == id2 && e.B.ID == id1) {
			return true
		}
	}
	return false
}

// Heal keeps the healed pair's direction, so some tests must assert the reverse edge is absent.
func hasDirectedEdge(g *Game, fromID, toID model.NodeID) bool {
	for _, e := range g.edges {
		if e.A.ID == fromID && e.B.ID == toID {
			return true
		}
	}
	return false
}

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

	// The destination sits inside the bystander edge x-y, so moveNode must splice n in, as a placed node would.
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

// Moving onto the healed a-c span re-splices b into it: extraction, heal and re-splice, with no unsplit a->c edge
// left behind.
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

// a-c is not orthogonal, so b is unhealable: extraction drops all its edges and no heal edge appears without the
// confirm dialog.
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

// The release that follows the MOVE press must not place the node; the destination is a separate click.
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

	// Eaten by the moveSkipRelease guard.
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

// Mobile moves through copy/paste and the long-press popup instead.
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

// N's old chain heals behind it and N splices into the C-D edge at its destination.
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

// Split at the bystander M, never a direct A->B.
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

// A single-edge node moves without a heal; an isolated node moves fine. A-N would stay orthogonal at the
// destination, and extraction must still drop it: edges are never re-anchored.
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

// An unhealable corner breaks silently on a direct moveNode call, which bypasses the interactive confirm. A-N
// would stay orthogonal at the destination, yet both edges go: no half heal.
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
