package ui

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestPerfFastPath_BypassesThrottleOnGraphEdit(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetInputForTest(
		func() (int, int) { return -9999, -9999 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	assertDefaultPerfFastPath(t, g)
	g.drum.SetFollow(false)
	g.drum.SetLength(16)

	// Simple square loop A->B->C->D->A.
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

	g.updateBeatInfos()
	g.refreshDrumRow()

	// Enter "playing" mode without triggering a transition hook so Update uses
	// the steady-state refresh throttle path.
	g.SetPlaying(true)
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs not initialised")
	}
	g.nextBeatIdxs[0] = 1 // keep window in preview mode (useLive=false)

	g.SetPerfFastPath(true)
	// Clear any one-shot force refresh from enabling fast path so we can test the modulo throttle.
	g.perfMode.ConsumeForceRefresh()

	// Frame 8 should refresh (modulo boundary).
	g.frame = 7
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if g.frame != 8 {
		t.Fatalf("expected frame=8 after update; got %d", g.frame)
	}
	if g.renderFrame != 8 {
		t.Fatalf("expected refresh at frame=8; renderFrame=%d", g.renderFrame)
	}
	before := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Frame 9 should skip refresh when no edits occur.
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if g.frame != 9 {
		t.Fatalf("expected frame=9 after update; got %d", g.frame)
	}
	if g.renderFrame != 8 {
		t.Fatalf("expected refresh skipped at frame=9; renderFrame=%d", g.renderFrame)
	}

	// Graph edit should bypass the modulo throttle immediately.
	nodeB, ok := g.graph.GetNodeByID(b.ID)
	if !ok {
		t.Fatalf("node B missing")
	}
	p := nodeB.Params
	p.LogicKind = "skip_every_n"
	p.LogicN = 1 // deterministic "never fire"
	g.graph.SetNodeParams(b.ID, p)

	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if g.frame != 10 {
		t.Fatalf("expected frame=10 after update; got %d", g.frame)
	}
	if g.renderFrame != 10 {
		t.Fatalf("expected refresh forced at frame=10 after edit; renderFrame=%d", g.renderFrame)
	}
	after := g.drum.Rows[0].Steps
	if slices.Equal(before, after) {
		t.Fatalf("expected row steps to change after edit under fast path; steps=%v", after)
	}
}
