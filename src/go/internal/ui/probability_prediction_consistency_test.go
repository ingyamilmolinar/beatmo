package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"math"
	"testing"
)

// buildProbChain builds a 2-node loop A->S->A with A regular and S silent.
func buildProbChain(g *Game, p float64) (A, B *uiNode) {
	A = g.tryAddNode(0, 0, model.NodeTypeRegular)
	B = g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, A)
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		pr := n.Params
		pr.LogicKind = "probability"
		pr.LogicP = p
		g.graph.SetNodeParams(A.ID, pr)
	}
	if n, ok := g.graph.GetNodeByID(B.ID); ok {
		pr := n.Params
		pr.LogicKind = "skip_every_n"
		pr.LogicN = 1
		g.graph.SetNodeParams(B.ID, pr)
	}
	return A, B
}

// sliceEq compares two bool slices of length n for equality.
func sliceEq(a, b []bool, n int) bool {
	if len(a) < n || len(b) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestProbabilityPreviewMatchesPrediction_RewindAndForward(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	A, _ := buildProbChain(g, 0.37)
	g.start = A
	g.graph.StartNodeID = A.ID
	horizon := 128
	window := 24
	g.drum.SetLength(window)
	g.updateBeatInfos()
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}
	g.engine.Predictor.Ensure(horizon)
	// Low-level graph stats
	row, loop, lstart := g.graph.CalculateBeatRowUnbounded()
	_ = row
	t.Logf("graph loop=%v graphLoopStart=%d", loop, lstart)
	t.Logf("edges: %v", g.graph.Edges)
	loopStart := -1
	if len(g.loopStartByRow) > 0 {
		loopStart = g.loopStartByRow[0]
	}
	t.Logf("dbg: isLoopByRow[0]=%v loopStart=%d lenInfos=%d", len(g.isLoopByRow) > 0 && g.isLoopByRow[0], loopStart, len(g.beatInfosByRow[0]))

	// Quick sanity at off=0
	g.drum.Offset = 0
	g.refreshDrumRow()
	pred0 := make([]bool, window)
	for i := 0; i < window; i++ {
		pred0[i] = g.engine.Predictor.VisibleAt(0, i)
	}
	t.Logf("pred0=%v", pred0)
	t.Logf("steps0=%v", g.drum.Rows[0].Steps)
	// Case 1: rewind windows (Offset before live nextBeatIdx)
	g.nextBeatIdxs = []int{64}
	for _, off := range []int{0, 7, 19, 37} {
		if off+window > horizon {
			break
		}
		g.drum.Offset = off
		g.refreshDrumRow()
		got := append([]bool(nil), g.drum.Rows[0].Steps...)
		want := make([]bool, window)
		for i := 0; i < window; i++ {
			want[i] = g.engine.Predictor.VisibleAt(0, off+i)
		}
		if !sliceEq(got, want, window) {
			t.Fatalf("rewind window off=%d mismatch\nwant=%v\ngot =%v", off, want, got)
		}
	}
	// Case 2: forward windows (Offset after live nextBeatIdx)
	g.nextBeatIdxs = []int{16}
	for _, off := range []int{32, 48, 64, 96} {
		if off+window > horizon {
			break
		}
		g.drum.Offset = off
		g.refreshDrumRow()
		got := append([]bool(nil), g.drum.Rows[0].Steps...)
		want := make([]bool, window)
		for i := 0; i < window; i++ {
			want[i] = g.engine.Predictor.VisibleAt(0, off+i)
		}
		if !sliceEq(got, want, window) {
			t.Fatalf("forward window off=%d mismatch\nwant=%v\ngot =%v", off, want, got)
		}
	}
}

func TestProbabilityAudibleRateRoughlyMatchesP(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	A, _ := buildProbChain(g, 0.31)
	g.start = A
	g.graph.StartNodeID = A.ID
	horizon := 512
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}
	g.engine.Predictor.Ensure(horizon)
	// Sanity: dump loop flags
	isLoop := len(g.isLoopByRow) > 0 && g.isLoopByRow[0]
	loopStart := -1
	if len(g.loopStartByRow) > 0 {
		loopStart = g.loopStartByRow[0]
	}
	t.Logf("isLoop=%v loopStart=%d infosLen=%d", isLoop, loopStart, len(g.beatInfosByRow[0]))
	// Count audible positions that land on A within the horizon
	hits := 0
	occ := 0
	for i := 0; i < horizon; i++ {
		bi := g.beatInfoAtRow(0, i)
		if i < 32 {
			t.Logf("i=%d node=%d type=%d audible=%v", i, bi.NodeID, bi.NodeType, g.engine.Predictor.AudibleAt(0, i))
		}
		if bi.NodeID == A.ID {
			occ++
			if g.engine.Predictor.AudibleAt(0, i) {
				hits++
			}
		}
	}
	if occ == 0 {
		t.Fatalf("no occurrences of A in horizon")
	}
	rate := float64(hits) / float64(occ)
	if math.Abs(rate-0.31) > 0.10 { // allow 10% tolerance over deterministic sequence
		t.Fatalf("audible rate %.3f deviates from p=0.31 with occ=%d hits=%d", rate, occ, hits)
	}
}

// Combine probability with every/skip and verify windows still match prediction.
func TestProbabilityWithEverySkip_WindowsMatchPrediction(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	// Loop: A(p=0.42) -> B(every2) -> C(skip3) -> A
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(1, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, A)
	if n, ok := g.graph.GetNodeByID(A.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.42
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
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(C.ID, p)
	}
	g.start = A
	g.graph.StartNodeID = A.ID
	horizon := 160
	win := 20
	g.drum.SetLength(win)
	g.updateBeatInfos()
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("missing engine predictor")
	}
	g.engine.Predictor.Ensure(horizon)
	// Sweep many windows (both rewind and forward relative to a base),
	// skipping exact seam boundaries to avoid cosmetic seam masks.
	g.nextBeatIdxs = []int{64}
	seg := g.loopLenByRow[0]
	start := g.loopStartByRow[0]
	for off := 0; off+win <= horizon; off += 7 {
		if seg > 0 {
			if off >= start+1 {
				k0 := (off - (start + 1)) % seg
				if k0 < 0 {
					k0 += seg
				}
				// window hits seam if distance to next seam <= win
				if seg-k0 <= win {
					continue
				}
			}
		}
		g.drum.Offset = off
		g.refreshDrumRow()
		got := append([]bool(nil), g.drum.Rows[0].Steps...)
		want := make([]bool, win)
		for i := 0; i < win; i++ {
			want[i] = g.engine.Predictor.VisibleAt(0, off+i)
		}
		if !sliceEq(got, want, win) {
			t.Fatalf("prob+every/skip window off=%d mismatch\nwant=%v\ngot =%v", off, want, got)
		}
	}
}
