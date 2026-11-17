package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Live edits (add/remove nodes) must not alter the past part of the visible
// window in the DrumView. Only the future portion updates, and other rows stay
// unchanged. Sprite caches must rebuild only for affected rows.
func TestLiveEdit_AddRemove_NoPastChange(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)
	g.drum.SetFollow(false) // fixed window

	// Build a 1x1 rectangle for row 0: a(0,0)->b(1,0)->c(1,1)->d(0,1)->a
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

	// Add a second loop (row 1) to ensure no cross-row cache pollution.
	e := g.tryAddNode(4, 0, model.NodeTypeRegular)
	f := g.tryAddNode(5, 0, model.NodeTypeRegular)
	h := g.tryAddNode(5, 1, model.NodeTypeRegular)
	i := g.tryAddNode(4, 1, model.NodeTypeRegular)
	g.addEdge(e, f)
	g.addEdge(f, h)
	g.addEdge(h, i)
	g.addEdge(i, e)
	g.drum.AddRow()
	g.drum.Rows[1].Origin = e.ID
	g.drum.Rows[1].Node = g.nodeByID(e.ID)

	g.drum.SetLength(24)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Build initial caches
	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if len(g.drum.rowCache) < 2 || g.drum.rowCache[0] == nil || g.drum.rowCache[1] == nil {
		t.Fatalf("expected two row caches built")
	}
	// Keep a copy of row 1 steps (snapshot just before the edit) to assert it
	// remains unchanged by row 0 edits

	// Start playback and freeze some past
	g.drum.SetBPM(120)
	pressPlay(t, g.drum)
	_ = g.Update()
	advancePlaybackByAbs(g, g.grid.MaxDiv()*3)
	pastAbs := 0
	if len(g.nextBeatIdxs) > 0 {
		pastAbs = g.nextBeatIdxs[0]
	}
	// snapshot visible row0 steps
	before := append([]bool(nil), g.drum.Rows[0].Steps...)
	// snapshot row1 steps at the same moment
	row1Before := append([]bool(nil), g.drum.Rows[1].Steps...)

	// Live edit in the FUTURE: reroute b->c via (2,0)->(2,1) then c
	nx1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	nx2 := g.tryAddNode(2, 1, model.NodeTypeRegular)
	// Delete old direct edge and create a detour
	g.deleteEdge(b, c)
	g.addEdge(b, nx1)
	g.addEdge(nx1, nx2)
	g.addEdge(nx2, c)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Draw again to rebuild caches
	g.drum.Draw(dst, nil, 0, nil, 0)
	after := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Past cells in window must remain identical
	for j := 0; j < len(after) && j < len(before); j++ {
		abs := g.drum.Offset + j
		if abs < pastAbs && after[j] != before[j] {
			t.Fatalf("past changed at abs=%d: before=%v after=%v", abs, before[j], after[j])
		}
	}
	// Row 1 (unaffected circuit) must not be marked changed
	if len(g.rowsPathChanged) > 1 && g.rowsPathChanged[1] {
		t.Fatalf("row 1 erroneously detected as changed")
	}
	// Row 1 (unaffected circuit) past window must remain identical
	row1Past := 0
	if len(g.nextBeatIdxs) > 1 {
		row1Past = g.nextBeatIdxs[1]
	}
	for j := 0; j < len(g.drum.Rows[1].Steps) && j < len(row1Before); j++ {
		abs := g.drum.Offset + j
		if abs < row1Past && g.drum.Rows[1].Steps[j] != row1Before[j] {
			t.Fatalf("row 1 past changed on row 0 edit at abs=%d", abs)
		}
	}

	// Now remove the detour and restore b->c; past must still be frozen
	g.deleteEdge(b, nx1)
	g.deleteEdge(nx1, nx2)
	g.deleteEdge(nx2, c)
	g.addEdge(b, c)
	g.deleteNode(nx1)
	g.deleteNode(nx2)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, nil, 0, nil, 0)
	after2 := append([]bool(nil), g.drum.Rows[0].Steps...)
	for j := 0; j < len(after2) && j < len(before); j++ {
		abs := g.drum.Offset + j
		if abs < pastAbs && after2[j] != before[j] {
			t.Fatalf("past changed after restore at abs=%d: before=%v after2=%v", abs, before[j], after2[j])
		}
	}
}
