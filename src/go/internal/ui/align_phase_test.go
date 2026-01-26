package ui

import (
	"math"
	"testing"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestNodeGridPhaseOnPanAndZoom verifies that node screen positions remain in
// phase with grid subdivisions across zoom and pan. We compare the node center
// against the expected pixel coordinate computed via rounded world→screen math
// that the grid tile uses.
func TestNodeGridPhaseOnPanAndZoom(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Place a single node at (i=10,j=5) grid coords
	n := g.tryAddNode(10, 5, 0)

	// Helper: check phase error for current camera.
	check := func() {
		// unit: px per subdivision
		stepPx := g.grid.StepPixels(g.cam.Scale)
		maxDiv := g.grid.MaxDiv()
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		// Node center predicted by grid rounding (same as grid tile lines):
		expX := offX + math.Round(float64(n.I)*float64(stepPx)/float64(maxDiv))
		expY := offY + float64(topOffset) + math.Round(float64(n.J)*float64(stepPx)/float64(maxDiv))
		// Actual node screen center from nodeScreenRect
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		cx := (x1 + x2) * 0.5
		cy := (y1 + y2) * 0.5
		if math.Abs(cx-expX) > 0.6 || math.Abs(cy-expY) > 0.6 {
			t.Fatalf("node/grid out of phase: got(%.3f,%.3f) exp(%.3f,%.3f) scale=%.3f off=(%.0f,%.0f) stepPx=%d", cx, cy, expX, expY, g.cam.Scale, offX, offY, stepPx)
		}
	}

	// Check at multiple scales
	for _, sc := range []float64{0.75, 1.0, 1.33, 2.0} {
		g.cam.Scale = sc
		g.cam.Snap()
		check()
		// Pan around and re-check
		for _, off := range []struct{ x, y float64 }{{0, 0}, {13, 7}, {101, 59}} {
			g.cam.OffsetX = off.x
			g.cam.OffsetY = off.y
			g.cam.Snap()
			check()
		}
	}
}
