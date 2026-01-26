package ui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestRowSnapshotMode_KeepsOldWindowImmutable(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultRowSnapshotMode(t, g)
	g.SetRowSnapshotModeForTest(true)
	g.drum.SetFollow(false)
	g.drum.SetLength(16)

	// Simple A<->B loop so skip_every_n on B changes the rendered window.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	g.updateBeatInfos()
	g.refreshDrumRow()

	steps1 := g.drum.Rows[0].Steps
	types1 := g.drum.Rows[0].CellTypes
	if len(steps1) == 0 || len(types1) == 0 {
		t.Fatalf("expected non-empty row window")
	}
	steps1Ptr := reflect.ValueOf(steps1).Pointer()
	types1Ptr := reflect.ValueOf(types1).Pointer()
	steps1Copy := append([]bool(nil), steps1...)
	types1Copy := append([]model.NodeType(nil), types1...)

	nodeB, ok := g.graph.GetNodeByID(b.ID)
	if !ok {
		t.Fatalf("node B missing")
	}
	p := nodeB.Params
	p.LogicKind = "skip_every_n"
	p.LogicN = 1                   // deterministic "never fire"
	g.graph.SetNodeParams(b.ID, p) // triggers refreshDrumRow via node-change hook

	steps2 := g.drum.Rows[0].Steps
	types2 := g.drum.Rows[0].CellTypes
	if len(steps2) == 0 || len(types2) == 0 {
		t.Fatalf("expected non-empty row window after edit")
	}
	if steps1Ptr == reflect.ValueOf(steps2).Pointer() {
		t.Fatalf("expected Steps slice replaced under snapshot mode")
	}
	if types1Ptr == reflect.ValueOf(types2).Pointer() {
		t.Fatalf("expected CellTypes slice replaced under snapshot mode")
	}
	if !slices.Equal(steps1, steps1Copy) {
		t.Fatalf("expected original steps window to remain immutable; before=%v after=%v", steps1Copy, steps1)
	}
	if !slices.Equal(types1, types1Copy) {
		t.Fatalf("expected original types window to remain immutable; before=%v after=%v", types1Copy, types1)
	}
	if slices.Equal(steps1Copy, steps2) {
		t.Fatalf("expected rendered steps to change after edit; steps=%v", steps2)
	}
}
