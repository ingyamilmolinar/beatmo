//go:build test

package ui

import (
	"math"
	"testing"
)

// TestEdgeNodeColocationVertical ensures vertical edges align with node centers
// across a spread of pans and zooms.
func TestEdgeNodeColocationVertical(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Vertical pair
	n0 := g.tryAddNode(4, 1, 0)
	n1 := g.tryAddNode(4, 7, 0)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	check := func() {
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		camScale := unitPx / g.grid.Unit()
		proj := func(wx, wy float64) (sx, sy float64) {
			sx = wx*camScale + offX
			sy = wy*camScale + offY + float64(gridTopOffset())
			return
		}
		// Centers from rects
		ax1, ay1, ax2, ay2 := g.nodeScreenRect(n0)
		acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
		bx1, by1, bx2, by2 := g.nodeScreenRect(n1)
		bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
		// Edge endpoints
		ex0, ey0 := proj(n0.X, n0.Y)
		ex1, ey1 := proj(n1.X, n1.Y)
		tol := 0.6
		if math.Abs(ex0-acx) > tol || math.Abs(ey0-acy) > tol {
			t.Fatalf("vertical edge start not colocated: got(%.2f,%.2f) want(%.2f,%.2f) scale=%.2f off=(%.0f,%.0f)", ex0, ey0, acx, acy, g.cam.Scale, offX, offY)
		}
		if math.Abs(ex1-bcx) > tol || math.Abs(ey1-bcy) > tol {
			t.Fatalf("vertical edge end not colocated: got(%.2f,%.2f) want(%.2f,%.2f) scale=%.2f off=(%.0f,%.0f)", ex1, ey1, bcx, bcy, g.cam.Scale, offX, offY)
		}
	}

	for _, sc := range []float64{0.66, 1.0, 1.5, 2.25} {
		g.cam.Scale = sc
		g.cam.Snap()
		for _, off := range []struct{ x, y float64 }{{0, 0}, {7, 13}, {95, 33}} {
			g.cam.OffsetX = off.x
			g.cam.OffsetY = off.y
			g.cam.Snap()
			check()
		}
	}
}
