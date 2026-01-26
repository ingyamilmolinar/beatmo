package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure a 1-subdivision bidirectional pair triggers both nodes at subdiv=8
// and continues to do so after changing to subdiv=16.
func TestBiDirImmediateStepsTriggersBothNodes_AcrossSubdiv(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}
	g.pendingStartRow = 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.pendingStartRow = -1
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	g.drum.SetBPM(200)
	g.SetAppliedBPMForTest(200)
	g.SetPlaying(true)

	// Count audible regular nodes across one beat window.
	countAudible := func(start, span int) (int, int, int) {
		if span <= 0 {
			span = g.grid.MaxDiv()
		}
		if span <= 0 {
			span = 8
		}
		if start < 0 {
			start = 0
		}
		end := start + span
		g.engine.Predictor.Ensure(end + 1)
		countA := 0
		countB := 0
		total := 0
		for abs := start; abs < end; abs++ {
			bi := g.beatInfoAtRow(0, abs)
			if bi.NodeType != model.NodeTypeRegular {
				continue
			}
			if !g.engine.Predictor.AudibleAt(0, abs) {
				t.Fatalf("expected audible regular node at abs=%d (node=%d)", abs, bi.NodeID)
			}
			total++
			if bi.NodeID == a.ID {
				countA++
			} else if bi.NodeID == b.ID {
				countB++
			}
		}
		return total, countA, countB
	}
	start8 := 0
	if len(g.nextBeatIdxs) > 0 {
		start8 = g.nextBeatIdxs[0]
	}
	span8 := g.grid.MaxDiv()
	total8, cA8, cB8 := countAudible(start8, span8)
	if total8 < 4 {
		t.Fatalf("insufficient events at subdiv=8: %d", total8)
	}
	if cA8 == 0 || cB8 == 0 {
		t.Fatalf("expected both nodes to trigger at subdiv=8; counts: A=%d B=%d", cA8, cB8)
	}

	// Stop and increase subdiv to 16; confirm remap and both still trigger.
	stopPlaybackForTest(g)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	if a.I != 0 || b.I != 2 {
		t.Fatalf("node remap mismatch after 8->16: a.I=%d b.I=%d", a.I, b.I)
	}
	g.SetPlaying(true)
	start16 := 0
	if len(g.nextBeatIdxs) > 0 {
		start16 = g.nextBeatIdxs[0]
	}
	span16 := g.grid.MaxDiv()
	total16, cA16, cB16 := countAudible(start16, span16)
	if total16 < 4 {
		t.Fatalf("insufficient events at subdiv=16: %d", total16)
	}
	if cA16 == 0 || cB16 == 0 {
		t.Fatalf("expected both nodes to trigger at subdiv=16; counts: A=%d B=%d", cA16, cB16)
	}
}
