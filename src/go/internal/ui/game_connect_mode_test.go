//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestEnterConnectMode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	nodeA := g.tryAddNode(0, 0, model.NodeTypeRegular)
	nodeB := g.tryAddNode(1, 0, model.NodeTypeRegular)

	// Select node A first.
	g.sel = nodeA
	nodeA.Selected = true

	// Enter connect mode from node B.
	g.enterConnectMode(nodeB)

	if !g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be true")
	}
	if g.modes.Subject() != nodeB {
		t.Fatal("connectFromNode should be nodeB")
	}
	if g.sel != nodeB {
		t.Fatal("sel should be nodeB")
	}
	if !nodeB.Selected {
		t.Fatal("nodeB should be Selected")
	}
	// Previous selection (nodeA) should be deselected.
	if nodeA.Selected {
		t.Fatal("nodeA should be deselected after entering connect mode from nodeB")
	}
}

func TestEnterConnectModeNoPriorSelection(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = nil // Ensure no prior selection.

	g.enterConnectMode(node)

	if !g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be true")
	}
	if g.modes.Subject() != node {
		t.Fatal("connectFromNode should be set")
	}
	if !node.Selected {
		t.Fatal("node should be Selected")
	}
}

func TestHandleConnectModeTapEmptyGrid(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(node)

	// Tap far from any node.
	g.handleConnectModeTap(600, 400)

	if g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be cancelled after tapping empty space")
	}
	if g.modes.Subject() != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
}

// No edge, and connect mode stays active.
func TestHandleConnectModeTapSelfTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	edgesBefore := len(g.edges)
	g.enterConnectMode(node)

	// Tap on the source node itself.
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(node)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if !g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should still be active after self-tap")
	}
	if g.modes.Subject() != node {
		t.Fatal("connectFromNode should still be the source node")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created; edges=%d want %d", len(g.edges), edgesBefore)
	}
}

// nodeAtScreen skips invisible nodes, so this is an empty-space tap.
func TestHandleConnectModeTapInvisibleNodePosition(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	from := g.tryAddNode(0, 0, model.NodeTypeRegular)
	// Place invisible node far from the source to avoid hit-test overlap.
	invis := g.tryAddNode(20, 0, model.NodeTypeInvisible)
	g.enterConnectMode(from)
	edgesBefore := len(g.edges)

	sx1, sy1, sx2, sy2 := g.nodeScreenRect(invis)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be cancelled (invisible node is not a valid target)")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created; edges=%d want %d", len(g.edges), edgesBefore)
	}
}

// An error shows and connect mode stays active.
func TestHandleConnectModeTapNonPerpendicular(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	from := g.tryAddNode(0, 0, model.NodeTypeRegular)
	diag := g.tryAddNode(3, 3, model.NodeTypeRegular)
	g.enterConnectMode(from)
	edgesBefore := len(g.edges)
	notifsBefore := g.drum.notifStore.Len()

	sx1, sy1, sx2, sy2 := g.nodeScreenRect(diag)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if !g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should still be active after non-perpendicular tap")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created for diagonal; edges=%d want %d", len(g.edges), edgesBefore)
	}
	if g.drum.notifStore.Len() <= notifsBefore {
		t.Fatal("expected an error notification for non-perpendicular connection")
	}
	last := g.drum.notifStore.Latest()
	if !last.isErr {
		t.Fatal("notification should be an error")
	}
}

func TestHandleConnectModeTapSuccessfulConnect(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	from := g.tryAddNode(0, 0, model.NodeTypeRegular)
	target := g.tryAddNode(1, 0, model.NodeTypeRegular)
	edgesBefore := len(g.edges)
	g.enterConnectMode(from)

	sx1, sy1, sx2, sy2 := g.nodeScreenRect(target)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be cancelled after successful edge")
	}
	if g.modes.Subject() != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
	if len(g.edges) != edgesBefore+1 {
		t.Fatalf("expected %d edges, got %d", edgesBefore+1, len(g.edges))
	}
}

func TestCancelConnectMode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(node)
	if !g.modes.Is(GridModeConnect) {
		t.Fatal("precondition: connectMode should be true")
	}

	g.modes.Cancel("test")

	if g.modes.Is(GridModeConnect) {
		t.Fatal("connectMode should be false after cancel")
	}
	if g.modes.Subject() != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
}

func TestDrawConnectModeNotActive(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dst := ebiten.NewImage(640, 480)
	// connect mode is inactive by default (g.modes.Active() == GridModeNone).
	assertNotPanics(t, func() {
		g.drawConnectMode(dst)
	})
}

func TestDrawConnectModeActive(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(node)

	dst := ebiten.NewImage(640, 480)
	assertNotPanics(t, func() {
		g.drawConnectMode(dst)
	})
}
