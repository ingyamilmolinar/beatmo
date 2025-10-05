package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
	"time"
)

// The past must never change: once playback has advanced past a subdivision,
// edits to the circuit must not alter previously-rendered cells. Only future
// cells may change. This also implies we can cache the past.
func TestDrumView_PastNeverChanges(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	t.Cleanup(func() { close(g.seqQuit) })
	g.Layout(800, 600)
	g.drum.follow = false // keep window fixed

	// Simple 4-node loop for row 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Length = 16
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Build caches and start playback.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.drum.SetBPM(120)
	// Allow BPM apply
	until := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	g.drum.playPressed = true

	// Run for ~150ms to accumulate some past.
	run := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(run) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	pastIdx := 0
	if len(g.nextBeatIdxs) > 0 {
		pastIdx = g.nextBeatIdxs[0]
	}
	snap := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Edit: silence node b (affects future predictions/visibility).
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		n.Type = model.NodeTypeSilent
		g.graph.Nodes[b.ID] = n
		g.notifyPredictorNode(b.ID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	after := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Past portion of the window must remain identical.
	for i := 0; i < len(after) && i < len(snap); i++ {
		abs := g.drum.Offset + i
		if abs < pastIdx {
			if after[i] != snap[i] {
				t.Fatalf("past changed at abs=%d: before=%v after=%v", abs, snap[i], after[i])
			}
		}
	}
}

func TestDrumView_PastCellTypesStayFrozenOnMuteToggle(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	t.Cleanup(func() { close(g.seqQuit) })
	g.Layout(800, 600)
	g.drum.follow = false

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Length = 16
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.drum.SetBPM(120)
	until := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	g.drum.playPressed = true

	run := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(run) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	pastIdx := 0
	if len(g.nextBeatIdxs) > 0 {
		pastIdx = g.nextBeatIdxs[0]
	}
	beforeTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)

	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		n.Type = model.NodeTypeMute
		g.graph.Nodes[b.ID] = n
		g.notifyPredictorNode(b.ID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	afterTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)

	for i := 0; i < len(afterTypes) && i < len(beforeTypes); i++ {
		abs := g.drum.Offset + i
		if abs < pastIdx {
			if afterTypes[i] != beforeTypes[i] {
				t.Fatalf("past cell type changed at abs=%d: before=%v after=%v", abs, beforeTypes[i], afterTypes[i])
			}
		}
	}
}
