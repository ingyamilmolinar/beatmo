package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestMultipleProbabilityNodes_SchedulerMatchesPrediction verifies that with
// several probability nodes in a single circuit (plus every/skip on others),
// the audio scheduler consults the single prediction source of truth and the
// drum view mirrors that exact state.
func TestMultipleProbabilityNodes_WindowsMatchPrediction(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(900, 600)
	// Build 6-node ring: A->B->C->D->E->F->A
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(1, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 0, model.NodeTypeRegular)
	D := g.tryAddNode(3, 0, model.NodeTypeRegular)
	E := g.tryAddNode(4, 0, model.NodeTypeRegular)
	F := g.tryAddNode(5, 0, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, E)
	g.addEdge(E, F)
	g.addEdge(F, A)
	// Probabilities on A, C, E; Every/skip on B, D; F regular
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.33
		g.graph.SetNodeParams(A.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(B.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(B.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(C.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.5
		g.graph.SetNodeParams(C.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(D.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(D.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(E.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.25
		g.graph.SetNodeParams(E.ID, p)
	}
	g.start = A
	g.graph.StartNodeID = A.ID
	horizon := 180
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	g.engine.Predictor.Ensure(horizon)
	// Drum should mirror prediction windows.
	g.drum.Offset = 0
	g.refreshDrumRow()
	gotSteps := append([]bool(nil), g.drum.Rows[0].Steps...)
	wantSteps := make([]bool, horizon)
	for i := 0; i < horizon; i++ {
		wantSteps[i] = g.engine.Predictor.VisibleAt(0, i)
	}
	for i := 0; i < horizon; i++ {
		if gotSteps[i] != wantSteps[i] {
			t.Fatalf("drum vs predVisible mismatch at %d: got=%v want=%v", i, gotSteps[i], wantSteps[i])
		}
	}
	// Also check multiple windows across offsets
	seg := g.loopLenByRow[0]
	start := g.loopStartByRow[0]
	win := 24
	g.drum.SetLength(win)
	for off := 0; off+win <= horizon; off += 9 {
		if seg > 0 {
			k0 := (off - (start + 1)) % seg
			if k0 < 0 {
				k0 += seg
			}
			if seg-k0 <= win {
				continue
			}
		}
		g.drum.Offset = off
		g.refreshDrumRow()
		gotW := append([]bool(nil), g.drum.Rows[0].Steps...)
		for i := 0; i < win; i++ {
			want := g.engine.Predictor.VisibleAt(0, off+i)
			if gotW[i] != want {
				t.Fatalf("window off=%d idx=%d mismatch: got=%v want=%v", off, i, gotW[i], want)
			}
		}
	}
}
