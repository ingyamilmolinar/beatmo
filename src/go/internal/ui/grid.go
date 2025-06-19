package ui

import "math"

const GridStep = 60 // world-space px between vertices

// snap world coords to nearest vertex
func Snap(x, y float64) (gx, gy float64, ix, iy int) {
	ix = int(math.Round(x / GridStep))
	iy = int(math.Round(y / GridStep))
	return float64(ix * GridStep), float64(iy * GridStep), ix, iy
}

// StepPixels converts a camera scale to an integer pixel spacing between grid
// lines. This helps keep vertical and horizontal gaps consistent across zoom
// levels.
func StepPixels(scale float64) int {
	px := int(math.Round(scale * GridStep))
	if px < 1 {
		return 1
	}
	return px
}

// GridLines returns the screen-space coordinates of grid lines for the
// given camera and screen size. The returned slices contain pixel positions
// for vertical (xs) and horizontal (ys) lines.
func GridLines(cam *Camera, screenW, screenH int) (xs, ys []float64) {
	stepPx := StepPixels(cam.Scale)
	camScale := float64(stepPx) / float64(GridStep)
	offX := math.Round(cam.OffsetX)
	offY := math.Round(cam.OffsetY)

	minX := (-cam.OffsetX) / cam.Scale
	maxX := (float64(screenW) - cam.OffsetX) / cam.Scale
	minY := (-cam.OffsetY - float64(topOffset)) / cam.Scale
	maxY := (float64(screenH) - cam.OffsetY - float64(topOffset)) / cam.Scale

	startI := int(math.Floor(minX / GridStep))
	endI := int(math.Ceil(maxX / GridStep))
	startJ := int(math.Floor(minY / GridStep))
	endJ := int(math.Ceil(maxY / GridStep))

	for i := startI; i <= endI; i++ {
		xs = append(xs, float64(i*GridStep)*camScale+offX)
	}
	for j := startJ; j <= endJ; j++ {
		ys = append(ys, float64(j*GridStep)*camScale+offY)
	}
	return
}
