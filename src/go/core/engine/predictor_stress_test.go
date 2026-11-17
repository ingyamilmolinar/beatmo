package engine

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// buildLongPath builds a long single-row loop path with regular nodes.
func buildLongPath(n int) (*model.Graph, [][]model.BeatInfo, []bool, []int, map[model.NodeID]model.Node) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	prev := g.AddNode(0, 0, model.NodeTypeRegular)
	ids := []model.NodeID{prev}
	for i := 1; i < n; i++ {
		cur := g.AddNode(i, 0, model.NodeTypeRegular)
		g.Edges[[2]model.NodeID{ids[i-1], cur}] = struct{}{}
		ids = append(ids, cur)
	}
	g.Edges[[2]model.NodeID{ids[n-1], ids[0]}] = struct{}{}
	g.StartNodeID = ids[0]
	row, loop, start := g.CalculateBeatRow()
	_ = loop
	nodes := make(map[model.NodeID]model.Node, len(g.Nodes))
	for id, node := range g.Nodes {
		nodes[id] = node
	}
	return g, [][]model.BeatInfo{row}, []bool{true}, []int{start}, nodes
}

func TestEnginePredictor_BackgroundKeepsAhead(t *testing.T) {
	// Build a long loop and start background precompute; ensure horizon grows.
	g, paths, isLoop, loopStart, nodes := buildLongPath(256)
	p := NewPredictor(g, nil)
	p.SetPaths(paths, isLoop, loopStart, nodes)
	targetH := 2048
	p.StartBackground(func() int { return targetH })
	defer p.StopBackground()
	// Allow some time for background to progress.
	time.Sleep(50 * time.Millisecond)
	// After a short time, we should be significantly ahead of 0.
	_, _, h := p.Snapshot()
	if h < 128 {
		t.Fatalf("predictor background too slow, horizon=%d", h)
	}
	// Foreground Ensure up to target should be quick now.
	start := time.Now()
	p.Ensure(targetH)
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("predictor Ensure took too long: %s", time.Since(start))
	}
}

func TestPredictorBeatInfoHandlesNegativeIndex(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)

	a := g.AddNode(0, 0, model.NodeTypeRegular)
	b := g.AddNode(1, 0, model.NodeTypeRegular)
	c := g.AddNode(2, 0, model.NodeTypeRegular)
	g.Edges[[2]model.NodeID{a, b}] = struct{}{}
	g.Edges[[2]model.NodeID{b, c}] = struct{}{}
	g.Edges[[2]model.NodeID{c, b}] = struct{}{}
	g.StartNodeID = a

	path, isLoop, loopStart := g.CalculateBeatRow()
	if !isLoop {
		t.Fatal("expected loop path")
	}
	if loopStart <= 0 {
		t.Fatalf("expected loop start >0, got %d", loopStart)
	}

	nodes := make(map[model.NodeID]model.Node, len(g.Nodes))
	for id, node := range g.Nodes {
		nodes[id] = node
	}
	p := NewPredictor(g, nil)
	p.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("beatInfoAtRow panicked: %v", r)
		}
	}()

	info := p.beatInfoAtRow(0, -1)
	if info.NodeID == model.InvalidNodeID {
		t.Fatalf("unexpected invalid beat info for negative index: %#v", info)
	}

	// Ensure also should handle indices before the loop start without panicking.
	p.Ensure(loopStart)
}
