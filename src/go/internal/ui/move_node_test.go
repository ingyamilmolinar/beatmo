package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestMoveNodeBasic(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 3, model.NodeTypeRegular)
	origID := n.ID

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
}

func TestMoveNodePreservesEdges(t *testing.T) {
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

	// Move b along the same row (still orthogonal with a and c)
	ok := g.moveNode(b, 6, 0)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	// Both edges should be preserved since they remain horizontal
	if len(g.edges) != edgesBefore {
		t.Fatalf("expected %d edges, got %d", edgesBefore, len(g.edges))
	}
}

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

	// Move b to (2,2) - diagonal to both a and c, neither orthogonal
	ok := g.moveNode(b, 2, 2)
	if !ok {
		t.Fatal("moveNode returned false")
	}
	if len(g.edges) != 0 {
		t.Fatalf("expected 0 edges after non-orthogonal move, got %d", len(g.edges))
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

	g.SetPlayFunc(func(string, float64, ...float64) {})
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

func TestMoveNodeEdgeLossPreview(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	c := g.tryAddNode(4, 4, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()

	// Move b to (2,2) - both edges become non-orthogonal
	loss := g.moveNodeEdgeLoss(b, 2, 2)
	if loss != 2 {
		t.Fatalf("expected 2 edge loss, got %d", loss)
	}

	// Move b to (2,0) - edge to a stays (same row), edge to c dropped
	loss2 := g.moveNodeEdgeLoss(b, 2, 0)
	if loss2 != 1 {
		t.Fatalf("expected 1 edge loss, got %d", loss2)
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
	g.moveMode = true
	g.movingNode = n
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
	if !g.moveMode {
		t.Fatal("moveMode should be true")
	}
	if !g.moveSkipRelease {
		t.Fatal("moveSkipRelease should be true while pressed")
	}

	// Frame 2: release — this should be eaten by moveSkipRelease guard,
	// NOT interpreted as a destination click.
	pressed = false
	_ = g.Update()

	if !g.moveMode {
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
// node popup menu does NOT contain a "Move Node" button rect (since it's
// provided via the long-press popup instead).
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

	moveRect := g.sidebar.rects["move"]
	if !moveRect.Empty() {
		t.Fatalf("move rect should be empty on mobile, got %v", moveRect)
	}
}

func TestMoveModeCancelEsc(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.moveMode = true
	g.movingNode = n

	if !g.moveMode {
		t.Fatal("moveMode should be true")
	}

	g.cancelMoveMode()

	if g.moveMode {
		t.Fatal("moveMode should be false after cancel")
	}
	if g.movingNode != nil {
		t.Fatal("movingNode should be nil after cancel")
	}
}
