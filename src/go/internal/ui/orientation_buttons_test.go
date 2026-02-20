package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestLandscapeButtonRectsInsideDrumBounds verifies that after laying out in
// landscape (side-by-side), drum pane buttons are positioned inside drum bounds.
func TestLandscapeButtonRectsInsideDrumBounds(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400) // landscape

	db := g.drum.Bounds
	if db.Empty() {
		t.Fatal("drum bounds empty after landscape layout")
	}

	playR := g.drum.playBtn.Rect()
	stopR := g.drum.stopBtn.Rect()

	if playR.Empty() {
		t.Fatal("playBtn rect is empty")
	}
	if stopR.Empty() {
		t.Fatal("stopBtn rect is empty")
	}

	// In landscape side-by-side, drum pane starts at split.X.
	// Buttons must be inside the drum bounds.
	if playR.Min.X < db.Min.X {
		t.Fatalf("playBtn.Min.X=%d < drumBounds.Min.X=%d", playR.Min.X, db.Min.X)
	}
	if stopR.Min.X < db.Min.X {
		t.Fatalf("stopBtn.Min.X=%d < drumBounds.Min.X=%d", stopR.Min.X, db.Min.X)
	}
	if playR.Max.X > db.Max.X {
		t.Fatalf("playBtn.Max.X=%d > drumBounds.Max.X=%d", playR.Max.X, db.Max.X)
	}
	if stopR.Max.X > db.Max.X {
		t.Fatalf("stopBtn.Max.X=%d > drumBounds.Max.X=%d", stopR.Max.X, db.Max.X)
	}
}

// TestPortraitToLandscapeButtonRectsUpdate verifies that switching from portrait
// to landscape moves drum pane buttons to the right-side drum pane.
func TestPortraitToLandscapeButtonRectsUpdate(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait first — stacked
	g.Layout(400, 800)
	playPortrait := g.drum.playBtn.Rect()
	if playPortrait.Empty() {
		t.Fatal("playBtn rect empty in portrait")
	}
	// In portrait, buttons should be near x=0 (drum bounds start at x=0).
	if playPortrait.Min.X > 50 {
		t.Fatalf("portrait: playBtn.Min.X=%d expected near 0", playPortrait.Min.X)
	}

	// Switch to landscape — side-by-side
	g.Layout(800, 400)
	playLandscape := g.drum.playBtn.Rect()
	if playLandscape.Empty() {
		t.Fatal("playBtn rect empty after switching to landscape")
	}
	// In landscape, drum pane starts at split.X (~400). Buttons must shift right.
	if playLandscape.Min.X < g.split.X {
		t.Fatalf("landscape: playBtn.Min.X=%d < split.X=%d — buttons didn't move", playLandscape.Min.X, g.split.X)
	}
}

// TestLandscapeToPortraitButtonRectsUpdate verifies that switching from landscape
// back to portrait repositions buttons to the bottom stacked pane.
func TestLandscapeToPortraitButtonRectsUpdate(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Landscape first — side-by-side
	g.Layout(800, 400)
	playLandscape := g.drum.playBtn.Rect()
	if playLandscape.Empty() {
		t.Fatal("playBtn rect empty in landscape")
	}
	if playLandscape.Min.X < g.split.X {
		t.Fatalf("landscape: playBtn.Min.X=%d < split.X=%d", playLandscape.Min.X, g.split.X)
	}

	// Switch to portrait — stacked
	g.Layout(400, 800)
	playPortrait := g.drum.playBtn.Rect()
	if playPortrait.Empty() {
		t.Fatal("playBtn rect empty after switching to portrait")
	}
	// In portrait, drum bounds start at x=0, y=split.Y.
	if playPortrait.Min.X > 50 {
		t.Fatalf("portrait: playBtn.Min.X=%d expected near 0 after switching from landscape", playPortrait.Min.X)
	}
	if playPortrait.Min.Y < g.split.Y {
		t.Fatalf("portrait: playBtn.Min.Y=%d < split.Y=%d — buttons didn't move to bottom pane", playPortrait.Min.Y, g.split.Y)
	}
}

// TestOrientationRoundTripButtonsValid verifies that after a full
// Portrait→Landscape→Portrait round trip, buttons remain valid and inside drum bounds.
func TestOrientationRoundTripButtonsValid(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	checkButtons := func(label string) {
		t.Helper()
		db := g.drum.Bounds
		if db.Empty() {
			t.Fatalf("%s: drum bounds empty", label)
		}
		playR := g.drum.playBtn.Rect()
		stopR := g.drum.stopBtn.Rect()
		if playR.Empty() {
			t.Fatalf("%s: playBtn rect empty", label)
		}
		if stopR.Empty() {
			t.Fatalf("%s: stopBtn rect empty", label)
		}
		if !playR.In(db) {
			t.Fatalf("%s: playBtn %v not inside drumBounds %v", label, playR, db)
		}
		if !stopR.In(db) {
			t.Fatalf("%s: stopBtn %v not inside drumBounds %v", label, stopR, db)
		}
	}

	g.Layout(400, 800) // portrait
	checkButtons("portrait")

	g.Layout(800, 400) // landscape
	checkButtons("landscape")

	g.Layout(400, 800) // portrait again
	checkButtons("portrait-2")
}

// TestOrientationSwitchCameraRecenters verifies that on first layout with
// default start enabled, the camera centers in the grid pane.
func TestOrientationSwitchCameraRecenters(t *testing.T) {
	setupMobileTest(t, true)
	// Enable default start so centering logic runs.
	SetDefaultStartForTest(true)
	t.Cleanup(func() { SetDefaultStartForTest(false) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait — camera should center for full-width grid pane
	g.Layout(400, 800)

	// In stacked mode, grid pane is full width (400). Camera X should be ~200.
	expectedCamX := float64(400) / 2
	if g.cam.OffsetX < expectedCamX*0.5 || g.cam.OffsetX > expectedCamX*1.5 {
		t.Fatalf("portrait camX=%.0f expected near %.0f", g.cam.OffsetX, expectedCamX)
	}

	// Switch to landscape — stacked, grid pane is full width (800)
	// Camera was already centered on first Layout, so it won't re-center
	// (centered=true). That's expected — the camera position is valid.
	g.Layout(800, 400)
	if g.cam.OffsetX <= 0 {
		t.Fatalf("landscape camX=%.0f should be positive", g.cam.OffsetX)
	}
}

// TestOrientationSwitchResetsAutoSize verifies that auto-sizing produces
// a valid split.Y after rotating (both stacked on mobile).
func TestOrientationSwitchResetsAutoSize(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Portrait layout
	g.Layout(400, 800)

	portraitY := g.split.Y
	if portraitY < 400 || portraitY > 700 {
		t.Fatalf("portrait split.Y=%d out of range [400,700] for h=800", portraitY)
	}

	// Rotate to landscape — still stacked, adaptive Y for h=400
	g.Layout(800, 400)

	// split.Y should be auto-sized for the new height
	if g.split.Y < 100 || g.split.Y > 350 {
		t.Fatalf("landscape split.Y=%d out of range [100,350] for h=400", g.split.Y)
	}
}
