package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The windowed buffer bakes cells at the CURRENT vertical scroll, so a pure vertical scroll must rebake it on the
// next draw, or it shows the previous rows' cells until something else forces a rebake.
func TestRowsWindowedCacheRebakesOnVerticalScroll(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 600)

	// More rows than the rack shows.
	for len(g.drum.Rows) < 16 {
		g.drum.AddRow()
	}

	// A looping circuit, so the window scrolls and the windowed path engages.
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.updateBeatInfos()

	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.drum.SetFollow(true)
	defer SetTrackBeatForceRefreshForTest(false)() // production scroll cadence
	pressPlay(t, g.drum)

	scr := ebiten.NewImage(900, 600)

	// Into steady scrolling, buffer baked and following.
	for i := 1; i <= 96; i++ {
		setPlayStartForAbs(g, i)
		_ = g.Update()
		g.Draw(scr)
	}

	// Not engaged means the setup is wrong.
	if !g.drum.rowsWinValid {
		t.Fatalf("windowed rows cache never engaged (rowsWinValid=false) — test cannot exercise the bug")
	}
	if g.drum.Offset <= 0 {
		t.Fatalf("window never scrolled (Offset=%d) — windowed path not in follow-scroll regime", g.drum.Offset)
	}
	if g.drum.visibleRows() >= len(g.drum.Rows) {
		t.Fatalf("no vertical scroll headroom: visibleRows=%d rows=%d", g.drum.visibleRows(), len(g.drum.Rows))
	}

	// rowOffset only, so the new scroll is the only reason to rebake.
	offBefore := g.drum.rowOffset
	genBefore := g.drum.rowsLayerGen
	g.drum.syncRowScroll()
	if g.drum.rowScroll().HandleWheel(-1) {
		g.drum.flushRowScroll()
	}
	if g.drum.rowOffset == offBefore {
		t.Fatalf("vertical scroll did not change rowOffset (still %d)", offBefore)
	}

	g.Draw(scr)

	if g.drum.rowsLayerGen == genBefore {
		t.Fatalf("windowed rows buffer was NOT re-baked after vertical scroll "+
			"(rowsLayerGen stayed %d): the stale buffer (rows baked at rowOffset=%d) "+
			"is still blitted at the new scroll position rowOffset=%d, corrupting the "+
			"visible/future drum cells until an unrelated rebake trigger fires.",
			genBefore, offBefore, g.drum.rowOffset)
	}
}
