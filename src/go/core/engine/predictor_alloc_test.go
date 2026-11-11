package engine

import (
	"io"
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestPredictorEnsureBoundedAllocation asserts that calling Predictor.Ensure
// repeatedly with horizon advancing one step at a time (the production
// scheduler pattern: seqScheduleTime calls Ensure(target+1) every ~4 ms with
// target advancing by one each tick) does NOT allocate quadratically.
//
// Background:
//
//	Ensure grows audibleByRow / visibleByRow / triggeredByRow with
//	  tmp := make([]bool, len(...), horizon)
//	  copy(tmp, oldSlice)
//	When the cap is exactly horizon and horizon then advances by one, the next
//	call hits cap < horizon again and allocates a fresh slice. The cumulative
//	allocation across N steps is sum(1..N) = O(N^2) bytes — for production-scale
//	N (tens of thousands) on WASM (single-threaded GC, 2 GB linear-memory
//	ceiling) this exceeds what the runtime can reclaim during gameplay and
//	manifests as the 2 GB OOM observed at ~10 minutes of playback.
//
// The fix is geometric growth: when cap < horizon, reallocate to at least
// 2× the old cap (or some other amortized policy). After the fix, total
// allocations across N steps must be O(N) — a small constant per step
// for amortized growth, plus a handful of doublings.
//
// The production OOM stack (predictor_compute.go:47, ~14 835-element slice)
// is reproduced by horizon advancing one step at a time across many rows.
func TestPredictorEnsureBoundedAllocation(t *testing.T) {
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
	pred.SetPaths(paths, loops, starts, nodes)

	// Warm up: skip allocations from the first growth burst.
	for h := 1; h <= 100; h++ {
		pred.Ensure(h)
	}

	var pre, post runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&pre)

	// Simulate ~600 s of production playback at 24 abs-index/s (BPM 120,
	// subdiv 12). 14400 single-step horizon advances; matches the production
	// crash horizon of ~14 835.
	const N = 14400
	for h := 101; h <= 100+N; h++ {
		pred.Ensure(h)
	}

	runtime.ReadMemStats(&post)
	allocBytes := post.TotalAlloc - pre.TotalAlloc
	bytesPerStep := float64(allocBytes) / float64(N)

	// With O(N^2) reallocation: cumulative bytes ≈ rows * 3 * N * (N/2 + start)
	// ≈ 6 * 3 * 14400 * 7300 = 1.89 GB. With geometric growth we expect under
	// ~256 bytes/step amortized — give 4x headroom (1 KB/step) for noise from
	// map updates and the per-step prediction work. A regression to O(N^2)
	// pushes bytes/step to ~131 KB at N=14400, four orders of magnitude past
	// the bound.
	const maxBytesPerStep = 1024
	if bytesPerStep > maxBytesPerStep {
		t.Errorf("Predictor.Ensure allocates %.0f bytes/step over %d single-step horizon advances "+
			"(total %d bytes / %.2f MB), bound=%d bytes/step. "+
			"This indicates non-geometric slice growth in Ensure (cap=horizon causes O(N^2) reallocations); "+
			"WASM single-threaded GC cannot reclaim this fast enough and OOMs at ~10 min of production playback.",
			bytesPerStep, N, allocBytes, float64(allocBytes)/(1024*1024), maxBytesPerStep)
	}
}
