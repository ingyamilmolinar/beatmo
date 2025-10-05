package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawRect draws a rectangle. It is defined as a variable so tests can
// override it to capture draw calls.
var drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
	if r.Empty() {
		return
	}
	// Fast path: draw axis-aligned rectangles via scaled 1x1 pixel blits.
	// This avoids vector path overhead and is significantly faster in WASM.
	px := pixel(c)
	if filled {
		var op ebiten.DrawImageOptions
		op.GeoM.Scale(float64(r.Dx()), float64(r.Dy()))
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(px, &op)
		return
	}
	// 1px stroke on each side.
	// Top
	var top ebiten.DrawImageOptions
	top.GeoM.Scale(float64(r.Dx()), 1)
	top.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(px, &top)
	// Bottom
	var bot ebiten.DrawImageOptions
	bot.GeoM.Scale(float64(r.Dx()), 1)
	bot.GeoM.Translate(float64(r.Min.X), float64(r.Max.Y-1))
	dst.DrawImage(px, &bot)
	// Left
	var left ebiten.DrawImageOptions
	left.GeoM.Scale(1, float64(r.Dy()))
	left.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(px, &left)
	// Right
	var right ebiten.DrawImageOptions
	right.GeoM.Scale(1, float64(r.Dy()))
	right.GeoM.Translate(float64(r.Max.X-1), float64(r.Min.Y))
	dst.DrawImage(px, &right)
}

// drawButton renders a filled rectangle with a border. It can be overridden in tests.
var drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
	if r.Empty() {
		return
	}
	fc := fill
	if pressed {
		if c, ok := fill.(color.RGBA); ok {
			fc = color.RGBA{c.R / 2, c.G / 2, c.B / 2, c.A}
		}
	}
	// Fill
	drawRect(dst, r, fc, true)
	// 3D bevel effect: lighter top/left, darker bottom/right.
	light := adjustColor(fc, 40)
	dark := adjustColor(fc, -40)
	// Top
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), light, true)
	// Left
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), light, true)
	// Bottom
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), dark, true)
	// Right
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), dark, true)
	// Border
	drawRect(dst, r, border, false)
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

// Icon primitives (screen-space, font independent)
var drawPlayIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	// Right-pointing triangle with vertical base on the left.
	if r.Empty() {
		return
	}
	x0 := r.Min.X
	x1 := r.Max.X - 1
	y0 := r.Min.Y
	y1 := r.Max.Y - 1
	mid := (y0 + y1) / 2
	// Fill top half
	for y := y0; y <= mid; y++ {
		if mid-y == 0 {
			continue
		}
		t := float64(y-y0) / float64(max1(mid-y0))
		xr := int(math.Round(float64(x0) + t*float64(x1-x0)))
		drawRect(dst, image.Rect(x0, y, xr, y+1), col, true)
	}
	// Fill bottom half
	for y := mid; y <= y1; y++ {
		denom := max1(y1 - mid)
		t := float64(y1-y) / float64(denom)
		xr := int(math.Round(float64(x0) + t*float64(x1-x0)))
		drawRect(dst, image.Rect(x0, y, xr, y+1), col, true)
	}
}

var drawPauseIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	w := r.Dx()
	gap := max1(w / 6)
	bar := max1(w / 5)
	left := image.Rect(r.Min.X, r.Min.Y, minI(r.Min.X+bar, r.Max.X), r.Max.Y)
	right := image.Rect(maxI(r.Max.X-bar, r.Min.X), r.Min.Y, r.Max.X, r.Max.Y)
	// Ensure spacing
	if right.Min.X-left.Max.X < gap {
		right.Min.X = left.Max.X + gap
		if right.Min.X > r.Max.X {
			right.Min.X = r.Max.X
		}
	}
	drawRect(dst, left, col, true)
	drawRect(dst, right, col, true)
}

var drawStopIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	size := minI(r.Dx(), r.Dy()) * 3 / 4
	if size < 2 {
		size = minI(r.Dx(), r.Dy())
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	rect := image.Rect(cx-size/2, cy-size/2, cx+size/2, cy+size/2)
	drawRect(dst, rect, col, true)
}

var drawPencilIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	// Draw a thin diagonal stroke from bottom-left toward top-right.
	w := r.Dx()
	h := r.Dy()
	thickness := max1(minI(w, h) / 10)
	if thickness < 2 {
		thickness = 2
	}
	x0 := r.Min.X + w/6
	y0 := r.Max.Y - h/6
	x1 := r.Max.X - w/6
	y1 := r.Min.Y + h/6
	steps := max1(absI(x1 - x0))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		drawRect(dst, image.Rect(x, y, x+thickness, y+thickness), col, true)
	}
}

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func absI(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
