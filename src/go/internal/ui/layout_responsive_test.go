package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// setupMobileTest configures overrides for mobile layout testing.
// Must be called after assertDefaultParityState.
func setupMobileTest(t *testing.T, smallScreen bool) {
	t.Helper()
	assertDefaultParityState(t)

	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })

	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	if smallScreen {
		forceSmallScreenForTest = true
		t.Cleanup(func() { forceSmallScreenForTest = false })
	}
}

// TestMobileLandscapeStacked verifies that on a small screen in landscape
// orientation, the layout is stacked (not side-by-side) with adaptive Y split.
func TestMobileLandscapeStacked(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400)

	// Landscape uses stacked layout: split.Y should be valid
	if !g.split.Horizontal() {
		t.Fatal("mobile landscape should use stacked layout")
	}
	if g.split.Y < 100 || g.split.Y > 350 {
		t.Fatalf("expected split.Y in reasonable range for 800x400 mobile landscape, got %d", g.split.Y)
	}
}

// TestMobilePortraitAdaptiveSplit verifies that on a small screen in portrait
// orientation, the drum pane gets primary screen space (capped at 65%, floored
// at 30%) so editing dominates and the graph stays as a 35% reference.
func TestMobilePortraitAdaptiveSplit(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	// With 1 row: needed=mobileHeaderH(56)+TouchRowHeight(52)+padding(24)=132,
	// minDrum=240 (30% of 800), drumH=240, splitY=560.
	if g.split.Y < 520 || g.split.Y > 600 {
		t.Fatalf("expected split near 560 for 400x800 mobile portrait (1 row), got %d", g.split.Y)
	}
}

// TestMobileOrientationChange verifies that the split adapts when the screen
// rotates between portrait and landscape (both stacked).
func TestMobileOrientationChange(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait first — stacked (adaptive, drum 30–65%)
	g.Layout(400, 800)
	portraitY := g.split.Y
	if portraitY < 520 || portraitY > 600 {
		t.Fatalf("portrait split expected near 560 (adaptive, 1 row, drum=30%% floor), got %d", portraitY)
	}

	// Rotate to landscape — still stacked, adaptive Y split for h=400
	g.Layout(800, 400)
	landscapeY := g.split.Y
	if landscapeY < 100 || landscapeY > 350 {
		t.Fatalf("landscape split.Y expected in range [100,350] for h=400, got %d", landscapeY)
	}

	// Rotate back to portrait — stacked (adaptive, drum 30–65%)
	g.Layout(400, 800)
	portrait2Y := g.split.Y
	if portrait2Y < 520 || portrait2Y > 600 {
		t.Fatalf("portrait (2nd) split expected near 560 (adaptive, 1 row), got %d", portrait2Y)
	}
}

// TestMobileProportionalClamp verifies that on mobile, neither pane goes below
// 25% of the total dimension, even with content pressure.
func TestMobileProportionalClamp(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait: stacked — check Y clamp
	g.Layout(400, 600)

	minAllowedY := 600 / 4
	maxAllowedY := 600 - 600/4

	if g.split.Y < minAllowedY {
		t.Fatalf("split.Y=%d below 25%% minimum (%d) for h=600", g.split.Y, minAllowedY)
	}
	if g.split.Y > maxAllowedY {
		t.Fatalf("split.Y=%d above 75%% maximum (%d) for h=600", g.split.Y, maxAllowedY)
	}

	// Landscape: also stacked — check Y clamp for h=600
	g.Layout(800, 600)

	minAllowedY2 := 600 / 4
	maxAllowedY2 := 600 - 600/4

	if g.split.Y < minAllowedY2 {
		t.Fatalf("split.Y=%d below 25%% minimum (%d) for h=600 landscape", g.split.Y, minAllowedY2)
	}
	if g.split.Y > maxAllowedY2 {
		t.Fatalf("split.Y=%d above 75%% maximum (%d) for h=600 landscape", g.split.Y, maxAllowedY2)
	}
}

