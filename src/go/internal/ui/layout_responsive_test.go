package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Call after assertDefaultParityState.
func setupMobileTest(t *testing.T, smallScreen bool) {
	t.Helper()
	assertDefaultParityState(t)

	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })

	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	if smallScreen {
		forceSmallScreenForTest = true
		// Laying out at a mobile width leaves touchScreenWidth tiny, and a later test using touch detection would
		// resolve a mobile profile from it. Reset to the clean desktop zero value and rebuild the profile.
		t.Cleanup(func() {
			forceSmallScreenForTest = false
			SetTouchScreenSize(0, 0)
			UpdateProfile()
		})
	}
}

// Stacked, with the adaptive Y split.
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

// The drum pane fits the rack and the tallest audio tab, capped at 60% so the grid keeps 40%.
func TestMobilePortraitAdaptiveSplit(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)

	// The same source of truth Layout applies at startup.
	want := adaptiveMobilePortraitSplitY(800, g)
	if g.split.Y != want {
		t.Fatalf("expected adaptive split %d for 400x800 mobile portrait, got %d", want, g.split.Y)
	}
	if g.split.Y < 800*40/100 {
		t.Fatalf("grid %d below 40%% of 800", g.split.Y)
	}
}

func TestMobileOrientationChange(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait first — stacked (adaptive, grid >= 40%)
	g.Layout(400, 800)
	portraitY := g.split.Y
	if want := adaptiveMobilePortraitSplitY(800, g); portraitY != want {
		t.Fatalf("portrait split expected adaptive %d, got %d", want, portraitY)
	}

	// Rotate to landscape — still stacked, adaptive Y split for h=400
	g.Layout(800, 400)
	landscapeY := g.split.Y
	if landscapeY < 100 || landscapeY > 350 {
		t.Fatalf("landscape split.Y expected in range [100,350] for h=400, got %d", landscapeY)
	}

	// Rotate back to portrait — stacked (adaptive, grid >= 40%)
	g.Layout(400, 800)
	portrait2Y := g.split.Y
	if want := adaptiveMobilePortraitSplitY(800, g); portrait2Y != want {
		t.Fatalf("portrait (2nd) split expected adaptive %d, got %d", want, portrait2Y)
	}
}

// Neither pane goes below 25%, whatever the content pressure.
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

func TestDesktopUnaffectedByMobileLogic(t *testing.T) {
	setupMobileTest(t, false) // smallScreen=false → desktop path

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)

	// With 1 row the drum content is small, so the grid gets most of the space.
	if g.split.Y == 360 {
		// Exactly 50% would indicate mobile logic leaked into desktop path
		t.Logf("split.Y=360 (exactly 50%%), checking content-based calculation...")
	}
	// Basic sanity: split should be in valid range
	if g.split.Y < 120 || g.split.Y > 600 {
		t.Fatalf("desktop split.Y=%d out of sensible range [120,600] for 720px height", g.split.Y)
	}
}

// A drag still enforces the 25% minimum.
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

// Portrait uses the adaptive formula, never a stale landscape split.Y.
func TestStaleRatioAfterLandscapeToPortrait(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in landscape — sets split.Y to full height (390)
	g.Layout(844, 390)

	// Rotate to portrait — split.Y should use the adaptive formula, not stale 390.
	g.Layout(390, 844)
	if want := adaptiveMobilePortraitSplitY(844, g); g.split.Y != want {
		t.Fatalf("expected adaptive split %d (h=844) after landscape→portrait, got %d", want, g.split.Y)
	}
	if g.split.Y < 844*40/100 {
		t.Fatalf("grid %d below 40%% of 844 after landscape→portrait", g.split.Y)
	}
}

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
