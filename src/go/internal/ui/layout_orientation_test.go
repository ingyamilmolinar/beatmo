package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Mobile is always stacked, landscape included.
func TestLandscapeMobileStacked(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400)

	b := g.drum.Bounds
	if b.Min.X != 0 {
		t.Fatalf("expected drum bounds Min.X == 0 for landscape stacked, got %d", b.Min.X)
	}
	if b.Min.Y == 0 {
		t.Fatalf("expected drum bounds Min.Y > 0 for landscape stacked, got %d", b.Min.Y)
	}
}

func TestPortraitMobileStacked(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	b := g.drum.Bounds
	if b.Min.X != 0 {
		t.Fatalf("expected drum bounds Min.X == 0 for portrait stacked, got %d", b.Min.X)
	}
	if b.Min.Y == 0 {
		t.Fatalf("expected drum bounds Min.Y > 0 for portrait stacked, got %d", b.Min.Y)
	}
}

func TestOrientationSwitchPortraitToLandscape(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait first — stacked
	g.Layout(400, 800)
	b := g.drum.Bounds
	if b.Min.X != 0 || b.Min.Y == 0 {
		t.Fatalf("portrait: expected stacked (MinX=0, MinY>0), got MinX=%d MinY=%d", b.Min.X, b.Min.Y)
	}

	// Switch to landscape — still stacked on mobile
	g.Layout(800, 400)
	b = g.drum.Bounds
	if b.Min.X != 0 || b.Min.Y == 0 {
		t.Fatalf("landscape: expected stacked (MinX=0, MinY>0), got MinX=%d MinY=%d", b.Min.X, b.Min.Y)
	}
}

func TestOrientationSwitchLandscapeToPortrait(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Landscape first — stacked (mobile always stacked)
	g.Layout(800, 400)
	b := g.drum.Bounds
	if b.Min.X != 0 || b.Min.Y == 0 {
		t.Fatalf("landscape: expected stacked (MinX=0, MinY>0), got MinX=%d MinY=%d", b.Min.X, b.Min.Y)
	}

	// Switch to portrait — still stacked
	g.Layout(400, 800)
	b = g.drum.Bounds
	if b.Min.X != 0 || b.Min.Y == 0 {
		t.Fatalf("portrait: expected stacked (MinX=0, MinY>0), got MinX=%d MinY=%d", b.Min.X, b.Min.Y)
	}
}

// Both panes keep full width and a reasonable height.
func TestLandscapeMobileBothPanesUsable(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400)

	grid := g.split.GridRect(800, 400)
	drum := g.split.DrumRect(800, 400)

	if grid.Dx() != 800 {
		t.Fatalf("grid pane width=%d, want full width 800", grid.Dx())
	}
	if drum.Dx() != 800 {
		t.Fatalf("drum pane width=%d, want full width 800", drum.Dx())
	}
	if grid.Dy() < 50 {
		t.Fatalf("grid pane height=%d too small in landscape stacked mode", grid.Dy())
	}
	if drum.Dy() < 50 {
		t.Fatalf("drum pane height=%d too small in landscape stacked mode", drum.Dy())
	}
}

// Without the small-screen override, landscape dimensions still stack.
func TestDesktopAlwaysStacked(t *testing.T) {
	setupMobileTest(t, false) // NOT small screen

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 400)

	if !g.split.Horizontal() {
		t.Fatal("desktop should always use stacked (horizontal) layout")
	}
	b := g.drum.Bounds
	if b.Min.X != 0 {
		t.Fatalf("desktop: expected stacked (MinX=0), got MinX=%d", b.Min.X)
	}
	if b.Min.Y == 0 {
		t.Fatalf("desktop: expected stacked (MinY>0), got MinY=%d", b.Min.Y)
	}
}

func TestLandscapeMobileSplitterStacked(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400)

	if !g.split.Horizontal() {
		t.Fatal("landscape mobile should use stacked layout (Horizontal() == true)")
	}
}

func TestLandscapeMobileSplitterDragClampsY(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400)

	// Simulate user dragging splitter to extreme top (y=30)
	g.split.userSet = true
	g.split.Y = 30
	g.split.ratio = float64(30) / float64(400)
	g.split.UpdateResize(400, 800)

	minAllowed := 120
	if g.split.Y < minAllowed {
		t.Fatalf("after drag to top, split.Y=%d below minimum (%d)", g.split.Y, minAllowed)
	}

	// Simulate dragging to extreme bottom (y=380)
	g.split.Y = 380
	g.split.ratio = float64(380) / float64(400)
	g.split.UpdateResize(400, 800)

	maxAllowed := 400 - 120
	if g.split.Y > maxAllowed {
		t.Fatalf("after drag to bottom, split.Y=%d above maximum (%d)", g.split.Y, maxAllowed)
	}
}