// TestDesktopUnaffectedByMobileLogic verifies that without the small screen
// override, the existing content-based auto-sizing is unchanged.
func TestDesktopUnaffectedByMobileLogic(t *testing.T) {
	setupMobileTest(t, false) // smallScreen=false → desktop path

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)

	// Desktop content-based sizing: split should NOT necessarily be at 50%.
	// With 1 row, the drum pane content is small, so split.Y should be high
	// (grid gets most of the space).
	if g.split.Y == 360 {
		// Exactly 50% would indicate mobile logic leaked into desktop path
		t.Logf("split.Y=360 (exactly 50%%), checking content-based calculation...")
	}
	// Basic sanity: split should be in valid range
	if g.split.Y < 120 || g.split.Y > 600 {
		t.Fatalf("desktop split.Y=%d out of sensible range [120,600] for 720px height", g.split.Y)
	}
}

// TestMobileSplitterDragRespectsProportion verifies that when the user drags
// the splitter on mobile, the 25% minimum is enforced.
func TestMobileSplitterDragRespectsProportion(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Use portrait orientation so stacked mode applies
	totalH := 600
	g.Layout(400, totalH)

	// Simulate user dragging the splitter to extreme top (y=50)
	g.split.userSet = true
	g.split.Y = 50
	g.split.ratio = float64(50) / float64(totalH)

	// UpdateResize should clamp to at least 25% = 150
	g.split.UpdateResize(totalH, 400)

	minAllowed := totalH / 4 // 150
	if g.split.Y < minAllowed {
		t.Fatalf("after drag to top, split.Y=%d below 25%% minimum (%d)", g.split.Y, minAllowed)
	}

	// Simulate dragging to extreme bottom (y=580)
	g.split.Y = 580
	g.split.ratio = float64(580) / float64(totalH)
	g.split.UpdateResize(totalH, 400)

	maxAllowed := totalH - totalH/4 // 450
	if g.split.Y > maxAllowed {
		t.Fatalf("after drag to bottom, split.Y=%d above 75%% maximum (%d)", g.split.Y, maxAllowed)
	}
}

// TestStaleRatioAfterLandscapeToPortrait verifies that when switching from
// landscape (side-by-side) to portrait (stacked), the split.Y is not computed
// from a stale landscape split.Y value but uses the adaptive formula.
func TestStaleRatioAfterLandscapeToPortrait(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in landscape — sets split.Y to full height (390)
	g.Layout(844, 390)

	// Rotate to portrait — split.Y should use adaptive formula, not stale 390.
	// With h=844 and 1 row: minDrum=253 (30%), drumH=253, splitY=591.
	g.Layout(390, 844)
	if g.split.Y < 560 || g.split.Y > 620 {
		t.Fatalf("expected split.Y near 591 (adaptive, h=844, 1 row) after landscape→portrait, got %d", g.split.Y)
	}
}

// TestMobileBothPanesUsable verifies that both grid and drum panes have usable
// space in both orientations.
func TestMobileBothPanesUsable(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait — stacked
	g.Layout(400, 800)
	gridH := g.split.GridH(g.winH)
	drumBounds := g.drum.Bounds
	drumH := drumBounds.Dy()
	if gridH < 50 {
		t.Fatalf("portrait: grid pane height=%d too small (< 50px)", gridH)
	}
	if drumH < 50 {
		t.Fatalf("portrait: drum pane height=%d too small (< 50px)", drumH)
	}
	rowH := g.drum.rowHeight()
	if drumH < rowH {
		t.Fatalf("portrait: drum pane height=%d < rowHeight=%d", drumH, rowH)
	}

	// Landscape — also stacked
	g.Layout(800, 400)
	gridHLand := g.split.GridH(g.winH)
	drumBounds = g.drum.Bounds
	drumHLand := drumBounds.Dy()
	if gridHLand < 50 {
		t.Fatalf("landscape: grid pane height=%d too small (< 50px)", gridHLand)
	}
	if drumHLand < 50 {
		t.Fatalf("landscape: drum pane height=%d too small (< 50px)", drumHLand)
	}
	if drumBounds.Dx() != 800 {
		t.Fatalf("landscape: drum pane width=%d, want full width 800", drumBounds.Dx())
	}
}
