package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestAdaptiveCellCountNarrowScreen verifies that on a narrow screen, the drum
// view clamps Length so cells are at least MinCellWidth() pixels wide.
func TestAdaptiveCellCountNarrowScreen(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Build a circuit with many nodes → long traversal (128 nodes in a loop).
	first := g.tryAddNode(0, 0, model.NodeTypeRegular)
	prev := first
	for i := 1; i < 128; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.addEdge(prev, first) // close the loop
	g.start = first
	g.graph.StartNodeID = first.ID
	g.drum.Rows[0].Origin = first.ID
	g.drum.Rows[0].Node = first

	// Narrow layout (mobile-ish width).
	g.Layout(400, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.calcLayout() // recompute dv.cell after Length change

	// The circuit has 128 nodes, but the visible Length should be clamped.
	tlW := g.drum.timelineRect.Dx()
	mcw := MinCellWidth()
	if tlW <= 0 {
		t.Fatalf("timeline width is %d, expected positive", tlW)
	}
	expectedMax := tlW / mcw
	if g.drum.Length > expectedMax {
		t.Fatalf("Length %d exceeds screen-aware max %d (tlW=%d, mcw=%d)",
			g.drum.Length, expectedMax, tlW, mcw)
	}

	// Verify cell width is at least MinCellWidth.
	if g.drum.cell < mcw {
		t.Fatalf("cell width %d < MinCellWidth %d", g.drum.cell, mcw)
	}

	// Verify the full circuit is still scrollable via timelineBeats.
	if g.drum.timelineBeats < 128 {
		t.Errorf("timelineBeats %d should cover full 128-node circuit", g.drum.timelineBeats)
	}
}

// TestAdaptiveCellCountWideScreen verifies that on a wide screen, Length is
// allowed to be larger (more cells visible) while still respecting the minimum.
func TestAdaptiveCellCountWideScreen(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Build a 64-node loop.
	first := g.tryAddNode(0, 0, model.NodeTypeRegular)
	prev := first
	for i := 1; i < 64; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.addEdge(prev, first)
	g.start = first
	g.graph.StartNodeID = first.ID
	g.drum.Rows[0].Origin = first.ID
	g.drum.Rows[0].Node = first

	// Wide layout (desktop).
	g.Layout(1200, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.calcLayout() // recompute dv.cell after Length change

	mcw := MinCellWidth()
	if g.drum.cell < mcw {
		t.Fatalf("cell width %d < MinCellWidth %d on wide screen", g.drum.cell, mcw)
	}

	// On a wide screen, 64 cells should easily fit.
	if g.drum.Length < 64 {
		t.Errorf("expected Length >= 64 on wide screen, got %d", g.drum.Length)
	}
}

// TestAdaptiveCellCountResizeReclamp verifies that resizing the window
// re-clamps Length so cells stay readable.
func TestAdaptiveCellCountResizeReclamp(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)

	dv := NewDrumView(image.Rect(0, 0, 1200, 400), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	// Set a large length (should fit on the wide screen).
	dv.SetLength(200)
	wideLen := dv.Length

	// Shrink the window — Length should decrease.
	dv.SetBounds(image.Rect(0, 0, 300, 400))
	narrowLen := dv.Length

	mcw := MinCellWidth()
	tlW := dv.timelineRect.Dx()
	maxNarrow := tlW / mcw
	if maxNarrow < 1 {
		maxNarrow = 1
	}

	if narrowLen > maxNarrow {
		t.Fatalf("after resize narrow, Length %d > maxVisible %d (tlW=%d, mcw=%d)",
			narrowLen, maxNarrow, tlW, mcw)
	}
	if narrowLen >= wideLen && wideLen > maxNarrow {
		t.Fatalf("expected Length to decrease after shrink: wide=%d narrow=%d max=%d",
			wideLen, narrowLen, maxNarrow)
	}
}

// TestAdaptiveCellCountSmallScreen verifies that the touch minimum cell width
// is used on small screens.
func TestAdaptiveCellCountSmallScreen(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 300), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	mcw := MinCellWidth()
	if mcw != touchMinCellWidthPx {
		t.Fatalf("expected MinCellWidth=%d on small screen, got %d", touchMinCellWidthPx, mcw)
	}

	dv.SetLengthClamped(200) // should be clamped by screen-aware limit
	tlW := dv.timelineRect.Dx()
	maxLen := tlW / mcw
	if dv.Length > maxLen {
		t.Fatalf("small screen: Length %d > max %d (tlW=%d, mcw=%d)",
			dv.Length, maxLen, tlW, mcw)
	}
}
