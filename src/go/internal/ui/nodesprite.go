package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image/color"
)

type spriteKey struct {
	rpx            int
	fr, fg, fb, fa uint8
	br, bg, bb, ba uint8
}

func rgba8(c color.Color) (r, g, b, a uint8) {
	cr, cg, cb, ca := color.RGBAModel.Convert(c).(color.RGBA).RGBA()
	return uint8(cr), uint8(cg), uint8(cb), uint8(ca)
}

// buildNodeSprite renders a filled rectangle with a 1px border into a
// screen-space image of size (2r)x(2r).
func buildNodeSprite(fill, border color.Color, r int) *ebiten.Image {
	if r < 1 {
		r = 1
	}
	w, h := 2*r, 2*r
	img := ebiten.NewImage(w, h)
	img.Fill(fill)
	// Border lines (1px) using scaled 1x1 pixel of the border color.
	px := pixel(border)
	// Top
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(float64(w), 1)
	img.DrawImage(px, &op)
	// Bottom
	op2 := ebiten.DrawImageOptions{}
	op2.GeoM.Scale(float64(w), 1)
	op2.GeoM.Translate(0, float64(h-1))
	img.DrawImage(px, &op2)
	// Left
	op3 := ebiten.DrawImageOptions{}
	op3.GeoM.Scale(1, float64(h))
	img.DrawImage(px, &op3)
	// Right
	op4 := ebiten.DrawImageOptions{}
	op4.GeoM.Scale(1, float64(h))
	op4.GeoM.Translate(float64(w-1), 0)
	img.DrawImage(px, &op4)
	return img
}
