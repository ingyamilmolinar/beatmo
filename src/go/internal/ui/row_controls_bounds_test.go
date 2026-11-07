package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// 1280 px tall, so the rack holds three rows plus the footer above the bottom bar.
func TestRowControlsBoundsCoverAllRowsMobile(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Add rows so we have 3 total (default is 1)
	g.drum.AddRow()
	g.drum.AddRow()

	g.Layout(390, 1280)
	advanceFrames(g, 2)

	bounds := g.drum.computeRowControlsBounds()
	if bounds.Empty() {
		t.Fatal("computeRowControlsBounds returned empty rect for 3-row mobile portrait")
	}

	nRows := len(g.drum.Rows)
	rh := g.drum.rowHeight()
	// (nRows-1) rows at least: the last row's buttons may not fill its height.
	minExpectedH := rh * (nRows - 1)

	if bounds.Dy() < minExpectedH {
		t.Fatalf("bounds height %d < expected minimum %d (%d rows, rowHeight=%d)",
			bounds.Dy(), minExpectedH, nRows, rh)
	}
}

func TestRowControlsBoundsCoverAllRowsLandscape(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.drum.AddRow()
	g.drum.AddRow()

	g.Layout(844, 390)
	advanceFrames(g, 2)

	bounds := g.drum.computeRowControlsBounds()
	if bounds.Empty() {
		t.Fatal("computeRowControlsBounds returned empty rect for 3-row mobile landscape")
	}

	vis := g.drum.visibleRows()
	if vis > len(g.drum.Rows) {
		vis = len(g.drum.Rows)
	}
	rh := g.drum.rowHeight()
	// Bounds must span at least (vis-1) * rowHeight
	minExpectedH := rh * (vis - 1)

	if bounds.Dy() < minExpectedH {
		t.Fatalf("bounds height %d < expected minimum %d (%d visible rows, rowHeight=%d)",
			bounds.Dy(), minExpectedH, vis, rh)
	}
}

func TestMobileRowLabelRectsNonEmpty(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Add rows to get 4 total
	g.drum.AddRow()
	g.drum.AddRow()
	g.drum.AddRow()

	g.Layout(390, 844)
	advanceFrames(g, 2)

	vis := g.drum.visibleRows()
	endRow := g.drum.rowOffset + vis
	if endRow > len(g.drum.Rows) {
		endRow = len(g.drum.Rows)
	}

	for i := g.drum.rowOffset; i < endRow; i++ {
		if i < len(g.drum.rowLabels()) {
			r := g.drum.rowLabels()[i].Rect()
			if r.Empty() {
				t.Errorf("row %d label rect is empty", i)
			}
		} else {
			t.Errorf("row %d has no label button (rowLabels len=%d)", i, len(g.drum.rowLabels()))
		}

		if i < len(g.drum.rowMuteBtns()) {
			r := g.drum.rowMuteBtns()[i].Rect()
			if r.Empty() {
				t.Errorf("row %d mute button should be visible inline on mobile, got empty rect", i)
			}
		} else {
			t.Errorf("row %d has no mute button (rowMuteBtns len=%d)", i, len(g.drum.rowMuteBtns()))
		}
	}
}

// At least 45% with the 50/50 split.
func TestLandscapeDrumPaneWidthSufficient(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	w, h := 844, 390
	g.Layout(w, h)
	advanceFrames(g, 2)

	drumW := g.drum.Bounds.Dx()
	minDrumW := w * 45 / 100

	if drumW < minDrumW {
		t.Fatalf("drum pane width %d < minimum %d (45%% of %d); split.X=%d",
			drumW, minDrumW, w, g.split.X)
	}

	// Timeline should have positive width
	timelineW := g.drum.Bounds.Dx()
	if timelineW <= 0 {
		t.Fatal("drum pane (timeline container) has non-positive width")
	}
}

func TestRowControlsBoundsDesktopUnchanged(t *testing.T) {
	setupMobileTest(t, false) // desktop

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.drum.AddRow()
	g.drum.AddRow()

	g.Layout(1280, 720)
	advanceFrames(g, 2)

	bounds := g.drum.computeRowControlsBounds()
	if bounds.Empty() {
		t.Fatal("desktop computeRowControlsBounds returned empty rect for 3-row layout")
	}

	// The rows the rack lays out widgets for: visibleRows() can over-count when the board gives the wave row a few
	// pixels.
	vis := g.drum.rackVisibleRows()
	if vis > len(g.drum.Rows) {
		vis = len(g.drum.Rows)
	}
	rh := g.drum.rowHeight()
	// Bounds must span at least (vis-1) * rowHeight
	minExpectedH := rh * (vis - 1)

	if bounds.Dy() < minExpectedH {
		t.Fatalf("desktop bounds height %d < expected minimum %d (%d visible rows, rowHeight=%d)",
			bounds.Dy(), minExpectedH, vis, rh)
	}

	// Width should be positive and within drum bounds
	if bounds.Dx() <= 0 {
		t.Fatal("desktop bounds width is non-positive")
	}
}
