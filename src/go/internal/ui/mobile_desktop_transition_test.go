package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// ─── Mobile ↔ Desktop Transition Tests ───────────────────────────

// TestSmallScreenTransitionResetsUserSetSplitter verifies that crossing
// the mobile ↔ desktop threshold clears userSet so auto-sizing takes over.
func TestSmallScreenTransitionResetsUserSetSplitter(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in desktop mode
	forceSmallScreenForTest = false
	g.Layout(1200, 800)

	// Simulate user dragging the splitter
	g.split.userSet = true
	g.split.Y = 200

	// Transition to mobile
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g.Layout(390, 844)

	if g.split.userSet {
		t.Fatal("expected userSet=false after mobile transition")
	}
	// Auto-sizing should have recalculated split.Y
	if g.split.Y == 200 {
		t.Fatal("expected split.Y to be recalculated after transition")
	}
}

// TestSmallScreenTransitionPreservesCellData verifies that Steps and CellTypes
// data is preserved (not zeroed) when Length reclamps during transitions.
func TestSmallScreenTransitionPreservesCellData(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in desktop mode with known data
	forceSmallScreenForTest = false
	g.Layout(1200, 800)

	if len(g.drum.Rows) == 0 || len(g.drum.Rows[0].Steps) == 0 {
		t.Fatal("need at least one row with steps")
	}

	// Set some cells to true
	origLen := len(g.drum.Rows[0].Steps)
	for i := 0; i < origLen && i < 4; i++ {
		g.drum.Rows[0].Steps[i] = true
		g.drum.Rows[0].CellTypes[i] = model.NodeTypeRegular
	}

	// Transition to mobile (may reclamp length)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g.Layout(390, 844)

	// Check preserved data (up to min of old/new length)
	newLen := len(g.drum.Rows[0].Steps)
	checkLen := origLen
	if newLen < checkLen {
		checkLen = newLen
	}
	if checkLen > 4 {
		checkLen = 4
	}
	for i := 0; i < checkLen; i++ {
		if !g.drum.Rows[0].Steps[i] {
			t.Fatalf("Steps[%d] zeroed after transition, expected true", i)
		}
		if g.drum.Rows[0].CellTypes[i] != model.NodeTypeRegular {
			t.Fatalf("CellTypes[%d] zeroed after transition, expected NodeTypeRegular", i)
		}
	}
}

// TestSmallScreenTransitionMobileEQReInitializes verifies that leaving mobile
// and re-entering causes the mobile EQ init to re-run (collapsing EQ again).
func TestSmallScreenTransitionMobileEQReInitializes(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Enter mobile
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g.Layout(390, 844)

	if !g.drum.mobileEQInited {
		t.Fatal("expected mobileEQInited=true after mobile layout")
	}

	// Go desktop
	forceSmallScreenForTest = false
	g.Layout(1200, 800)

	if g.drum.mobileEQInited {
		t.Fatal("expected mobileEQInited=false after desktop transition")
	}

	// Re-enter mobile — mobileEQInited should be false so recalcButtons re-runs init
	forceSmallScreenForTest = true
	g.Layout(390, 844)

	// After layout + recalcButtons, mobileEQInited should be set again
	g.drum.recalcButtons()
	if !g.drum.mobileEQInited {
		t.Fatal("expected mobileEQInited=true after re-entering mobile")
	}
	if !g.drum.mobileEQCollapsed {
		t.Fatal("expected mobileEQCollapsed=true after re-entering mobile")
	}
}

// TestSmallScreenTransitionClearsEQMode verifies that mobileEQMode is cleared
// when transitioning from mobile to desktop.
func TestSmallScreenTransitionClearsEQMode(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start mobile with EQ mode active
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g.Layout(390, 844)
	g.drum.mobileEQMode = true

	// Go desktop
	forceSmallScreenForTest = false
	g.Layout(1200, 800)

	if g.drum.mobileEQMode {
		t.Fatal("expected mobileEQMode=false after desktop transition")
	}
	if g.drum.currentViewMode != viewModeRows {
		t.Fatalf("expected currentViewMode=viewModeRows after desktop transition, got %v", g.drum.currentViewMode)
	}
}

// TestSmallScreenRoundTripLayoutCorrectness cycles mobile→desktop→mobile→desktop
// and verifies layout sanity at each step.
func TestSmallScreenRoundTripLayoutCorrectness(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { forceSmallScreenForTest = false })

	type step struct {
		small bool
		w, h  int
	}
	steps := []step{
		{true, 390, 844},
		{false, 1200, 800},
		{true, 414, 896},
		{false, 1400, 900},
	}

	for i, s := range steps {
		forceSmallScreenForTest = s.small
		g.Layout(s.w, s.h)

		mode := "desktop"
		if s.small {
			mode = "mobile"
		}

		// split.Y must be within window bounds
		if g.split.Y <= 0 || g.split.Y >= s.h {
			t.Fatalf("step %d (%s): split.Y=%d out of bounds [1, %d)", i, mode, g.split.Y, s.h)
		}

		// Drum bounds must be valid
		db := g.drum.Bounds
		if db.Dx() <= 0 || db.Dy() <= 0 {
			t.Fatalf("step %d (%s): drum bounds empty: %v", i, mode, db)
		}

		// Grid + drum should cover the window vertically
		gridH := g.split.Y
		drumH := s.h - g.split.Y
		if gridH+drumH != s.h {
			t.Fatalf("step %d (%s): gridH(%d)+drumH(%d) != winH(%d)", i, mode, gridH, drumH, s.h)
		}

		// visibleRows > 0 (at least one row should be visible)
		vis := g.drum.visibleRows()
		if vis <= 0 {
			t.Fatalf("step %d (%s): visibleRows=%d, want > 0", i, mode, vis)
		}
	}
}

// TestSmallScreenTransitionCacheInvalidation verifies that caches are properly
// invalidated after a mode transition.
func TestSmallScreenTransitionCacheInvalidation(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start desktop
	forceSmallScreenForTest = false
	g.Layout(1200, 800)

	// Set some cache state to non-dirty
	g.drum.bgDirty = false
	g.drum.rowsLayerDirty = false

	// Transition to mobile
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g.Layout(390, 844)

	if !g.drum.bgDirty {
		t.Fatal("expected bgDirty=true after transition")
	}
	if !g.drum.rowsLayerDirty {
		t.Fatal("expected rowsLayerDirty=true after transition")
	}
	if g.drum.toolbarCache != nil {
		t.Fatal("expected toolbarCache=nil after transition")
	}
}
