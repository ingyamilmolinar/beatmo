package ui

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const Tile = 40 // world-space pixels per grid step (before camera scale)

/* ------------------------------------------------------------------
   cache 1×1 images per colour
   ------------------------------------------------------------------ */

var pixelCache = map[string]*ebiten.Image{}

func key(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("%d_%d_%d_%d", r, g, b, a)
}

func pixel(c color.Color) *ebiten.Image {
	k := key(c)
	if img, ok := pixelCache[k]; ok {
		return img
	}
	img := ebiten.NewImage(1, 1)
	img.Fill(c)
	pixelCache[k] = img
	return img
}

/*
------------------------------------------------------------------

	DrawLineCam – world-coords → line with camera transform
	------------------------------------------------------------------
*/
var lineOpt ebiten.DrawImageOptions

func DrawLineCam(dst *ebiten.Image,
	x1, y1, x2, y2 float64,
	cam *ebiten.GeoM,
	col color.Color, thick float64) {

	if thick <= 0 {
		thick = 1
	}
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	angle := math.Atan2(dy, dx)

	// reset GeoM in place (no new allocation)
	lineOpt.GeoM.Reset()
	lineOpt.GeoM.Scale(length, thick)
	lineOpt.GeoM.Rotate(angle)
	lineOpt.GeoM.Translate(x1, y1)
	lineOpt.GeoM.Concat(*cam)

	dst.DrawImage(pixel(col), &lineOpt)
}
