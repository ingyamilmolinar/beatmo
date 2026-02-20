package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestNodePopupShowsCoordinates(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(3, 5, model.NodeTypeRegular)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	hdr, ok := g.sidebar.rects["header"]
	if !ok || hdr.Empty() {
		t.Fatal("header rect missing or empty")
	}
	if hdr.Dx() < 20 || hdr.Dy() < 10 {
		t.Fatalf("header rect too small: %v", hdr)
	}
}

func TestNodePopupShowsInstrument(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a circuit so the node belongs to a row
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()

	// Open popup on node a
	g.sidebar.Open(a)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	// Verify header exists
	hdr, ok := g.sidebar.rects["header"]
	if !ok || hdr.Empty() {
		t.Fatal("header rect missing")
	}

	// Verify node belongs to row 0
	row, ok := g.nodeRows[a.ID]
	if !ok || row != 0 {
		t.Fatalf("expected node in row 0, got row=%d ok=%v", row, ok)
	}

	// Verify instrument name is accessible
	if g.drum.Rows[0].Name == "" {
		t.Fatal("row 0 should have an instrument name")
	}
}

func TestNodePopupMoveButtonExists(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	moveRect, ok := g.sidebar.rects["move"]
	if !ok || moveRect.Empty() {
		t.Fatal("move button rect missing or empty")
	}
	if moveRect.Dx() < 10 || moveRect.Dy() < 10 {
		t.Fatalf("move button too small: %v", moveRect)
	}
}

func TestCoordBadgeSetOnNodeCreate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(3, 5, model.NodeTypeRegular)
	if g.coordBadgeNode != n {
		t.Fatal("coordBadgeNode should be set after tryAddNode")
	}
	if g.coordBadgeFrame != g.frame {
		t.Fatalf("coordBadgeFrame expected %d, got %d", g.frame, g.coordBadgeFrame)
	}
}
