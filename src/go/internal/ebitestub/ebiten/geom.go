//go:build test

package ebiten

type GeoM struct{}

func (g *GeoM) Translate(x, y float64) {}
func (g *GeoM) Concat(o GeoM)          {}
func (g *GeoM) Scale(x, y float64)     {}
func (g *GeoM) Reset()                 {}
func (g *GeoM) Rotate(theta float64)   {}
