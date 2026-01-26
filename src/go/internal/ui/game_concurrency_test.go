//go:build test

package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestScheduleSoundConcurrentMapSafety(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)

	nodeID := g.graph.AddNode(0, 0, model.NodeTypeRegular)
	info := model.BeatInfo{
		NodeID:   nodeID,
		NodeType: model.NodeTypeRegular,
		I:        0,
		J:        0,
	}

	g.SetPlaying(true)

	const workers = 8
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(offset float64) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				g.scheduleSound(0, j, info, "hihat", 1, 0, 1, offset, true)
			}
		}(float64(i) * 0.01)
	}
	wg.Wait()
}
