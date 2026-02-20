//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestCellWidthFillsTimeline verifies that cell * len(Steps) fills timelineRect.Dx()
// to within len(Steps) pixels (integer division remainder only).
func TestCellWidthFillsTimeline(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 1200, 600))
	// Ensure rows have steps
	if len(dv.Rows) == 0 || len(dv.Rows[0].Steps) == 0 {
		t.Fatal("DrumView should have at least one row with steps")
	}

	dv.calcLayout()

	nSteps := len(dv.Rows[0].Steps)
	cellTotal := dv.cell * nSteps
	tlW := dv.timelineRect.Dx()

	if tlW <= 0 {
		t.Fatalf("timelineRect width should be positive, got %d", tlW)
	}

	// The gap should be at most nSteps-1 pixels (integer division truncation).
	gap := tlW - cellTotal
	if gap < 0 {
		gap = -gap
	}
	if gap >= nSteps {
		t.Errorf("Cell total (%d*%d=%d) leaves %d px gap in timeline (%d px); expected gap < %d",
			dv.cell, nSteps, cellTotal, tlW-cellTotal, tlW, nSteps)
	}
}

// TestCellWidthAfterResize verifies cells fill timeline after resizing from narrow to wide.
func TestCellWidthAfterResize(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)

	// Start narrow (600x600)
	dv := NewDrumView(image.Rect(0, 300, 600, 600), g, logger)
	dv.widgets.SetBounds(image.Rect(0, 300, 600, 600))
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	nSteps := len(dv.Rows[0].Steps)
	if nSteps == 0 {
		t.Fatal("no steps in row")
	}

	// Check narrow layout fills
	narrowTLW := dv.timelineRect.Dx()
	narrowGap := narrowTLW - dv.cell*nSteps
	if narrowGap < 0 {
		narrowGap = -narrowGap
	}
	if narrowGap >= nSteps {
		t.Errorf("Narrow: cell total gap %d px >= %d steps", narrowGap, nSteps)
	}

	// Resize to wide (1200x600)
	dv.Bounds = image.Rect(0, 300, 1200, 600)
	dv.widgets.SetBounds(dv.Bounds)
	dv.labelWidthDirty = true
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	wideTLW := dv.timelineRect.Dx()
	wideGap := wideTLW - dv.cell*nSteps
	if wideGap < 0 {
		wideGap = -wideGap
	}
	if wideGap >= nSteps {
		t.Errorf("Wide: cell total (%d*%d=%d) leaves %d px gap in timeline (%d px); expected gap < %d",
			dv.cell, nSteps, dv.cell*nSteps, wideTLW-dv.cell*nSteps, wideTLW, nSteps)
	}
}

// TestCellWidthRoundTrip verifies cell width is stable across resize round-trip.
func TestCellWidthRoundTrip(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)

	// Start wide (1200x600)
	dv := NewDrumView(image.Rect(0, 300, 1200, 600), g, logger)
	dv.widgets.SetBounds(image.Rect(0, 300, 1200, 600))
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	initialCell := dv.cell

	// Resize to narrow
	dv.Bounds = image.Rect(0, 300, 600, 600)
	dv.widgets.SetBounds(dv.Bounds)
	dv.labelWidthDirty = true
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	// Resize back to wide
	dv.Bounds = image.Rect(0, 300, 1200, 600)
	dv.widgets.SetBounds(dv.Bounds)
	dv.labelWidthDirty = true
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	finalCell := dv.cell

	if finalCell != initialCell {
		t.Errorf("Cell width should be stable across round-trip resize: initial=%d final=%d", initialCell, finalCell)
	}
}
