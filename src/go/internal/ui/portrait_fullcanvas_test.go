package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Mobile has no transport bar above the grid, so grid content starts at y=0 with no dead band.
func TestPortraitMobileNoTopGap(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	// Place a node at world origin (grid 0,0).
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}

	// Set camera offset to (0,0) so world origin maps to the screen origin.
	g.cam.OffsetX = 0
	g.cam.OffsetY = 0
	g.cam.Snap()

	// Get the node's screen position.
	_, y1, _, y2 := g.nodeScreenRect(n)
	centerY := (y1 + y2) / 2

	if centerY > 5 {
		t.Fatalf("node at world (0,0) with cam (0,0) has screen center y=%.1f; "+
			"expected y near 0 (no dead zone), but content is shifted down — "+
			"this creates a black/empty gap at the top of the canvas",
			centerY)
	}
}

// No dead zone where taps are silently rejected.
func TestPortraitMobileInputAcceptsTopOfGrid(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	// Place a node and position camera so the node appears near y=5.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}

	g.cam.OffsetX = 200
	g.cam.OffsetY = 5
	g.cam.Snap()

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	centerX := int((x1 + x2) / 2)
	centerY := (y1 + y2) / 2

	if centerY > 20 {
		t.Fatalf("expected node near top of grid (y<20), got y=%.1f — "+
			"gridTopOffset() is likely shifting content down", centerY)
	}

	// A tap where the user sees the node.
	found := g.nodeAtScreen(centerX, int(centerY))
	if found == nil {
		t.Fatalf("nodeAtScreen(%d, %.0f) returned nil — tap rejected at top "+
			"of grid pane; mobile input should accept the full grid area",
			centerX, centerY)
	}
}

// The usable height is the whole grid pane, not splitY - gridTopOffset().
func TestPortraitMobileContentFillsFullGridHeight(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	splitY := g.split.Y

	// Centred within the FULL pane height: the centre Y is splitY/2.
	g.cam.OffsetX = float64(g.split.GridW(g.winW)) / 2
	g.cam.OffsetY = float64(splitY) / 2
	g.cam.Snap()

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}

	_, y1, _, y2 := g.nodeScreenRect(n)
	centerY := (y1 + y2) / 2

	expectedCenter := float64(splitY) / 2
	tolerance := 5.0

	if centerY < expectedCenter-tolerance || centerY > expectedCenter+tolerance {
		t.Fatalf("node center y=%.1f, expected near %.1f (splitY/2=%d); "+
			"gridTopOffset()=%d likely creating a gap — content should fill "+
			"the full grid pane from y=0 to y=%d",
			centerY, expectedCenter, splitY/2, gridTopOffset(), splitY)
	}
}

func TestPortraitMobileDrumPaneBottomEdge(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	winH := 800
	g.Layout(400, winH)

	drumRect := g.split.DrumRect(g.winW, g.winH)

	// Drum pane must extend exactly to the bottom of the canvas.
	if drumRect.Max.Y != winH {
		t.Fatalf("drum pane bottom edge at y=%d, expected y=%d (winH) — "+
			"the UI does not cover the full canvas", drumRect.Max.Y, winH)
	}

	// Verify actual drum view bounds match.
	if g.drum.Bounds.Max.Y != winH {
		t.Fatalf("drum view bounds bottom at y=%d, expected y=%d",
			g.drum.Bounds.Max.Y, winH)
	}
}

func TestPortraitMobileGridAndDrumTileCanvas(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	winW, winH := 400, 800
	g.Layout(winW, winH)

	gridRect := g.split.GridRect(g.winW, g.winH)
	drumRect := g.split.DrumRect(g.winW, g.winH)

	// Grid pane must start at the very top of the canvas.
	if gridRect.Min.Y != 0 {
		t.Fatalf("grid pane top at y=%d, expected y=0", gridRect.Min.Y)
	}
	if gridRect.Min.X != 0 {
		t.Fatalf("grid pane left at x=%d, expected x=0", gridRect.Min.X)
	}

	// Grid pane bottom must equal drum pane top (no gap between them).
	if gridRect.Max.Y != drumRect.Min.Y {
		t.Fatalf("gap between panes: grid bottom y=%d != drum top y=%d",
			gridRect.Max.Y, drumRect.Min.Y)
	}

	// Drum pane must reach the bottom of the canvas.
	if drumRect.Max.Y != winH {
		t.Fatalf("drum pane bottom y=%d != canvas bottom y=%d",
			drumRect.Max.Y, winH)
	}

	// Both panes must span the full width.
	if gridRect.Max.X != winW {
		t.Fatalf("grid pane width %d != canvas width %d", gridRect.Max.X, winW)
	}
	if drumRect.Max.X != winW {
		t.Fatalf("drum pane width %d != canvas width %d", drumRect.Max.X, winW)
	}

	// gridTopOffset() must not shrink the usable area on mobile.
	gridH := gridRect.Dy()
	effectiveGridH := gridH - gridTopOffset()
	if effectiveGridH < gridH {
		t.Fatalf("grid pane allocated %dpx but only %dpx is usable (gridTopOffset()=%d "+
			"creates a %dpx dead zone at top); on mobile the full height should "+
			"be usable", gridH, effectiveGridH, gridTopOffset(), gridTopOffset())
	}
}

func TestPortraitMobileCameraUsesFullGridHeight(t *testing.T) {
	setupMobileTest(t, true)
	// Enable default start so Layout sets the camera center.
	SetDefaultStartForTest(true)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	splitY := g.split.Y

	expectedCameraY := float64(splitY) / 2.0
	actualCameraY := g.cam.OffsetY

	// Allow some tolerance for rounding/snapping.
	tolerance := 5.0
	if actualCameraY < expectedCameraY-tolerance || actualCameraY > expectedCameraY+tolerance {
		t.Fatalf("camera OffsetY=%.1f, expected near %.1f (splitY/2); "+
			"camera is centered in a reduced area (splitY-gridTopOffset())/2=%.1f, "+
			"which creates a visual offset on mobile",
			actualCameraY, expectedCameraY, float64(splitY-gridTopOffset())/2)
	}
}
