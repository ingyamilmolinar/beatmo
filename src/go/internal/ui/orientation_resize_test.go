package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestOrientationChangeDrumLengthPreserved verifies that rotating from portrait
// to landscape and back does not permanently shrink the drum Length.
// Bug 1: SetBounds() clamps dv.Length via clampLength, but on returning to a
// wider orientation, the already-clamped value is used as input, so Length
// never grows back.
func TestOrientationChangeDrumLengthPreserved(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Start in landscape (wide drum pane)
	g.Layout(844, 390)

	origLen := g.drum.Length
	if origLen < 16 {
		// Set a known length via the graph so that drum and graph agree.
		g.drum.changeLength(32)
		g.drum.SetBeatLength(32)
		g.Layout(844, 390)
		origLen = g.drum.Length
	}

	t.Logf("landscape length=%d", origLen)

	// Rotate to portrait (narrower drum pane, Length may shrink)
	g.Layout(390, 844)
	portraitLen := g.drum.Length
	t.Logf("portrait length=%d", portraitLen)

	// Rotate back to landscape (wider pane again)
	g.Layout(844, 390)
	restoredLen := g.drum.Length

	t.Logf("restored landscape length=%d", restoredLen)

	// Length should recover to at least the original value.
	if restoredLen < origLen {
		t.Fatalf("drum length shrank permanently: original=%d, after round-trip=%d", origLen, restoredLen)
	}
}

