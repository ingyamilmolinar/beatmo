//go:build test

package ui

import (
	"math"
	"testing"
)

// TestEdgeNodeColocationOnPanAndZoom verifies that edge endpoints project to the
// exact same screen positions as their corresponding node centers across zoom and pan.
func TestEdgeNodeColocationOnPanAndZoom(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)

	// Build two nodes with a single horizontal edge on row 0.
	n0 := g.tryAddNode(3, 2, 0) // arbitrary grid coords
	n1 := g.tryAddNode(8, 2, 0)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	check := func() {
		// Reconstruct the edge camera transform used in drawGridPane.
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		camScale := unitPx / g.grid.Unit()

		// Helper to project world→screen like DrawLineCam(cam).
		proj := func(wx, wy float64) (sx, sy float64) {
			sx = wx*camScale + offX
			sy = wy*camScale + offY + float64(topOffset)
			return
		}

		// Node centers from screen rects
		x10, y10, x11, y11 := g.nodeScreenRect(n0)
		cx0 := (x10 + x11) * 0.5
		cy0 := (y10 + y11) * 0.5
		x20, y20, x21, y21 := g.nodeScreenRect(n1)
		cx1 := (x20 + x21) * 0.5
		cy1 := (y20 + y21) * 0.5

		// Edge endpoints in world coords → screen
		ex0, ey0 := proj(n0.X, n0.Y)
		ex1, ey1 := proj(n1.X, n1.Y)

		// Tolerance: within 0.6 pixels to account for rounding.
		tol := 0.6
		if math.Abs(ex0-cx0) > tol || math.Abs(ey0-cy0) > tol {
			t.Fatalf("edge start not colocated with node0: got(%.3f,%.3f) want(%.3f,%.3f) scale=%.3f off=(%.0f,%.0f)", ex0, ey0, cx0, cy0, g.cam.Scale, offX, offY)
		}
		if math.Abs(ex1-cx1) > tol || math.Abs(ey1-cy1) > tol {
			t.Fatalf("edge end not colocated with node1: got(%.3f,%.3f) want(%.3f,%.3f) scale=%.3f off=(%.0f,%.0f)", ex1, ey1, cx1, cy1, g.cam.Scale, offX, offY)
		}
	}

	// Multiple scales and pans
	for _, sc := range []float64{0.75, 1.0, 1.33, 2.0} {
		g.cam.Scale = sc
		g.cam.Snap()
		for _, off := range []struct{ x, y float64 }{{0, 0}, {13, 7}, {101, 59}} {
			g.cam.OffsetX = off.x
			g.cam.OffsetY = off.y
			g.cam.Snap()
			check()
		}
	}
}
