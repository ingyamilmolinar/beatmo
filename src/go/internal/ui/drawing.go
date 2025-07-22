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

// drawButton renders a filled rectangle with a border. It can be overridden in tests.
var drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
	fc := fill
	if pressed {
		if c, ok := fill.(color.RGBA); ok {
			fc = color.RGBA{c.R / 2, c.G / 2, c.B / 2, c.A}
		}
	}
	vector.DrawFilledRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), fc, false)
	vector.StrokeRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 1, border, false)
}