// TestOrientationChangeNoGapsNoOverlap verifies that after rotation the grid
// and drum panes cover the full screen without gaps.
func TestOrientationChangeNoGapsNoOverlap(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		grid := g.split.GridRect(w, h)
		drum := g.split.DrumRect(w, h)

		// Mobile always stacked: grid top, drum bottom.
		if drum.Min.Y != grid.Max.Y {
			t.Errorf("[%s] gap between grid and drum: grid.Max.Y=%d drum.Min.Y=%d", label, grid.Max.Y, drum.Min.Y)
		}
		// Together they cover the full height.
		if grid.Min.Y != 0 || drum.Max.Y != h {
			t.Errorf("[%s] grid+drum don't cover full height: grid.Min.Y=%d drum.Max.Y=%d h=%d", label, grid.Min.Y, drum.Max.Y, h)
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
	// Rotate back
	check("portrait2", 390, 844)
}

// TestOrientationChangeButtonsInBounds verifies that after rotation all button
// rects are non-empty and within drum bounds.
func TestOrientationChangeButtonsInBounds(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		bounds := g.drum.Bounds

		btns := map[string]*Button{
			"play":   g.drum.playBtn,
			"stop":   g.drum.stopBtn,
			"bpmInc": g.drum.bpmIncBtn,
			"bpmDec": g.drum.bpmDecBtn,
			"subdiv": g.drum.subdivBtn,
		}
		for name, btn := range btns {
			r := btn.Rect()
			if r.Empty() {
				t.Errorf("[%s] %s button rect is empty", label, name)
				continue
			}
			if r.Min.X < bounds.Min.X || r.Min.Y < bounds.Min.Y ||
				r.Max.X > bounds.Max.X || r.Max.Y > bounds.Max.Y {
				t.Errorf("[%s] %s button rect %v outside drum bounds %v", label, name, r, bounds)
			}
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
}

// TestOrientationChangeCellWidthPositive verifies that after rotation the cell
// width is at least 1 pixel.
// Bug 2: calcLayout() computes dv.cell = w / len(Steps) which can be 0.
func TestOrientationChangeCellWidthPositive(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		if g.drum.cell < 1 {
			t.Errorf("[%s] cell width=%d < 1 after layout %dx%d", label, g.drum.cell, w, h)
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
	// Small viewport stress
	check("tiny", 320, 480)
}

// TestOrientationChangeTimelineRectValid verifies that after rotation the
// timeline rect is non-empty with positive dimensions inside drum bounds.
func TestOrientationChangeTimelineRectValid(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		tl := g.drum.timelineRect
		bounds := g.drum.Bounds

		if tl.Empty() {
			t.Errorf("[%s] timeline rect is empty", label)
			return
		}
		if tl.Dx() <= 0 || tl.Dy() <= 0 {
			t.Errorf("[%s] timeline rect has non-positive dimensions: %v", label, tl)
		}
		if tl.Min.X < bounds.Min.X || tl.Max.X > bounds.Max.X {
			t.Errorf("[%s] timeline rect %v outside drum bounds %v horizontally", label, tl, bounds)
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
}

// TestOrientationChangeWidgetRectsValid verifies that Transport, Rack, and
// Timeline widget rects are non-empty and within drum bounds after rotation.
func TestOrientationChangeWidgetRectsValid(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		bounds := g.drum.Bounds

		widgets := map[WidgetKind]string{
			WidgetTransport: "Transport",
			WidgetRack:      "Rack",
			WidgetTimeline:  "Timeline",
		}
		for kind, name := range widgets {
			r, ok := g.drum.widgetRects[kind]
			if !ok || r.Empty() {
				// Some widgets may legitimately be empty depending on configuration.
				t.Logf("[%s] %s widget rect is empty or missing", label, name)
				continue
			}
			if r.Min.X < bounds.Min.X || r.Max.X > bounds.Max.X ||
				r.Min.Y < bounds.Min.Y || r.Max.Y > bounds.Max.Y {
				t.Errorf("[%s] %s widget rect %v outside drum bounds %v", label, name, r, bounds)
			}
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
}

// TestOrientationRapidCycleStability cycles portrait↔landscape 10 times and
// checks that the final state has valid splitter, bounds, and length.
func TestOrientationRapidCycleStability(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			g.Layout(390, 844)
		} else {
			g.Layout(844, 390)
		}
	}

	// Final state should be valid.
	bounds := g.drum.Bounds
	if bounds.Empty() {
		t.Fatal("drum bounds empty after 10 orientation cycles")
	}
	if g.drum.Length < 1 {
		t.Fatalf("drum length=%d < 1 after 10 orientation cycles", g.drum.Length)
	}
	if g.drum.cell < 1 {
		t.Fatalf("cell width=%d < 1 after 10 orientation cycles", g.drum.cell)
	}
	if g.split == nil {
		t.Fatal("splitter is nil after 10 orientation cycles")
	}
}

// TestDesktopResizePreservesDrumLength verifies that a desktop window resize
// from wide to narrow and back preserves drum length.
func TestDesktopResizePreservesDrumLength(t *testing.T) {
	setupMobileTest(t, false) // desktop path

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)
	origLen := g.drum.Length

	// Shrink window
	g.Layout(640, 480)
	// Expand back
	g.Layout(1280, 720)
	restoredLen := g.drum.Length

	if restoredLen < origLen {
		t.Fatalf("desktop drum length shrank: original=%d, restored=%d", origLen, restoredLen)
	}
}

// TestOrientationChangeRowsVisible verifies that at least one row is visible
// after rotation in both orientations.
func TestOrientationChangeRowsVisible(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		vis := g.drum.visibleRows()
		if vis < 1 {
			t.Errorf("[%s] visibleRows()=%d < 1 after layout %dx%d", label, vis, w, h)
		}
	}

	check("portrait", 390, 844)
	check("landscape", 844, 390)
}

// TestOrientationChangeLabelWidthReasonable verifies that after rotation the
// label width is positive and not more than half the drum pane width.
// Bug 3: label width cache isn't invalidated on orientation change, so a cached
// value from a wide orientation may be too large for a narrow one.
func TestOrientationChangeLabelWidthReasonable(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	check := func(label string, w, h int) {
		t.Helper()
		g.Layout(w, h)
		lw := g.drum.labelW
		halfBounds := g.drum.Bounds.Dx() / 2

		if lw <= 0 {
			t.Errorf("[%s] labelW=%d <= 0", label, lw)
		}
		if lw > halfBounds {
			t.Errorf("[%s] labelW=%d > half of drum bounds width (%d)", label, lw, halfBounds)
		}
	}

	// Start wide, then go narrow to stress the cache
	check("landscape", 844, 390)
	check("portrait", 390, 844)
	// And back
	check("landscape2", 844, 390)
}
