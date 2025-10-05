package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

const Tile = 40 // world-space pixels per grid step (before camera scale)

/* ------------------------------------------------------------------
   cache 1×1 images per colour
   ------------------------------------------------------------------ */

var pixelCache = map[uint32]*ebiten.Image{}

func packRGBA(c color.Color) uint32 {
	r, g, b, a := color.RGBAModel.Convert(c).(color.RGBA).RGBA()
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
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

/*
------------------------------------------------------------------

	DrawLineCam – world-coords → line with camera transform
	------------------------------------------------------------------
*/
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
	// Log final on-screen rectangle if available (non-test builds)
	// This helps correlate the visual with numeric endpoints.
	// In test builds, logLineFinal is not linked and this call is a no-op.
	logLineFinal(lineOpt.GeoM, thick)

	if debugGeom {
		// Log the world-space endpoints; screen-space can be inferred from
		// [DRAW-CAM] logs (camScale/offX/offY) without depending on GeoM internals.
		// This keeps tests (which stub ebiten) buildable.
		fmt.Printf("[DRAW-LINE] world=(%.2f,%.2f)->(%.2f,%.2f) thick=%.3f\n", x1, y1, x2, y2, thick)
	}
}
