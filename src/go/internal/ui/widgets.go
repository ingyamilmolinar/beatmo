package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

const Tile = 40 // world-space pixels per grid step (before camera scale)

var pixelCache = map[uint32]*ebiten.Image{}

func packRGBA(c color.Color) uint32 {
	// Shift each 16-bit channel down first: ORing them into 8-bit slots collided whole colour families onto one key.
	// c.RGBA() directly, not RGBAModel.Convert, which boxes and allocates on every drawRect; the result is bit-identical.
	r, g, b, a := c.RGBA()
	return uint32(r>>8) | uint32(g>>8)<<8 | uint32(b>>8)<<16 | uint32(a>>8)<<24
}

func pixel(c color.Color) *ebiten.Image {
	k := packRGBA(c)
	if img, ok := pixelCache[k]; ok {
		return img
	}
	img := ebiten.NewImage(1, 1)
	img.Fill(c)
	pixelCache[k] = img
	return img
}

var lineOpt ebiten.DrawImageOptions
var debugGeom = (os.Getenv("DEBUG_GEOM") == "1")

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
	// A no-op in test builds, where logLineFinal is stubbed.
	logLineFinal(lineOpt.GeoM, thick)

	if debugGeom {
		// World-space only, so tests (stubbed ebiten) stay buildable.
		fmt.Printf("[DRAW-LINE] world=(%.2f,%.2f)->(%.2f,%.2f) thick=%.3f\n", x1, y1, x2, y2, thick)
	}
}
