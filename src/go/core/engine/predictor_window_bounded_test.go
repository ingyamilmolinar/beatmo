package engine

import (
	"io"
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// buildLoopedPredictor returns a 6-row predictor whose paths each loop the
// same n0→n1 cycle. Used by the windowing tests below.
func buildLoopedPredictor(t *testing.T, windowCap int) *Predictor {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	n1 := graph.AddNode(1, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{n0, n1}] = struct{}{}
	graph.Edges[[2]model.NodeID{n1, n0}] = struct{}{}
	graph.StartNodeID = n0

	path := []model.BeatInfo{
		{NodeID: n0, NodeType: model.NodeTypeRegular},
		{NodeID: n1, NodeType: model.NodeTypeRegular},
	}
	nodes := map[model.NodeID]model.Node{
		n0: graph.Nodes[n0],
		n1: graph.Nodes[n1],
	}

	const rows = 6
	paths := make([][]model.BeatInfo, rows)
	loops := make([]bool, rows)
	starts := make([]int, rows)
	for i := 0; i < rows; i++ {
		paths[i] = path
		loops[i] = true
	}

	pred := NewPredictor(graph, nil)
	pred.SetWindowCap(windowCap)
	pred.SetPaths(paths, loops, starts, nodes)
	return pred
}

// TestPredictorWindowSizeBounded drives Ensure(h) for h∈[1,1_000_000] and
// asserts the per-row buffer never exceeds windowCap and windowStart slides
// forward correctly once the cap is reached.
func TestPredictorWindowSizeBounded(t *testing.T) {
	const windowCap = 4096
	pred := buildLoopedPredictor(t, windowCap)

	for h := 1; h <= 1_000_000; h++ {
		pred.Ensure(h)
	}

	start, end, capCfg := pred.WindowBoundsForTest()
	if capCfg != windowCap {
		t.Fatalf("WindowCap=%d, want %d", capCfg, windowCap)
	}
	if end != 1_000_000 {
		t.Fatalf("windowEnd=%d, want 1_000_000", end)
	}
	if start != end-windowCap {
		t.Fatalf("windowStart=%d, want %d (end-windowCap)", start, end-windowCap)
	}
	caps := pred.BufferCapsForTest()
	for r, row := range caps {
		for b, lc := range row {
			if lc[1] > windowCap {
				t.Fatalf("row=%d buf=%d len=%d cap=%d exceeds windowCap=%d",
					r, b, lc[1], lc[0], windowCap)
			}
		}
	}
}

// TestPredictorEvictionDropsAncientReads asserts that abs values older than
// windowStart return false from VisibleAt/AudibleAt while values inside the
// window resolve correctly.
func TestPredictorEvictionDropsAncientReads(t *testing.T) {
	const windowCap = 4096
	pred := buildLoopedPredictor(t, windowCap)

	const target = 100_000
	pred.Ensure(target)

	start, end, _ := pred.WindowBoundsForTest()
	if end != target {
		t.Fatalf("windowEnd=%d want %d", end, target)
	}
	if start != target-windowCap {
		t.Fatalf("windowStart=%d want %d", start, target-windowCap)
	}

	// Reads below windowStart are evicted history.
	if pred.VisibleAt(0, 10) {
		t.Errorf("VisibleAt(0,10) should be false (evicted, abs<windowStart=%d)", start)
	}
	if pred.AudibleAt(0, 0) {
		t.Errorf("AudibleAt(0,0) should be false (evicted)")
	}

	// Reads inside the window must still resolve. Both n0 and n1 are regular
	// nodes in a 2-step loop; AudibleAt should be true at both even/odd abs.
	if !pred.AudibleAt(0, target-1) {
		t.Errorf("AudibleAt(0,%d) should be true (inside window, regular node)", target-1)
	}
	if !pred.VisibleAt(0, start) {
		t.Errorf("VisibleAt(0,%d) should be true (at windowStart, regular node)", start)
	}
}

// TestPredictorAllocationStaysBoundedToWindowCap mirrors
// TestPredictorEnsureBoundedAllocation but runs 100_000 single-step horizon
// advances — well past the windowCap=4096 plateau. Once the cap is reached,
// future advances slide in place and allocate nothing; bytesPerStep should
// drop to a small constant.
func TestPredictorAllocationStaysBoundedToWindowCap(t *testing.T) {
	const windowCap = 4096
	pred := buildLoopedPredictor(t, windowCap)

	// Warm up across the full grow-then-plateau ramp so the geometric doubling
	// allocations are not counted in the steady-state measurement.
	for h := 1; h <= windowCap*2; h++ {
		pred.Ensure(h)
	}

	var pre, post runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&pre)

	const N = 100_000
	for h := windowCap*2 + 1; h <= windowCap*2+N; h++ {
		pred.Ensure(h)
	}

	runtime.ReadMemStats(&post)
	allocBytes := post.TotalAlloc - pre.TotalAlloc
	bytesPerStep := float64(allocBytes) / float64(N)

	// Post-cap, slide is in-place copy; per-step work allocates only via
	// per-eval map updates and small temporaries. Bound at 1 KB/step gives
	// generous headroom for noise; a regression would jump several orders
	// of magnitude.
	const maxBytesPerStep = 1024
	if bytesPerStep > maxBytesPerStep {
		t.Errorf("Predictor.Ensure allocates %.0f bytes/step over %d post-cap advances "+
			"(total %d bytes / %.2f MB), bound=%d bytes/step. "+
			"This indicates the sliding window is reallocating instead of sliding in place.",
			bytesPerStep, N, allocBytes, float64(allocBytes)/(1024*1024), maxBytesPerStep)
	}
}
