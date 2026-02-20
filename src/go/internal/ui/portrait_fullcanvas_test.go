package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestPortraitMobileNoTopGap verifies that in portrait mobile mode, grid
// content starts at y=0 — there must be no dead zone (black section) at the
// top of the canvas. Historically, gridTopOffset() pushed all grid content down
// by 40px, wasting that space on mobile where no transport bar exists.
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

	// On mobile portrait, the node at world (0,0) with camera at (0,0)
	// should appear near the top of the grid pane (y close to 0).
	// The UI must cover the full canvas — no 40px dead zone at the top.
	// gridTopOffset() returns 0 on mobile, so center should be at ~0.
	if centerY > 5 {
		t.Fatalf("node at world (0,0) with cam (0,0) has screen center y=%.1f; "+
			"expected y near 0 (no dead zone), but content is shifted down — "+
			"this creates a black/empty gap at the top of the canvas",
			centerY)
	}
}

// TestPortraitMobileInputAcceptsTopOfGrid verifies that touch/click input
// is accepted at the very top of the grid pane (y near 0). On mobile,
// there should be no dead zone where taps are silently rejected.
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

	// Position camera so node appears at a small y value (near top of screen).
	// On mobile, y=5 should be a valid interactive coordinate.
	g.cam.OffsetX = 200
	g.cam.OffsetY = 5
	g.cam.Snap()

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	centerX := int((x1 + x2) / 2)
	centerY := (y1 + y2) / 2

	// The node should be near the top of the grid pane.
	// On mobile, a node at world (0,0) with camera OffsetY=5 should appear
	// at screen y ≈ 5. With gridTopOffset(), it appears at y ≈ 45 instead.
	if centerY > 20 {
		t.Fatalf("expected node near top of grid (y<20), got y=%.1f — "+
			"gridTopOffset() is likely shifting content down", centerY)
	}

	// Attempt to find the node via nodeAtScreen at its visual center.
	// This simulates a user tap at where they see the node.
	found := g.nodeAtScreen(centerX, int(centerY))
	// On mobile, taps at y < gridTopOffset() should NOT be rejected.
	// Input must be accepted anywhere in the grid pane including near y=0.
	if found == nil {
		t.Fatalf("nodeAtScreen(%d, %.0f) returned nil — tap rejected at top "+
			"of grid pane; mobile input should accept the full grid area",
			centerX, centerY)
	}
}

// TestPortraitMobileContentFillsFullGridHeight verifies that the grid pane's
// content area equals the full grid pane rect. On mobile, the usable content
// height should be splitY (the entire grid pane), not splitY - gridTopOffset().
func TestPortraitMobileContentFillsFullGridHeight(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	splitY := g.split.Y

	// The grid pane occupies [0, 0, 400, splitY].
	// On mobile, the camera should center content within the FULL height
	// (0 to splitY), not within a reduced height (gridTopOffset() to splitY).
	//
	// With camera centered in the grid pane, the camera center Y should be
	// at splitY/2, not at (splitY + gridTopOffset())/2 or (splitY - gridTopOffset())/2.
	g.cam.OffsetX = float64(g.split.GridW(g.winW)) / 2
	g.cam.OffsetY = float64(splitY) / 2
	g.cam.Snap()

	// Place a node at world (0,0); it should appear at the center of the
	// full grid pane (near y = splitY/2).
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}

	_, y1, _, y2 := g.nodeScreenRect(n)
	centerY := (y1 + y2) / 2

	// The node should be centered at approximately splitY/2.
	// If gridTopOffset() is applied, the center will be at splitY/2 + gridTopOffset()
	// instead, which wastes the top portion of the grid pane.
	expectedCenter := float64(splitY) / 2
	tolerance := 5.0

	if centerY < expectedCenter-tolerance || centerY > expectedCenter+tolerance {
		t.Fatalf("node center y=%.1f, expected near %.1f (splitY/2=%d); "+
			"gridTopOffset()=%d likely creating a gap — content should fill "+
			"the full grid pane from y=0 to y=%d",
			centerY, expectedCenter, splitY/2, gridTopOffset(), splitY)
	}
}

// TestPortraitMobileDrumPaneBottomEdge verifies that the drum pane extends
// exactly to the bottom of the canvas (y = winH) with no gap or overflow.
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

// TestPortraitMobileGridAndDrumTileCanvas verifies that the grid pane and
// drum pane together cover the ENTIRE canvas with no gap between them,
// no gap at the top, and no gap at the bottom.
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

	// CRITICAL: The effective content height of the grid pane must equal its
	// allocated height. On mobile, gridTopOffset() should NOT reduce the usable
	// grid area. The camera should place world origin at y=0 of the grid
	// pane, not at y=gridTopOffset().
	gridH := gridRect.Dy()
	effectiveGridH := gridH - gridTopOffset()
	if effectiveGridH < gridH {
		t.Fatalf("grid pane allocated %dpx but only %dpx is usable (gridTopOffset()=%d "+
			"creates a %dpx dead zone at top); on mobile the full height should "+
			"be usable", gridH, effectiveGridH, gridTopOffset(), gridTopOffset())
	}
}

// TestPortraitMobileCameraUsesFullGridHeight verifies that the camera offset
// calculation for portrait mobile centers content within the full grid pane
// height (0 to splitY), not within a reduced area that accounts for gridTopOffset().
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

	// The camera center Y should place the world origin near the center
	// of the full grid pane: approximately splitY / 2.
	// If gridTopOffset() is subtracted: (splitY - gridTopOffset()) / 2 = (400 - 40) / 2 = 180
	// Correct (no gridTopOffset()): splitY / 2 = 400 / 2 = 200
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
