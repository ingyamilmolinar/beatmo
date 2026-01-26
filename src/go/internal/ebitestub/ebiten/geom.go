//go:build test

package ebiten

// GeoM tracks simple scale/translate transforms for test stubs. Rotation is
// accepted but ignored; this is sufficient for the lightweight draw routines
// used in headless tests.
type GeoM struct {
	scaleX, scaleY         float64
	translateX, translateY float64
}

func (g *GeoM) Reset() {
	g.scaleX, g.scaleY = 1, 1
	g.translateX, g.translateY = 0, 0
}

func (g *GeoM) Translate(x, y float64) {
	g.translateX += x
	g.translateY += y
}

func (g *GeoM) Scale(x, y float64) {
	if g.scaleX == 0 && g.scaleY == 0 {
		g.scaleX, g.scaleY = 1, 1
	}
	g.scaleX *= x
	g.scaleY *= y
}

func (g *GeoM) Rotate(theta float64) {}

func (g *GeoM) Concat(o GeoM) {
	if g.scaleX == 0 && g.scaleY == 0 {
		g.scaleX, g.scaleY = 1, 1
	}
	if o.scaleX == 0 && o.scaleY == 0 {
		o.scaleX, o.scaleY = 1, 1
	}
	g.scaleX *= o.scaleX
	g.scaleY *= o.scaleY
	g.translateX += o.translateX
	g.translateY += o.translateY
}
