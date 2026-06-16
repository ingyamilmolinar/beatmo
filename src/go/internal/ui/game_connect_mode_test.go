//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestEnterConnectMode verifies that entering connect mode sets the expected
// state and deselects any previously selected node.
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

	if !g.connectMode {
		t.Fatal("connectMode should be true")
	}
	if g.connectFromNode != nodeB {
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

// TestEnterConnectModeNoPriorSelection verifies entering connect mode when no
// node was previously selected.
func TestEnterConnectModeNoPriorSelection(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = nil // Ensure no prior selection.

	g.enterConnectMode(node)

	if !g.connectMode {
		t.Fatal("connectMode should be true")
	}
	if g.connectFromNode != node {
		t.Fatal("connectFromNode should be set")
	}
	if !node.Selected {
		t.Fatal("node should be Selected")
	}
}

// TestHandleConnectModeTapEmptyGrid verifies that tapping empty grid space
// cancels connect mode.
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

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after tapping empty space")
	}
	if g.connectFromNode != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
}

// TestHandleConnectModeTapSelfTap verifies that tapping the source node itself
// is a no-op (connect mode remains active, no edge created).
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

	if !g.connectMode {
		t.Fatal("connectMode should still be active after self-tap")
	}
	if g.connectFromNode != node {
		t.Fatal("connectFromNode should still be the source node")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created; edges=%d want %d", len(g.edges), edgesBefore)
	}
}

// TestHandleConnectModeTapInvisibleNodePosition verifies that tapping at the
// position of an invisible node (far from any visible node) is treated as an
// empty-space tap (because nodeAtScreen skips invisible nodes), which cancels
// connect mode.
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

	// Tap at the invisible node's position. nodeAtScreen skips invisible
	// nodes, so this acts like an empty-space tap and cancels connect mode.
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(invis)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled (invisible node is not a valid target)")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created; edges=%d want %d", len(g.edges), edgesBefore)
	}
}

// TestHandleConnectModeTapNonPerpendicular verifies that tapping a diagonal
// node shows an error and keeps connect mode active.
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

	if !g.connectMode {
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

// TestHandleConnectModeTapSuccessfulConnect verifies that tapping a
// perpendicular node creates an edge and exits connect mode.
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

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after successful edge")
	}
	if g.connectFromNode != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
	if len(g.edges) != edgesBefore+1 {
		t.Fatalf("expected %d edges, got %d", edgesBefore+1, len(g.edges))
	}
}

// TestCancelConnectMode verifies that cancelConnectMode resets all state.
func TestCancelConnectMode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	node := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(node)
	if !g.connectMode {
		t.Fatal("precondition: connectMode should be true")
	}

	g.cancelConnectMode()

	if g.connectMode {
		t.Fatal("connectMode should be false after cancel")
	}
	if g.connectFromNode != nil {
		t.Fatal("connectFromNode should be nil after cancel")
	}
}

// TestDrawConnectModeNotActive verifies that drawConnectMode is a no-op when
// connect mode is not active.
func TestDrawConnectModeNotActive(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dst := ebiten.NewImage(640, 480)
	// connectMode is false by default.
	assertNotPanics(t, func() {
		g.drawConnectMode(dst)
	})
}

// TestDrawConnectModeNilFromNode verifies that drawConnectMode returns early
// when connectMode is true but connectFromNode is nil.
func TestDrawConnectModeNilFromNode(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.connectMode = true
	g.connectFromNode = nil

	dst := ebiten.NewImage(640, 480)
	assertNotPanics(t, func() {
		g.drawConnectMode(dst)
	})
}

// TestDrawConnectModeActive verifies that drawConnectMode runs without error
// when connect mode is active with a valid source node.
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
