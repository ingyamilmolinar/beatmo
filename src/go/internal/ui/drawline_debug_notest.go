//go:build !test

package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// debugApply logs the final on-screen corners of the line rectangle by applying
// the composed transform. This uses ebiten.GeoM.Element which is not available
// under our test stub, so it lives behind a !test build tag.
func debugApply(m ebiten.GeoM, px, py float64) (sx, sy float64) {
	a := m.Element(0, 0)
	b := m.Element(0, 1)
	c := m.Element(0, 2)
	d := m.Element(1, 0)
	e := m.Element(1, 1)
	f := m.Element(1, 2)
	sx = a*px + b*py + c
	sy = d*px + e*py + f
	return
}

// logLineFinal prints the on-screen rectangle corners if DEBUG_GEOM=1.
func logLineFinal(m ebiten.GeoM, thick float64) {
	if !debugGeom {
		return
	}
	x0, y0 := debugApply(m, 0, 0)
	x1, y1 := debugApply(m, 1, 0)
	x2, y2 := debugApply(m, 1, 1)
	x3, y3 := debugApply(m, 0, 1)
	fmt.Printf("[DRAW-LINE-OUT] p0=(%.2f,%.2f) p1=(%.2f,%.2f) p2=(%.2f,%.2f) p3=(%.2f,%.2f) t=%.3f\n", x0, y0, x1, y1, x2, y2, x3, y3, thick)
}
