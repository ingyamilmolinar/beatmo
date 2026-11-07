//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Import rebuilds the graph wholesale, so the coordinate badge must follow the node's lifecycle and go with it.
func TestUndoCreateNodeClearsCoordBadge(t *testing.T) {
	g := newTestGameForUndo(t)
	g.undoManager.OnExternalLoad()

	g.tryAddNode(7, 7, model.NodeTypeRegular)
	if g.coordBadgeNode == nil {
		t.Fatal("precondition: creating a regular node should arm the coordinate badge")
	}

	g.undoManager.Undo()

	if g.nodeAt(7, 7) != nil {
		t.Fatal("precondition: undo should have deleted the created node")
	}
	if g.coordBadgeNode != nil {
		t.Fatalf("coordinate badge still references a node after undoing its creation " +
			"(node deleted but coordinates linger)")
	}
}

// It cannot keep pointing at a node that no longer exists.
func TestUndoCreateNodeClosesNodeMenu(t *testing.T) {
	g := newTestGameForUndo(t)
	g.undoManager.OnExternalLoad()

	g.tryAddNode(8, 8, model.NodeTypeRegular)
	node := g.nodeAt(8, 8)
	if node == nil {
		t.Fatal("failed to add node")
	}
	g.sidebar.Open(node)
	if !g.sidebar.IsOpen() {
		t.Fatal("precondition: node menu should be open")
	}

	g.undoManager.Undo()

	if g.nodeAt(8, 8) != nil {
		t.Fatal("precondition: undo should have deleted the created node")
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("node menu still open after undoing the node's creation " +
			"(menu points at a deleted node)")
	}
}

// The tie is not undo-specific.
func TestDeleteNodeClosesMenuAndBadge(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(9, 9, model.NodeTypeRegular)
	node := g.nodeAt(9, 9)
	if node == nil {
		t.Fatal("failed to add node")
	}
	g.sidebar.Open(node)
	g.coordBadgeNode = node

	g.deleteNode(node)

	if g.coordBadgeNode != nil {
		t.Fatal("coordinate badge lingers after the node was deleted")
	}
	if g.sidebar.IsOpen() {
		t.Fatal("node menu lingers open after the node was deleted")
	}
}

// A node that survives a rebuild (same id, new pointer) keeps its badge, repointed to the live node.
func TestPruneRepointsCoordBadgeForSurvivingNode(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(10, 10, model.NodeTypeRegular)
	node := g.nodeAt(10, 10)
	if node == nil {
		t.Fatal("failed to add node")
	}
	g.coordBadgeNode = node
	id := node.ID

	// Export then import rebuilds every node with a fresh *uiNode, ids preserved.
	snap := g.undoCapture()
	if err := g.Import(snap); err != nil {
		t.Fatalf("import: %v", err)
	}

	live := g.nodeAt(10, 10)
	if live == nil {
		t.Fatal("precondition: node should survive the round-trip")
	}
	if g.coordBadgeNode == nil {
		t.Fatal("coordinate badge wrongly cleared for a node that still exists")
	}
	if g.coordBadgeNode != live || g.coordBadgeNode.ID != id {
		t.Fatalf("coordinate badge not repointed to the live node (got %v, want %v)", g.coordBadgeNode, live)
	}
}
