package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure a 1-subdivision bidirectional pair triggers both nodes at subdiv=8
// and continues to do so after changing to subdiv=16.
func TestBiDirImmediateStepsTriggersBothNodes_AcrossSubdiv(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
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

	// Capture scheduled NodeIDs via scheduleHook.
	got8 := []model.NodeID{}
	g.scheduleHook = func(row, idx int) {
		if row != 0 {
			return
		}
		bi := g.beatInfoAtRow(row, idx)
		if bi.NodeType == model.NodeTypeRegular {
			got8 = append(got8, bi.NodeID)
		}
	}
	g.drum.playPressed = true
	end := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) {
		_ = g.Update()
		time.Sleep(3 * time.Millisecond)
	}
	if len(got8) < 4 {
		t.Fatalf("insufficient events at subdiv=8: %d", len(got8))
	}
	cA8, cB8 := 0, 0
	for _, id := range got8 {
		if id == a.ID {
			cA8++
		} else if id == b.ID {
			cB8++
		}
	}
	if cA8 == 0 || cB8 == 0 {
		t.Fatalf("expected both nodes to trigger at subdiv=8; counts: A=%d B=%d", cA8, cB8)
	}

	// Stop and increase subdiv to 16; confirm remap and both still trigger.
	g.drum.stopPressed = true
	_ = g.Update()
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	if a.I != 0 || b.I != 2 {
		t.Fatalf("node remap mismatch after 8->16: a.I=%d b.I=%d", a.I, b.I)
	}
	got16 := []model.NodeID{}
	g.scheduleHook = func(row, idx int) {
		if row != 0 {
			return
		}
		bi := g.beatInfoAtRow(row, idx)
		if bi.NodeType == model.NodeTypeRegular {
			got16 = append(got16, bi.NodeID)
		}
	}
	g.drum.playPressed = true
	end = time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) {
		_ = g.Update()
		time.Sleep(3 * time.Millisecond)
	}
	if len(got16) < 4 {
		t.Fatalf("insufficient events at subdiv=16: %d", len(got16))
	}
	cA16, cB16 := 0, 0
	for _, id := range got16 {
		if id == a.ID {
			cA16++
		} else if id == b.ID {
			cB16++
		}
	}
	if cA16 == 0 || cB16 == 0 {
		t.Fatalf("expected both nodes to trigger at subdiv=16; counts: A=%d B=%d", cA16, cB16)
	}
}
