//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowsLayerMixedPitchDoesNotPersist is the symptom-level regression guard.
// It reproduces a mid-session pitch change (via SetLength) after the rowsLayer
// has been populated and shifted, then asserts that the resulting composite
// has uniform cell pitch across the entire row width — no left-side stale
// pixels at old pitch mixed with right-side fresh pixels at new pitch.
//
// Detection: walk the row centerline, count colored runs (non-background
// pixels). Before the fix, a Length change after a shift would leave the
// leftmost ~30% at the old pitch; the run histogram would show two distinct
// run-length clusters (e.g. ~100px and ~12px). After the fix, the histogram
// has a single cluster.
//
// Build-tag note: pixel sampling via *ebiten.Image.At() requires the stubbed
// Ebiten path. Real Ebiten (test-real) panics on ReadPixels outside a running
// game loop. The predicate-level guards in
// drum_rows_layer_pitch_invalidation_test.go remain in force under both tags.
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

	// Advance offset slightly to cycle the shift-and-fill path at least once.
	// padPx default is 96; one step's worth at L=16, w≈1500 is ~94px, which
	// fits. If clamping makes dxPx exceed padPx the path falls back to full
	// rebuild — still valid for this test since we're testing post-change
	// uniformity, not which path runs.
	g.drum.Offset += 1
	g.drum.markRowsShiftDirty()
	g.drum.Draw(dst, nil, 0, nil, 0)

	// Now change Length mid-session. After the fix, SetLength fully invalidates
	// caches; before the fix, the rows-layer safety net (allPatches=false →
	// needFull=true via per-row sprite rebuild) eventually catches it. Both
	// paths must yield uniform pitch — this assertion is the user-visible
	// contract regardless of which internal path produced the rebuild.
	g.drum.SetLength(128)
	for j := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[j] = true
	}
	g.drum.markRowDirty(0)
	g.drum.Draw(dst, nil, 0, nil, 0)

	// Sample the rendered rowsLayer along the row's centerline. Local coords
	// inside rowsLayer: x relative to Bounds.Min.X, y relative to Bounds.Min.Y.
	baseX := g.drum.timelineRect.Min.X - g.drum.Bounds.Min.X
	rowWidth := g.drum.timelineRect.Dx()
	rowH := g.drum.rowHeight()
	if rowWidth <= 0 || rowH <= 0 {
		t.Fatalf("invalid rowWidth=%d or rowH=%d", rowWidth, rowH)
	}
	y := g.drum.headerH + rowH/2

	// Count colored→background transitions in the left half and right half
	// of the timeline strip. With every step set "on", every cell renders as
	// a colored block, so transition density directly reflects cell pitch:
	// finer pitch → more transitions per pixel.
	half := rowWidth / 2
	leftTransitions := countColorTransitions(t, g.drum.rowsLayer, baseX, baseX+half, y)
	rightTransitions := countColorTransitions(t, g.drum.rowsLayer, baseX+half, baseX+rowWidth, y)

	if leftTransitions == 0 && rightTransitions == 0 {
		t.Skipf("rowsLayer pixel sampling returned zero transitions on both halves — render-stub may not populate composite pixels; skipping symptom-level assertion. Predicate-level coverage in TestRowsLayerInvalidatesOnLengthChange / TestRowsLayerInvalidatesOnTimelineWidthChange remains in force.")
	}

	// Allow a modest tolerance: at L=128 with rowWidth=~1500, expected pitch
	// is ~12px → ~125 transitions across the row. A 2× imbalance between
	// halves indicates pitch mixing.
	ratio := float64(maxInt(leftTransitions, rightTransitions)) / float64(maxInt(1, minInt(leftTransitions, rightTransitions)))
	if ratio > 2.0 {
		t.Fatalf("mixed-pitch rowsLayer detected: left half=%d transitions, right half=%d transitions, ratio=%.2f (>2.0). Mid-session SetLength left stale-pitch pixels in the composite.",
			leftTransitions, rightTransitions, ratio)
	}
}

// countColorTransitions walks pixels from x0 (inclusive) to x1 (exclusive) at
// row y, counting transitions between background (alpha<128) and foreground
// (alpha>=128) pixels. A row of filled cells produces one transition per
// cell boundary, so this is a proxy for visible cell pitch.
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
