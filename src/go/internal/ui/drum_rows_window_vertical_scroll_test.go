package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestRowsWindowedCacheRebakesOnVerticalScroll pins the fix for the
// "scrolling up/down corrupts future drum cells during playback" bug.
//
// During scrolled follow playback the rows composite is served from the
// WIDE windowed buffer (drumview_cache_rows_window.go). That buffer bakes
// each visible row's cells at y = rowBase + (i-rowOffset)*rowHeight, so its
// pixels are tied to the CURRENT vertical scroll position (dv.rowOffset).
//
// Pre-fix the rebake predicate keyed only on the HORIZONTAL offset, buffer
// size, baseX, length and a content-dirty flag — never on dv.rowOffset. A
// vertical scroll therefore left the stale buffer in place and kept blitting
// it (showing the previous rows' cells at the new positions) until some other
// trigger — a content change or the playhead drifting past the pad — forced a
// rebake. That is exactly the user-visible "messed up for a while, then snaps
// back to consistent" corruption.
//
// A rebake re-renders the buffer against the current dv.rowOffset and bumps
// rowsLayerGen (drumview_cache_rows_window.go). This test drives steady-state
// scrolled follow playback until the windowed path is engaged, then performs a
// pure vertical scroll (no playhead/content change) and asserts the next draw
// rebakes the buffer so the freshly-scrolled-in rows are rendered.
func TestRowsWindowedCacheRebakesOnVerticalScroll(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 600)

	// Enough drum rows that the rack cannot show them all at once, so a
	// vertical scroll has headroom.
	for len(g.drum.Rows) < 16 {
		g.drum.AddRow()
	}

	// Minimal looping circuit so the playhead advances and the window scrolls
	// (Offset > 0), which is what engages the windowed composite path.
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.updateBeatInfos()

	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.SetFollow(true)
	defer SetTrackBeatForceRefreshForTest(false)() // production scroll cadence
	pressPlay(t, g.drum)

	scr := ebiten.NewImage(900, 600)

	// Advance into the steady-state scrolling regime so the windowed buffer is
	// baked and the playhead is following.
	for i := 1; i <= 96; i++ {
		setPlayStartForAbs(g, i)
		_ = g.Update()
		g.Draw(scr)
	}

	// Preconditions: the windowed path must actually be engaged for this test
	// to exercise the bug. If it is not, the setup is wrong, not the code.
	if !g.drum.rowsWinValid {
		t.Fatalf("windowed rows cache never engaged (rowsWinValid=false) — test cannot exercise the bug")
	}
	if g.drum.Offset <= 0 {
		t.Fatalf("window never scrolled (Offset=%d) — windowed path not in follow-scroll regime", g.drum.Offset)
	}
	if g.drum.visibleRows() >= len(g.drum.Rows) {
		t.Fatalf("no vertical scroll headroom: visibleRows=%d rows=%d", g.drum.visibleRows(), len(g.drum.Rows))
	}

	// Perform a PURE vertical scroll: change rowOffset only. Do NOT advance the
	// playhead or mutate content, so the ONLY reason for a rebake on the next
	// draw is the new vertical scroll position.
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
