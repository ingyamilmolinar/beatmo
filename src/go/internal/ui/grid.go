package ui

import "math"

const DefaultGridStep = 60 // world-space px between vertices

// Grid encapsulates grid spacing so future subdivisions can vary the step.
type Grid struct {
	Step float64 // world-space px between vertices
}

func NewGrid(step float64) *Grid { return &Grid{Step: step} }

// Snap world coords to nearest vertex.
func (g *Grid) Snap(x, y float64) (gx, gy float64, ix, iy int) {
	ix = int(math.Round(x / g.Step))
	iy = int(math.Round(y / g.Step))
	return float64(ix) * g.Step, float64(iy) * g.Step, ix, iy
}

// StepPixels converts a camera scale to an integer pixel spacing between grid
// lines. This helps keep vertical and horizontal gaps consistent across zoom
// levels.
func (g *Grid) StepPixels(scale float64) int {
	px := int(math.Round(scale * g.Step))
	if px < 1 {
		return 1
	}
	return px
}

// Lines returns the screen-space coordinates of grid lines for the
// given camera and screen size. The returned slices contain pixel positions
// for vertical (xs) and horizontal (ys) lines.
func (g *Grid) Lines(cam *Camera, screenW, screenH int) (xs, ys []float64) {
	stepPx := g.StepPixels(cam.Scale)
	camScale := float64(stepPx) / g.Step
	offX := math.Round(cam.OffsetX)
	offY := math.Round(cam.OffsetY)

	minX := (-cam.OffsetX) / cam.Scale
	maxX := (float64(screenW) - cam.OffsetX) / cam.Scale
	minY := (-cam.OffsetY - float64(topOffset)) / cam.Scale
	maxY := (float64(screenH) - cam.OffsetY - float64(topOffset)) / cam.Scale

	startI := int(math.Floor(minX / g.Step))
	endI := int(math.Ceil(maxX / g.Step))
	startJ := int(math.Floor(minY / g.Step))
	endJ := int(math.Ceil(maxY / g.Step))

	for i := startI; i <= endI; i++ {
		xs = append(xs, float64(i)*g.Step*camScale+offX)
	}
	for j := startJ; j <= endJ; j++ {
		ys = append(ys, float64(j)*g.Step*camScale+offY)
	}
	return
}
