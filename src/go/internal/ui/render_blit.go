package ui

import "github.com/hajimehoshi/ebiten/v2"

// blitCache centralizes cache blitting so tests can capture the applied
// translation offsets without needing to introspect GeoM.
var blitCache = func(dst, src *ebiten.Image, ox, oy int) {
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(ox), float64(oy))
	dst.DrawImage(src, &op)
}
