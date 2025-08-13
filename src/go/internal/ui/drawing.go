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

	// 3D bevel effect: lighter top/left, darker bottom/right.
	lx0, ly0 := float32(r.Min.X), float32(r.Min.Y)
	lx1, ly1 := float32(r.Max.X-1), float32(r.Max.Y-1)
	light := adjustColor(fc, 40)
	dark := adjustColor(fc, -40)
	vector.DrawFilledRect(dst, lx0, ly0, lx1-lx0+1, 1, light, false)
	vector.DrawFilledRect(dst, lx0, ly0, 1, ly1-ly0+1, light, false)
	vector.DrawFilledRect(dst, lx0, ly1, lx1-lx0+1, 1, dark, false)
	vector.DrawFilledRect(dst, lx1, ly0, 1, ly1-ly0+1, dark, false)

	vector.StrokeRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 1, border, false)
}

// adjustColor lightens or darkens a color by delta (-255..255).
func adjustColor(c color.Color, delta int) color.Color {
	r, g, b, a := c.RGBA()
	rr := clamp(int(r>>8)+delta, 0, 255)
	gg := clamp(int(g>>8)+delta, 0, 255)
	bb := clamp(int(b>>8)+delta, 0, 255)
	return color.RGBA{uint8(rr), uint8(gg), uint8(bb), uint8(a >> 8)}
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
