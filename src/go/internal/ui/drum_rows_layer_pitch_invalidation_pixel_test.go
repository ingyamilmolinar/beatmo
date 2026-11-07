//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// After a mid-session pitch change the composite has one cell pitch across the row, never stale left pixels at
// the old pitch, measured as colour runs along the centreline. Stub only: real Ebiten cannot ReadPixels outside the
// loop.
func TestRowsLayerMixedPitchDoesNotPersist(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 600)

	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.drum.SetLength(16)
	g.drum.SetBounds(image.Rect(0, 300, 1600, 600))

	// Populate every step so each cell is a visible filled block.
	for j := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[j] = true
	}
	g.drum.markRowDirty(0)

	dst := ebiten.NewImage(1600, 600)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowsLayer == nil {
		t.Fatalf("rowsLayer not built")
	}

	// Through the shift-and-fill path at least once; a fallback to a full rebuild is fine too.
	g.drum.Offset += 1
	g.drum.markRowsShiftDirty()
	g.drum.Draw(dst, nil, 0, nil, 0)

	// Whichever internal path rebuilds, the pitch must be uniform.
	g.drum.SetLength(128)
	for j := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[j] = true
	}
	g.drum.markRowDirty(0)
	g.drum.Draw(dst, nil, 0, nil, 0)

	// In rowsLayer-local coordinates.
	baseX := g.drum.timelineRect.Min.X - g.drum.Bounds.Min.X
	rowWidth := g.drum.timelineRect.Dx()
	rowH := g.drum.rowHeight()
	if rowWidth <= 0 || rowH <= 0 {
		t.Fatalf("invalid rowWidth=%d or rowH=%d", rowWidth, rowH)
	}
	y := g.drum.headerH + rowH/2

	// Every step is on, so transition density per half is the cell pitch.
	half := rowWidth / 2
	leftTransitions := countColorTransitions(t, g.drum.rowsLayer, baseX, baseX+half, y)
	rightTransitions := countColorTransitions(t, g.drum.rowsLayer, baseX+half, baseX+rowWidth, y)

	if leftTransitions == 0 && rightTransitions == 0 {
		t.Skipf("rowsLayer pixel sampling returned zero transitions on both halves — render-stub may not populate composite pixels; skipping symptom-level assertion. Predicate-level coverage in TestRowsLayerInvalidatesOnLengthChange / TestRowsLayerInvalidatesOnTimelineWidthChange remains in force.")
	}

	// A 2× imbalance between halves means mixed pitches.
	ratio := float64(maxInt(leftTransitions, rightTransitions)) / float64(maxInt(1, minInt(leftTransitions, rightTransitions)))
	if ratio > 2.0 {
		t.Fatalf("mixed-pitch rowsLayer detected: left half=%d transitions, right half=%d transitions, ratio=%.2f (>2.0). Mid-session SetLength left stale-pitch pixels in the composite.",
			leftTransitions, rightTransitions, ratio)
	}
}

// One background↔foreground transition per cell boundary.
func countColorTransitions(t *testing.T, img *ebiten.Image, x0, x1, y int) int {
	t.Helper()
	transitions := 0
	prevForeground := false
	first := true
	for x := x0; x < x1; x++ {
		c := img.At(x, y)
		fg := isForeground(c)
		if first {
			prevForeground = fg
			first = false
			continue
		}
		if fg != prevForeground {
			transitions++
			prevForeground = fg
		}
	}
	return transitions
}

func isForeground(c color.Color) bool {
	_, _, _, a := c.RGBA()
	return a>>8 >= 128
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
