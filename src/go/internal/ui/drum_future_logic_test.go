package ui

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func advancePlaybackLogic(g *Game, dur time.Duration) {
	if g == nil {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.drum.BPM()
	}
	if bpm <= 0 {
		bpm = 120
	}
	steps := int(math.Ceil(dur.Seconds() * float64(bpm) / 60.0 * float64(div)))
	if steps < 1 {
		steps = 1
	}
	advancePlaybackByAbs(g, steps)
}

func TestDrumView_FutureLogicChangeResetsLiveCounts(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Simple 1D loop so future beats are predictable.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)

	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(48)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(640, 240)
	g.drum.Draw(dst, nil, 0, nil, 0)

	// Simulate live playback conditions so refreshDrumRow uses buildPreviewFromLive.
	g.SetPlaying(true)
	g.nextBeatIdxs = []int{g.drum.Offset}
	before := append([]bool(nil), g.drum.Rows[0].Steps...)
	futureAbs := -1
	for abs := g.drum.Offset + 1; abs < g.drum.Offset+g.drum.Length; abs++ {
		if g.beatInfoAtRow(0, abs).NodeID == b.ID {
			futureAbs = abs
			break
		}
	}
	if futureAbs < 0 {
		t.Fatalf("did not find future beat for node b within window (offset=%d len=%d)", g.drum.Offset, g.drum.Length)
	}
	targetRel := futureAbs - g.drum.Offset
	if targetRel < 0 || targetRel >= len(before) {
		t.Fatalf("future index %d out of range len=%d offset=%d", targetRel, len(before), g.drum.Offset)
	}
	if !before[targetRel] {
		t.Fatalf("expected future cell to be active before logic change at rel=%d", targetRel)
	}

	// Seed live trigger counts to emulate prior playback state.
	if g.nodeTriggerCountsByRow[0] == nil {
		g.nodeTriggerCountsByRow[0] = make(map[model.NodeID]int)
	}
	g.nodeTriggerCountsByRow[0][b.ID] = 1

	// Change logic so node b should now fire only every other trigger.
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}

	_ = g.Update()
	after := append([]bool(nil), g.drum.Rows[0].Steps...)

	if targetRel >= len(after) {
		t.Fatalf("future index %d out of range after logic change len=%d", targetRel, len(after))
	}
	if after[targetRel] {
		t.Fatalf("future cell remained active after enabling every_n_triggers (rel=%d offset=%d abs=%d)", targetRel, g.drum.Offset, futureAbs)
	}
}
