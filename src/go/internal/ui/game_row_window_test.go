package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestRowWindowIncludesTypes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv: %v", err)
	}

	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(root, next)
	g.pendingStartRow = -1
	g.start = root
	g.graph.StartNodeID = root.ID

	g.updateBeatInfos()
	g.refreshDrumRow()

	win := g.rowWindow(0)
	if win.Offset != g.drum.Offset {
		t.Fatalf("unexpected offset %d want %d", win.Offset, g.drum.Offset)
	}
	if len(win.Steps) == 0 {
		t.Fatalf("expected steps populated")
	}
	if len(win.Types) != len(win.Steps) {
		t.Fatalf("types length %d mismatch steps %d", len(win.Types), len(win.Steps))
	}
	foundRegular := false
	for i := range win.Steps {
		if win.Steps[i] && win.Types[i] == model.NodeTypeRegular {
			foundRegular = true
			break
		}
	}
	if !foundRegular {
		t.Fatalf("expected at least one regular cell in window")
	}
}
