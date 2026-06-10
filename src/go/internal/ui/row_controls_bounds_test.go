package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowControlsBoundsCoverAllRowsMobile verifies that in portrait orientation
// with 3+ rows, the computed bounds height covers all visible rows.
//
// Uses a 1280-px tall viewport so the rack column has room for 3 rows
// PLUS the addRowBtn footer ABOVE the bottom action bar / EQ peek strip
// (which together reserve ~68 px below the rack). With the smaller
// 844-px viewport that this test originally used, the rack zone (after
// the rowsBottom() clamp in drumview_layout.go) only had room for ≈1
// row above the bar — causing the test to flake against the legacy
// behavior where the rack erroneously overlapped the bar.
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
	// Bounds must span at least (nRows-1) * rowHeight to prove all rows are
	// included (the last row's buttons may not fill the full row height).
	minExpectedH := rh * (nRows - 1)

	if bounds.Dy() < minExpectedH {
		t.Fatalf("bounds height %d < expected minimum %d (%d rows, rowHeight=%d)",
			bounds.Dy(), minExpectedH, nRows, rh)
	}
}

// TestRowControlsBoundsCoverAllRowsLandscape verifies that in landscape
// orientation with 3+ rows, the computed bounds height covers all visible rows.
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

// TestMobileRowLabelRectsNonEmpty verifies that in portrait mode with 4 rows,
// every visible row's label and mute button have non-empty rects.
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

// TestLandscapeDrumPaneWidthSufficient verifies that in landscape mode the drum
// pane gets at least 45% of the screen width (with the 50/50 split).
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

// TestRowControlsBoundsDesktopUnchanged verifies that desktop layout with 3 rows
// still computes correct bounds (not broken by the mobile fix).
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

	// Use rackVisibleRows() — the row count the rack zone actually
	// lays out widgets for, since the bounds are computed from those
	// widgets. dv.visibleRows() (bounds-based) can over-estimate when
	// the widget board allocates row 2 (wave) some pixels but the rack
	// rect stops at the wave widget's top edge.
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
