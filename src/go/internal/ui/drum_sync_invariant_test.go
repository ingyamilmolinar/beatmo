package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func assertRowMatchesPredictor(t *testing.T, g *Game, label string, row int) {
	t.Helper()
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("%s: engine predictor unavailable", label)
	}
	need := g.drum.Offset + g.drum.Length
	if need < g.drum.Length {
		need = g.drum.Length
	}
	g.engine.Predictor.Ensure(need)
	steps := g.drum.Rows[row].Steps
	for i, on := range steps {
		abs := g.drum.Offset + i
		bi := g.beatInfoAtRow(row, abs)
		var want bool
		switch bi.NodeType {
		case model.NodeTypeRegular:
			want = g.engine.Predictor.VisibleAt(row, abs)
		case model.NodeTypeMute:
			want = g.engine.Predictor.TriggeredAt(row, abs)
		default:
			want = false
		}
		if on != want {
			t.Fatalf("%s: row=%d abs=%d node=%d type=%v step=%v want=%v", label, row, abs, bi.NodeID, bi.NodeType, on, want)
		}
		if bi.NodeType == model.NodeTypeRegular {
			if audible := g.engine.Predictor.AudibleAt(row, abs); audible != want {
				t.Fatalf("%s: row=%d abs=%d audible=%v want=%v", label, row, abs, audible, want)
			}
		}
	}
}

func findAbsForNode(g *Game, row int, id model.NodeID) int {
	if id == model.InvalidNodeID {
		return -1
	}
	limit := g.drum.Offset + g.drum.Length*2
	for abs := g.drum.Offset; abs <= limit; abs++ {
		if info := g.beatInfoAtRow(row, abs); info.NodeID == id {
			return abs
		}
	}
	return -1
}

func TestNodeLogicUpdatesRefreshDrumView(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.SetBeatLength(16)
	g.drum.Offset = 0

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if a == nil || b == nil {
		t.Fatalf("failed to create baseline nodes")
	}
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.refreshDrumRow()

	targetAbs := findAbsForNode(g, 0, b.ID)
	if targetAbs < 0 {
		t.Fatalf("could not locate target node in beat path")
	}
	targetRel := targetAbs - g.drum.Offset
	if targetRel < 0 || targetRel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("relative index out of range (%d / %d)", targetRel, len(g.drum.Rows[0].Steps))
	}
	if !g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("expected initial step to be active at abs=%d", targetAbs)
	}

	node, ok := g.graph.GetNodeByID(b.ID)
	if !ok {
		t.Fatalf("graph node %d missing", b.ID)
	}
	params := node.Params
	params.LogicKind = "probability"
	params.LogicP = 0
	g.graph.SetNodeParams(b.ID, params)

	if targetRel < len(g.drum.Rows[0].Steps) && g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("logic change did not update drum steps at abs=%d rel=%d", targetAbs, targetRel)
	}
	assertRowMatchesPredictor(t, g, "logic update", 0)
}

func TestCircuitEditsRefreshDrumView(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.SetBeatLength(24)
	g.drum.Offset = 0

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil {
		t.Fatalf("failed to create nodes for circuit test")
	}
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)
	g.refreshDrumRow()

	targetAbs := findAbsForNode(g, 0, b.ID)
	if targetAbs < 0 {
		t.Fatalf("initial beat path missing middle node")
	}
	targetRel := targetAbs - g.drum.Offset
	if targetRel < 0 || targetRel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("relative index out of range (%d / %d)", targetRel, len(g.drum.Rows[0].Steps))
	}
	if !g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("expected middle step active before delete")
	}

	g.deleteNode(b)
	if targetRel < len(g.drum.Rows[0].Steps) && g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("step remained active after deleting node at rel=%d", targetRel)
	}

	re := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if re == nil {
		t.Fatalf("failed to re-add middle node")
	}
	aNode := g.nodeAt(0, 0)
	cNode := g.nodeAt(2, 0)
	if aNode == nil || cNode == nil {
		t.Fatalf("failed to locate endpoints after re-add")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{aNode.ID, cNode.ID}]; ok {
		g.deleteEdge(aNode, cNode)
	}
	g.addEdge(aNode, re)
	g.addEdge(re, cNode)

	targetAbs2 := findAbsForNode(g, 0, re.ID)
	if targetAbs2 < 0 {
		t.Fatalf("re-added node missing from beat path")
	}
	targetRel2 := targetAbs2 - g.drum.Offset
	if targetRel2 < 0 || targetRel2 >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("relative index out of range after re-add (%d / %d)", targetRel2, len(g.drum.Rows[0].Steps))
	}
	if !g.drum.Rows[0].Steps[targetRel2] {
		t.Fatalf("step did not reactivate after re-adding node at abs=%d rel=%d", targetAbs2, targetRel2)
	}
	assertRowMatchesPredictor(t, g, "circuit re-add", 0)
}
