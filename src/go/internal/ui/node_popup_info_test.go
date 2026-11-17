package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSidebarViewportStartsBelowGrownHeader guards the input / viewport content-top
// math against the header band's actual laid-out height. While playing, the header
// grows to a two-line coordinate readout (taller than the legacy fixed
// sidebarHeaderH constant); the scroll viewport must start at the real header
// bottom, not the stale constant — otherwise header taps fall inside the scroll
// viewport and are mis-handled as content presses.
func TestSidebarViewportStartsBelowGrownHeader(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	g.SetPlaying(true)
	advanceFrames(g, 2)
	g.sidebar.Open(a)
	g.sidebar.layout()

	hdr := g.sidebar.rects[sbHeader]
	realTop := hdr.Max.Y + sidebarGap
	gridH := g.split.GridH(g.winH)

	// Discover the content-top that inViewport actually enforces: the smallest y
	// at which a 1px probe rect first counts as scrollable content.
	probeTop := -1
	for y := hdr.Min.Y; y < gridH; y++ {
		if g.sidebar.inViewport(image.Rect(0, y, 1, y+1)) {
			probeTop = y
			break
		}
	}
	if probeTop != realTop {
		t.Fatalf("inViewport content top = %d, want header-derived %d (legacy stale = %d)",
			probeTop, realTop, sidebarPad+sidebarHeaderH+sidebarGap)
	}
}

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

	hdr, ok := g.sidebar.rects[sbHeader]
	if !ok || hdr.Empty() {
		t.Fatal("header rect missing or empty")
	}
	if hdr.Dx() < 20 || hdr.Dy() < 10 {
		t.Fatalf("header rect too small: %v", hdr)
	}
}

func TestNodePopupCoordinateLine(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions: %v", err)
	}

	// 2-node loop so node a belongs to a circuit.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()

	g.sidebar.Open(a)
	g.sidebar.layout()

	// The coords rect exists inside the header band and is non-empty.
	coords, ok := g.sidebar.rects[sbCoords]
	if !ok || coords.Empty() {
		t.Fatal("coords rect missing or empty")
	}
	hdr := g.sidebar.rects[sbHeader]
	if !coords.In(hdr) {
		t.Fatalf("coords rect %v must sit inside header band %v", coords, hdr)
	}

	// The formatted home line carries the grid coords and the circuit beat.
	ta := g.nodeLiveTiming(a.ID)
	line := g.formatNodeCoordHome(a.I, a.J, ta)
	if !strings.Contains(line, "(0, 0)") || !strings.Contains(line, "Beat 1") {
		t.Fatalf("coord line %q missing grid coords or beat", line)
	}
}

// TestNodePopupHeaderDrawPlaying exercises the playing-state header render path
// (the two-line coordinate readout with the live last/next fire) end to end: it
// reserves a second line and drawHeader paints it without panicking.
func TestNodePopupHeaderDrawPlaying(t *testing.T) {
	g, a, _ := buildLoopGame(t)
	g.SetPlaying(true)
	advanceFrames(g, 2)

	g.sidebar.Open(a)
	g.sidebar.layout()

	// Playing reserves a second coord line.
	coords := g.sidebar.rects[sbCoords]
	if coords.Dy() < 2*StyledTextHeight(RoleCaption) {
		t.Fatalf("playing coords rect too short for 2 lines: %v (line=%d)", coords, StyledTextHeight(RoleCaption))
	}

	// Render the sidebar; drawHeader's playing branch must not panic.
	img := ebiten.NewImage(g.winW, g.winH)
	g.sidebar.Draw(img)
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
	hdr, ok := g.sidebar.rects[sbHeader]
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

// TestNodePopupHasNoMoveButton pins the removal of the "Move Node" row (and its
// section header) from the main node popup on desktop: moving a node is already
// covered by copy/paste and by the long-press quick-action popup, so the popup
// no longer duplicates it. The mobile counterpart is
// TestMoveButtonRemovedFromMobilePopup.
func TestNodePopupHasNoMoveButton(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	for _, id := range []string{"move", "sec-move"} {
		if _, ok := g.sidebar.laidOutID(id); ok {
			t.Fatalf("%q should not be laid out in the node popup", id)
		}
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
