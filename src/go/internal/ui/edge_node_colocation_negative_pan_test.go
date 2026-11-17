//go:build test

package ui

import (
	"math"
	"testing"
)

// Negative panning should not break colocation due to modulo arithmetic.
func TestEdgeNodeColocationNegativePan(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	a := g.tryAddNode(2, 3, 0)
	b := g.tryAddNode(9, 3, 0)
	g.addEdge(a, b)
	g.updateBeatInfos()

	// Large negative pans across tile boundaries
	for _, off := range []struct{ x, y float64 }{{-500, -320}, {-81, -41}, {-1234, -987}} {
		g.cam.OffsetX = off.x
		g.cam.OffsetY = off.y
		g.cam.Scale = 1.37
		g.cam.Snap()
		// Compare projected edge endpoints with node centers
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		camScale := unitPx / g.grid.Unit()
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		proj := func(wx, wy float64) (sx, sy float64) {
			sx = wx*camScale + offX
			sy = wy*camScale + offY + float64(gridTopOffset())
			return
		}
		// Node centers
		ax1, ay1, ax2, ay2 := g.nodeScreenRect(a)
		acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
		bx1, by1, bx2, by2 := g.nodeScreenRect(b)
		bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
		// Edge endpoints
		ex0, ey0 := proj(a.X, a.Y)
		ex1, ey1 := proj(b.X, b.Y)
		tol := 0.6
		if math.Abs(ex0-acx) > tol || math.Abs(ey0-acy) > tol {
			t.Fatalf("neg pan start mismatch: got(%.2f,%.2f) want(%.2f,%.2f) off=(%.0f,%.0f)", ex0, ey0, acx, acy, offX, offY)
		}
		if math.Abs(ex1-bcx) > tol || math.Abs(ey1-bcy) > tol {
			t.Fatalf("neg pan end mismatch: got(%.2f,%.2f) want(%.2f,%.2f) off=(%.0f,%.0f)", ex1, ey1, bcx, bcy, offX, offY)
		}
	}
}
