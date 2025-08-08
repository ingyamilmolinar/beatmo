package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawRect draws a rectangle. It is defined as a variable so tests can
// override it to capture draw calls.
var drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
	if filled {
		vector.DrawFilledRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), c, false)
	} else {
		vector.StrokeRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 1, c, false)
	}
}
