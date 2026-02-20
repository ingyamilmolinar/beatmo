package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestLayoutPaneCoveragePortrait checks that GridRect + DrumRect cover the full
// canvas in portrait mode with no gap or overlap.
func TestLayoutPaneCoveragePortrait(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	w, h := 390, 844
	g.Layout(w, h)

	grid := g.split.GridRect(w, h)
	drum := g.split.DrumRect(w, h)

	// Grid should span full width
	if grid.Min.X != 0 || grid.Max.X != w {
		t.Fatalf("portrait GridRect width: want [0,%d], got [%d,%d]", w, grid.Min.X, grid.Max.X)
	}
	// Drum should span full width
	if drum.Min.X != 0 || drum.Max.X != w {
		t.Fatalf("portrait DrumRect width: want [0,%d], got [%d,%d]", w, drum.Min.X, drum.Max.X)
	}
	// No gap between panes
	if grid.Max.Y != drum.Min.Y {
		t.Fatalf("portrait pane gap: GridRect.Max.Y=%d != DrumRect.Min.Y=%d", grid.Max.Y, drum.Min.Y)
	}
	// Together they cover full height
	if grid.Min.Y != 0 || drum.Max.Y != h {
		t.Fatalf("portrait panes don't cover [0,%d]: grid=[%d,%d] drum=[%d,%d]",
			h, grid.Min.Y, grid.Max.Y, drum.Min.Y, drum.Max.Y)
	}
}

// TestLayoutPaneCoverageLandscape checks that GridRect + DrumRect cover the full
// canvas in landscape mode (stacked on mobile — same as portrait).
func TestLayoutPaneCoverageLandscape(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	w, h := 844, 390
	g.Layout(w, h)

	grid := g.split.GridRect(w, h)
	drum := g.split.DrumRect(w, h)

	// Both should span full width (stacked layout)
	if grid.Min.X != 0 || grid.Max.X != w {
		t.Fatalf("landscape GridRect width: want [0,%d], got [%d,%d]", w, grid.Min.X, grid.Max.X)
	}
	if drum.Min.X != 0 || drum.Max.X != w {
		t.Fatalf("landscape DrumRect width: want [0,%d], got [%d,%d]", w, drum.Min.X, drum.Max.X)
	}
	// No gap between panes
	if grid.Max.Y != drum.Min.Y {
		t.Fatalf("landscape pane gap: GridRect.Max.Y=%d != DrumRect.Min.Y=%d", grid.Max.Y, drum.Min.Y)
	}
	// Together they cover full height
	if grid.Min.Y != 0 || drum.Max.Y != h {
		t.Fatalf("landscape panes don't cover [0,%d]: grid=[%d,%d] drum=[%d,%d]",
			h, grid.Min.Y, grid.Max.Y, drum.Min.Y, drum.Max.Y)
	}
}

// TestLayoutPaneCoverageAfterLandscapeToPortrait is the primary regression test:
// after rotating from landscape back to portrait, both panes must cover the full
// window width. Before the fix, content was squeezed to ~40-50% of the width.
func TestLayoutPaneCoverageAfterLandscapeToPortrait(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in portrait
	g.Layout(390, 844)

	// Rotate to landscape
	g.Layout(844, 390)

	// Rotate back to portrait — this is the failing scenario
	w, h := 390, 844
	g.Layout(w, h)

	grid := g.split.GridRect(w, h)
	drum := g.split.DrumRect(w, h)

	if grid.Dx() != w {
		t.Fatalf("after L→P, GridRect width=%d, want %d", grid.Dx(), w)
	}
	if drum.Dx() != w {
		t.Fatalf("after L→P, DrumRect width=%d, want %d", drum.Dx(), w)
	}
	if g.drum.Bounds.Dx() != drum.Dx() {
		t.Fatalf("after L→P, drum.Bounds.Dx()=%d != DrumRect.Dx()=%d", g.drum.Bounds.Dx(), drum.Dx())
	}
}

// TestLayoutPaneCoverageAfterPortraitToLandscape checks the reverse rotation.
func TestLayoutPaneCoverageAfterPortraitToLandscape(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in landscape
	g.Layout(844, 390)

	// Rotate to portrait
	g.Layout(390, 844)

	// Rotate back to landscape — still stacked on mobile
	w, h := 844, 390
	g.Layout(w, h)

	grid := g.split.GridRect(w, h)
	drum := g.split.DrumRect(w, h)

	// Both panes should span full width (stacked)
	if grid.Dx() != w {
		t.Fatalf("after P→L, GridRect width=%d, want %d", grid.Dx(), w)
	}
	if drum.Dx() != w {
		t.Fatalf("after P→L, DrumRect width=%d, want %d", drum.Dx(), w)
	}
}

// TestLayoutWidgetRectsWithinDrumBounds checks that after a L→P round-trip,
// all widget rects (Transport, Rack, Timeline) are within drum.Bounds.
func TestLayoutWidgetRectsWithinDrumBounds(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// L→P round-trip
	g.Layout(844, 390)
	g.Layout(390, 844)

	bounds := g.drum.Bounds
	if bounds.Empty() {
		t.Fatal("drum.Bounds empty after L→P rotation")
	}

	for kind, rect := range g.drum.widgetRects {
		if rect.Empty() {
			continue // some widgets may be hidden
		}
		if !rect.In(bounds) {
			t.Errorf("widget %v rect %v not within drum.Bounds %v", kind, rect, bounds)
		}
	}
}

