package engine

import (
	"io"
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure advancing one step at a time (the scheduler's pattern) must allocate O(N), not O(N²): with cap == horizon each
// advance reallocated, and on WASM (single-threaded GC, 2 GB ceiling) that garbage rate OOMed long sessions.
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

	// ~600 s of playback at 24 abs/s (120 BPM, subdiv 12), the scale of the OOM horizon.
	const N = 14400
	for h := 101; h <= 100+N; h++ {
		pred.Ensure(h)
	}

	runtime.ReadMemStats(&post)
	allocBytes := post.TotalAlloc - pre.TotalAlloc
	bytesPerStep := float64(allocBytes) / float64(N)

	// Geometric growth stays far under 1 KB/step (4x headroom for map updates); O(N²) is four orders of magnitude past it.
	const maxBytesPerStep = 1024
	if bytesPerStep > maxBytesPerStep {
		t.Errorf("Predictor.Ensure allocates %.0f bytes/step over %d single-step horizon advances "+
			"(total %d bytes / %.2f MB), bound=%d bytes/step. "+
			"This indicates non-geometric slice growth in Ensure (cap=horizon causes O(N^2) reallocations); "+
			"WASM single-threaded GC cannot reclaim this fast enough and OOMs at ~10 min of production playback.",
			bytesPerStep, N, allocBytes, float64(allocBytes)/(1024*1024), maxBytesPerStep)
	}
}