// TestLayoutButtonRectsValidAfterRotation checks that play/stop/bpm buttons
// have non-empty rects within drum bounds after L→P rotation.
func TestLayoutButtonRectsValidAfterRotation(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// L→P round-trip
	g.Layout(844, 390)
	g.Layout(390, 844)

	bounds := g.drum.Bounds
	btns := map[string]*Button{
		"play":   g.drum.playBtn(),
		"stop":   g.drum.stopBtn(),
		"bpmInc": g.drum.bpmIncBtn(),
		"bpmDec": g.drum.bpmDecBtn(),
	}
	for name, btn := range btns {
		if btn == nil {
			t.Errorf("button %q is nil after rotation", name)
			continue
		}
		r := btn.Rect()
		if r.Empty() {
			t.Errorf("button %q rect is empty after L→P rotation", name)
			continue
		}
		if !r.In(bounds) {
			t.Errorf("button %q rect %v not within drum.Bounds %v", name, r, bounds)
		}
	}
}

// TestLayoutInitialPortraitNoTopGap checks that the first Layout call in
// portrait produces GridRect starting at Y=0 (no black gap at top).
func TestLayoutInitialPortraitNoTopGap(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)

	grid := g.split.GridRect(390, 844)
	if grid.Min.Y != 0 {
		t.Fatalf("initial portrait GridRect.Min.Y=%d, want 0", grid.Min.Y)
	}
}

// TestLayoutInitialPortraitNoCoverageGap checks that there is no gap between
// the grid and drum panes on initial layout.
func TestLayoutInitialPortraitNoCoverageGap(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)

	grid := g.split.GridRect(390, 844)
	drum := g.split.DrumRect(390, 844)

	if grid.Max.Y != drum.Min.Y {
		t.Fatalf("gap between panes: GridRect.Max.Y=%d != DrumRect.Min.Y=%d", grid.Max.Y, drum.Min.Y)
	}
}

// TestLayoutRapidRotationStress performs 10 rapid orientation changes and
// verifies pane coverage remains valid after each.
func TestLayoutRapidRotationStress(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	dims := [][2]int{
		{390, 844}, {844, 390}, {390, 844}, {844, 390}, {390, 844},
		{844, 390}, {390, 844}, {844, 390}, {390, 844}, {844, 390},
	}
	for i, d := range dims {
		w, h := d[0], d[1]
		g.Layout(w, h)

		grid := g.split.GridRect(w, h)
		drum := g.split.DrumRect(w, h)

		// Mobile always stacked — both orientations use Y-based split
		if grid.Dx() != w || drum.Dx() != w {
			t.Fatalf("rotation %d (%dx%d): width mismatch grid=%d drum=%d want %d",
				i, w, h, grid.Dx(), drum.Dx(), w)
		}
		if grid.Max.Y != drum.Min.Y {
			t.Fatalf("rotation %d (%dx%d): gap grid.Max.Y=%d != drum.Min.Y=%d",
				i, w, h, grid.Max.Y, drum.Min.Y)
		}
	}
}

// TestLayoutLandscapeToPortraitGridCacheRebuild checks that after L→P,
// the grid cache dimensions no longer match the new layout, forcing a rebuild.
func TestLayoutLandscapeToPortraitGridCacheRebuild(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in landscape
	g.Layout(844, 390)
	// Simulate having a cached grid from a draw at landscape dimensions
	g.gridCache = ebiten.NewImage(844, 390)
	g.gridCacheW = 844
	g.gridCacheH = 390

	// Rotate to portrait — cache dimensions should no longer match new layout
	g.Layout(390, 844)

	newW := g.split.GridW(g.winW) + 2*g.gridCachePad
	newH := g.split.GridH(g.winH) + 2*g.gridCachePad
	if g.gridCache != nil && g.gridCacheW == newW && g.gridCacheH == newH {
		t.Fatal("gridCache should not match new dimensions after resize, but it does")
	}
}

// TestLayoutOrientationCacheInvalidation checks that the frame buffer is
// cleared on resize so the next frame does a full render.
func TestLayoutOrientationCacheInvalidation(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in portrait
	g.Layout(390, 844)
	// Simulate having a frame buffer from a previous draw
	g.frameBuffer = ebiten.NewImage(390, 844)
	g.frameBufferW = 390
	g.frameBufferH = 844

	// Rotate to landscape — frame buffer should be cleared (dimension change)
	g.Layout(844, 390)

	if g.frameBuffer != nil {
		t.Fatal("frameBuffer should be nil after resize to force full re-render")
	}
}

// TestLayoutOrientationFrameBufferInvalidation checks that frameBuffer is
// cleared on orientation change to prevent stale frame reuse.
func TestLayoutOrientationFrameBufferInvalidation(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in portrait
	g.Layout(390, 844)
	// Simulate having a frame buffer from a previous draw
	g.frameBuffer = ebiten.NewImage(390, 844)
	g.frameBufferW = 390
	g.frameBufferH = 844

	// Rotate to landscape — should clear frame buffer
	g.Layout(844, 390)

	if g.frameBuffer != nil {
		t.Fatal("frameBuffer should be nil after orientation change")
	}
}

// TestLayoutZeroDimensionGuard checks that Layout handles zero-size calls
// gracefully (returns at least 1x1, prevents zero-size splitter).
func TestLayoutZeroDimensionGuard(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	rw, rh := g.Layout(0, 0)
	if rw < 1 || rh < 1 {
		t.Fatalf("Layout(0,0) returned %dx%d, want at least 1x1", rw, rh)
	}
}
